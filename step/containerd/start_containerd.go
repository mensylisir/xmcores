package containerd

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mensylisir/xmcores/connector" // Required for connector.Host
	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// StartContainerdStep starts the containerd systemd service.
type StartContainerdStep struct {
	step.BaseStep
	ServiceName string // Name of the service, e.g., "containerd.service"
}

// NewStartContainerdStep creates a new StartContainerdStep.
func NewStartContainerdStep() step.Step {
	return &StartContainerdStep{
		BaseStep:    step.NewBaseStep("StartContainerd", "Start Containerd service"),
		ServiceName: "containerd.service", // Default service name
	}
}

func (s *StartContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.ServiceName == "" {
		return fmt.Errorf("service name cannot be empty for StartContainerdStep")
	}
	log.Infof("StartContainerdStep initialized: ServiceName=%s", s.ServiceName)
	return s.BaseStep.Init(rt, log)
}

func (s *StartContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting StartContainerd step: %s", s.Description())
	ctx := context.Background()

	processingTracker := struct {
		mu             sync.Mutex
		errors         []string
		outputs        []string
		overallSuccess bool
	}{
		overallSuccess: true, // Initialize assuming success
	}
	var wg sync.WaitGroup

	targetHosts := rt.AllHosts()
	if len(targetHosts) == 0 {
		log.Warn("No target hosts specified in runtime. Skipping service start.")
		return "No target hosts specified, skipping service start.", true, nil
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


			remoteExec, err := executor.NewRemoteExecutor(hostConn, hostRunner)
			if err != nil {
				err := fmt.Errorf("host %s (%s): failed to create remote executor: %w", host.GetName(), host.GetAddress(), err)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				return
			}

			startCmd := fmt.Sprintf("systemctl start %s", s.ServiceName)
			hostLog.Infof("Host %s (%s): Running '%s'", host.GetName(), host.GetAddress(), startCmd)

			stdout, stderr, exitCode, execErr := remoteExec.SudoExecute(ctx, startCmd)
			cmdOutput := fmt.Sprintf("Host %s (%s) '%s' stdout:\n%s\nstderr:\n%s", host.GetName(), host.GetAddress(), startCmd, stdout, stderr)

			processingTracker.mu.Lock()
			processingTracker.outputs = append(processingTracker.outputs, cmdOutput)
			processingTracker.mu.Unlock()

			if execErr != nil {
				err := fmt.Errorf("host %s (%s): command '%s' execution failed: %w. Output:\n%s", host.GetName(), host.GetAddress(), startCmd, execErr, cmdOutput)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
			} else if exitCode != 0 {
				err := fmt.Errorf("host %s (%s): command '%s' failed with exit code %d. Output:\n%s", host.GetName(), host.GetAddress(), startCmd, exitCode, cmdOutput)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
			} else {
				msg := fmt.Sprintf("Host %s (%s): Command '%s' completed successfully.", host.GetName(), host.GetAddress(), startCmd)
				hostLog.Info(msg)
			}
		}(hostEntry)
	}
	wg.Wait()

	finalOutput := strings.Join(processingTracker.outputs, "\n")
	if !processingTracker.overallSuccess {
		errorSummary := "" // Initialize errorSummary
		if len(processingTracker.errors) > 0 {
			errorSummary = strings.Join(processingTracker.errors, "\n")
			finalOutput = fmt.Sprintf("Completed with errors.\nOutputs:\n%s\nErrors:\n%s", finalOutput, errorSummary)
		}
		log.Errorf("StartContainerd step completed with errors:\n%s", errorSummary)
		if len(processingTracker.errors) > 0 {
			return finalOutput, false, fmt.Errorf("one or more errors occurred during service start: %s", errorSummary)
		}
		// This case should ideally not be reached if overallSuccess is false, as an error should have been added.
		return finalOutput, false, fmt.Errorf("start containerd step failed with unreported errors")
	}

	log.Infof("StartContainerd step completed successfully for all processed hosts.")
	return finalOutput, true, nil
}
