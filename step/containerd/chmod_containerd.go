package containerd

import (
	"context"
	"fmt"
	"os"
	"path" // For remote paths
	"strings"
	"sync"

	"github.com/mensylisir/xmcores/connector" // Required for connector.Host type
	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// ChmodContainerdStep ensures correct permissions for containerd binaries on remote hosts.
type ChmodContainerdStep struct {
	step.BaseStep
	TargetDir   string      // Remote directory where binaries are (e.g., /usr/local/bin)
	Binaries    []string    // List of binary names
	Permissions os.FileMode // Desired permissions (e.g., 0755)
}

// NewChmodContainerdStep creates a new ChmodContainerdStep.
func NewChmodContainerdStep(targetDir string, binaries []string, permissions os.FileMode) step.Step {
	if len(binaries) == 0 {
		binaries = DefaultContainerdBinaries // Use the same default as Delivery
	}
	if permissions == 0 {
		permissions = 0755 // Default executable permissions
	}
	return &ChmodContainerdStep{
		BaseStep:    step.NewBaseStep("ChmodContainerd", "Set executable permissions for Containerd binaries"),
		TargetDir:   targetDir,
		Binaries:    binaries,
		Permissions: permissions,
	}
}

func (s *ChmodContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.TargetDir == "" {
		s.TargetDir = "/usr/local/bin" // Default target directory
	}
	if len(s.Binaries) == 0 {
		return fmt.Errorf("binaries list cannot be empty for ChmodContainerd step")
	}
	if s.Permissions == 0 {
		return fmt.Errorf("permissions cannot be zero for ChmodContainerd step; use a valid file mode like 0755")
	}

	log.Infof("ChmodContainerdStep initialized: TargetDir=%s, Binaries=%v, Permissions=%s",
		s.TargetDir, s.Binaries, s.Permissions.String())
	return s.BaseStep.Init(rt, log)
}

func (s *ChmodContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting ChmodContainerd step: %s", s.Description())
	ctx := context.Background() // Context for operations

	var wg sync.WaitGroup

	targetHosts := rt.AllHosts()
	if len(targetHosts) == 0 {
		log.Warn("No target hosts specified in runtime. Skipping chmod.")
		return "No target hosts specified, skipping chmod.", true, nil
	}

	processingTracker := struct {
		mu sync.Mutex
		errors []string
		outputs []string
		overallSuccess bool
	} {
		overallSuccess: true,
	}


	for _, hostEntry := range targetHosts {
		wg.Add(1)
		go func(host connector.Host) { // Use connector.Host from the import
			defer wg.Done()
			hostLog := log.WithField("host", host.GetName())

			hostConn, hostRunner, err := rt.GetHostConnectorAndRunner(host)
			if err != nil {
				hostErr := fmt.Errorf("host %s (%s): failed to get connector/runner: %w", host.GetName(), host.GetAddress(), err)
				hostLog.Error(hostErr.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, hostErr.Error())
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

			remoteExec, err := executor.NewRemoteExecutor(hostConn, hostRunner)
			if err != nil {
				hostErr := fmt.Errorf("host %s (%s): failed to create remote executor: %w", host.GetName(), host.GetAddress(), err)
				hostLog.Error(hostErr.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, hostErr.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				return
			}

			for _, binaryName := range s.Binaries {
				remoteBinaryPath := path.Join(s.TargetDir, binaryName)

				hostLog.Infof("Setting permissions %s for %s on host %s", s.Permissions.String(), remoteBinaryPath, host.GetName())

				exists, statErr := remoteExec.RemoteFileExists(ctx, remoteBinaryPath)
				if statErr != nil {
					err := fmt.Errorf("host %s (%s): failed to check existence of %s: %w", host.GetName(), host.GetAddress(), remoteBinaryPath, statErr)
					hostLog.Error(err.Error())
					processingTracker.mu.Lock()
					processingTracker.errors = append(processingTracker.errors, err.Error())
					processingTracker.overallSuccess = false
					processingTracker.mu.Unlock()
					if !rt.IgnoreError() {
						return
					}
					continue
				}
				if !exists {
					err := fmt.Errorf("host %s (%s): binary %s does not exist, cannot chmod", host.GetName(), host.GetAddress(), remoteBinaryPath)
					hostLog.Warn(err.Error())
					processingTracker.mu.Lock()
					processingTracker.errors = append(processingTracker.errors, err.Error())
					processingTracker.mu.Unlock()
					continue
				}

				err = remoteExec.ChmodRemote(ctx, remoteBinaryPath, s.Permissions)
				if err != nil {
					hostErr := fmt.Errorf("host %s (%s): failed to chmod binary %s to %s: %w", host.GetName(), host.GetAddress(), remoteBinaryPath, s.Permissions.String(), err)
					hostLog.Error(hostErr.Error())
					processingTracker.mu.Lock()
					processingTracker.errors = append(processingTracker.errors, hostErr.Error())
					processingTracker.overallSuccess = false
					processingTracker.mu.Unlock()
					if !rt.IgnoreError() {
						return
					}
				} else {
					msg := fmt.Sprintf("Host %s (%s): Successfully set permissions %s for %s", host.GetName(), host.GetAddress(), s.Permissions.String(), remoteBinaryPath)
					hostLog.Info(msg)
					processingTracker.mu.Lock()
					processingTracker.outputs = append(processingTracker.outputs, msg)
					processingTracker.mu.Unlock()
				}
			}
		}(hostEntry)
	}
	wg.Wait()

	finalOutput := strings.Join(processingTracker.outputs, "\n")
	if len(processingTracker.errors) > 0 {
		errorSummary := strings.Join(processingTracker.errors, "\n")
		finalOutput = fmt.Sprintf("Completed with errors.\nOutputs:\n%s\nErrors:\n%s", finalOutput, errorSummary)
		log.Errorf("ChmodContainerd step completed with errors:\n%s", errorSummary)
		return finalOutput, processingTracker.overallSuccess, fmt.Errorf("one or more errors occurred during chmod: %s", errorSummary)
	}

	log.Infof("ChmodContainerd step completed successfully for all processed hosts.")
	return finalOutput, true, nil
}
