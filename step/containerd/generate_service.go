package containerd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"

	"github.com/mensylisir/xmcores/connector" // Required for connector.Host
	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

const (
	DefaultContainerdServiceFilePath = "/etc/systemd/system/containerd.service"
	// Default containerd systemd service file content
	// Note: You might need to adjust Environment="PATH=..." based on where runc and other tools are.
	// Or ensure /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin is in the system's default PATH for services.
	DefaultContainerdServiceTemplate = `[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart={{.ContainerdBinPath}}
KillMode=process
Delegate=yes
LimitNOFILE=1048576
# Having non-zero Limit*s causes performance problems due to accounting overhead
# in the kernel. We recommend using cgroups to do container-local accounting.
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
`
)

// GenerateServiceContainerdStep creates the systemd service file for containerd.
type GenerateServiceContainerdStep struct {
	step.BaseStep
	ServiceFilePath     string // Remote path for the systemd service file
	ServiceFileTemplate string // Template for the service file content
	ContainerdBinPath   string // Path to the containerd binary on the remote host
}

// NewGenerateServiceContainerdStep creates a new GenerateServiceContainerdStep.
func NewGenerateServiceContainerdStep(containerdBinPath string) step.Step {
	return &GenerateServiceContainerdStep{
		BaseStep:            step.NewBaseStep("GenerateServiceContainerd", "Generate systemd service file for Containerd"),
		ServiceFilePath:     DefaultContainerdServiceFilePath,
		ServiceFileTemplate: DefaultContainerdServiceTemplate,
		ContainerdBinPath:   containerdBinPath,
	}
}

func (s *GenerateServiceContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.ContainerdBinPath == "" {
		// Default based on common installation path, assuming DeliveryContainerdStep used /usr/local/bin
		s.ContainerdBinPath = "/usr/local/bin/containerd"
	}
	if s.ServiceFilePath == "" {
		return fmt.Errorf("service file path cannot be empty")
	}
	if s.ServiceFileTemplate == "" {
		return fmt.Errorf("service file template cannot be empty")
	}

	log.Infof("GenerateServiceContainerdStep initialized: ServiceFilePath=%s, ContainerdBinPath=%s",
		s.ServiceFilePath, s.ContainerdBinPath)
	return s.BaseStep.Init(rt, log)
}

func (s *GenerateServiceContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting GenerateServiceContainerd step: %s", s.Description())
	ctx := context.Background()

	tmpl, err := template.New("containerdService").Parse(s.ServiceFileTemplate)
	if err != nil {
		return "Failed to parse service file template", false, fmt.Errorf("error parsing containerd service template: %w", err)
	}

	templateData := struct {
		ContainerdBinPath string
	}{
		ContainerdBinPath: s.ContainerdBinPath,
	}

	var serviceFileContentBuffer bytes.Buffer
	if err := tmpl.Execute(&serviceFileContentBuffer, templateData); err != nil {
		return "Failed to execute service file template", false, fmt.Errorf("error executing containerd service template: %w", err)
	}
	serviceFileContent := serviceFileContentBuffer.String()

	// Using a struct for synchronized access to shared variables by goroutines
	processingTracker := struct {
		mu             sync.Mutex
		errors         []string
		outputs        []string
		overallSuccess bool
	}{
		overallSuccess: true,
	}
	var wg sync.WaitGroup

	targetHosts := rt.AllHosts()
	if len(targetHosts) == 0 {
		log.Warn("No target hosts specified in runtime. Skipping service generation.")
		return "No target hosts specified, skipping service generation.", true, nil
	}

	for _, hostEntry := range targetHosts {
		wg.Add(1)
		go func(host connector.Host) { // Use connector.Host from import
			defer wg.Done()
			hostLog := log.WithField("host", host.GetName())

			hostConn, hostRunner, err := rt.GetHostConnectorAndRunner(host)
			if err != nil {
				err := fmt.Errorf("host %s (%s): failed to get connector/runner: %w", host.GetName(), host.GetAddress(), err)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				return
			}
			defer func() {
				if err_close := hostConn.Close(); err_close != nil {
					hostLog.Warnf("Error closing connection: %v", err_close)
				}
			}()

			hostLog.Debugf("Successfully obtained connector and runner for host %s", host.GetName())

			// remoteExec is not directly used here, hostConn.Scp is used.
			// remoteExec, err := executor.NewRemoteExecutor(hostConn, hostRunner)
			// if err != nil {
			// 	err := fmt.Errorf("host %s (%s): failed to create remote executor: %w", host.GetName(), host.GetAddress(), err)
			// 	hostLog.Error(err.Error())
			// 	processingTracker.mu.Lock()
			// 	processingTracker.errors = append(processingTracker.errors, err.Error())
			// 	processingTracker.overallSuccess = false
			// 	processingTracker.mu.Unlock()
			// 	return
			// }

			hostLog.Infof("Writing containerd service file to %s on host %s", s.ServiceFilePath, host.GetName())

			contentReader := strings.NewReader(serviceFileContent)
			// Use 0644 for systemd service files
			err = hostConn.Scp(ctx, contentReader, s.ServiceFilePath, int64(len(serviceFileContent)), 0644)

			if err != nil {
				err := fmt.Errorf("host %s (%s): failed to write service file %s: %w", host.GetName(), host.GetAddress(), s.ServiceFilePath, err)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				// if !rt.IgnoreError() { // This logic is complex with goroutines, handled by overallSuccess
				// 	return
				// }
			} else {
				msg := fmt.Sprintf("Host %s (%s): Successfully wrote service file to %s", host.GetName(), host.GetAddress(), s.ServiceFilePath)
				hostLog.Info(msg)
				processingTracker.mu.Lock()
				processingTracker.outputs = append(processingTracker.outputs, msg)
				processingTracker.mu.Unlock()
			}
		}(hostEntry)
	}
	wg.Wait()

	finalOutput := strings.Join(processingTracker.outputs, "\n")
	if len(processingTracker.errors) > 0 {
		errorSummary := strings.Join(processingTracker.errors, "\n")
		finalOutput = fmt.Sprintf("Completed with errors.\nOutputs:\n%s\nErrors:\n%s", finalOutput, errorSummary)
		log.Errorf("GenerateServiceContainerd step completed with errors:\n%s", errorSummary)
		return finalOutput, processingTracker.overallSuccess, fmt.Errorf("one or more errors occurred during service file generation: %s", errorSummary)
	}

	log.Infof("GenerateServiceContainerd step completed successfully for all processed hosts.")
	return finalOutput, true, nil
}
