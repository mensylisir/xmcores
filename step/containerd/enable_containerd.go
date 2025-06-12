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

// EnableContainerdStep enables the containerd systemd service.
type EnableContainerdStep struct {
	step.BaseStep
	ServiceName   string // Name of the service, e.g., "containerd.service"
	ReloadSystemd bool   // Whether to run 'systemctl daemon-reload' before enabling
	EnableNow     bool   // Whether to use 'systemctl enable --now'
}

// NewEnableContainerdStep creates a new EnableContainerdStep.
func NewEnableContainerdStep(reloadSystemd, enableNow bool) step.Step {
	return &EnableContainerdStep{
		BaseStep:      step.NewBaseStep("EnableContainerd", "Enable and optionally start Containerd service"),
		ServiceName:   "containerd.service", // Default service name
		ReloadSystemd: reloadSystemd,
		EnableNow:     enableNow,
	}
}

func (s *EnableContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.ServiceName == "" {
		return fmt.Errorf("service name cannot be empty for EnableContainerdStep")
	}
	log.Infof("EnableContainerdStep initialized: ServiceName=%s, ReloadSystemd=%t, EnableNow=%t",
		s.ServiceName, s.ReloadSystemd, s.EnableNow)
	return s.BaseStep.Init(rt, log)
}

func (s *EnableContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting EnableContainerd step: %s (ReloadSystemd: %t, EnableNow: %t)", s.Description(), s.ReloadSystemd, s.EnableNow)
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
		log.Warn("No target hosts specified in runtime. Skipping service enable.")
		return "No target hosts specified, skipping service enable.", true, nil
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

			// Reload systemd if requested
			if s.ReloadSystemd {
				hostLog.Infof("Host %s (%s): Running 'systemctl daemon-reload'", host.GetName(), host.GetAddress())
				reloadCmd := "systemctl daemon-reload"
				stdout, stderr, exitCode, execErr := remoteExec.SudoExecute(ctx, reloadCmd)
				// Capture output regardless of error for better debugging
				cmdOutput := fmt.Sprintf("Host %s (%s) daemon-reload stdout:\n%s\nstderr:\n%s", host.GetName(), host.GetAddress(), stdout, stderr)

				processingTracker.mu.Lock()
				processingTracker.outputs = append(processingTracker.outputs, cmdOutput)
				processingTracker.mu.Unlock()

				if execErr != nil {
					err := fmt.Errorf("host %s (%s): 'systemctl daemon-reload' execution failed: %w. Output:\n%s", host.GetName(), host.GetAddress(), execErr, cmdOutput)
					hostLog.Error(err.Error())
					processingTracker.mu.Lock()
					processingTracker.errors = append(processingTracker.errors, err.Error())
					processingTracker.overallSuccess = false
					processingTracker.mu.Unlock()
					if !rt.IgnoreError() { return }
				} else if exitCode != 0 {
					err := fmt.Errorf("host %s (%s): 'systemctl daemon-reload' failed with exit code %d. Output:\n%s", host.GetName(), host.GetAddress(), exitCode, cmdOutput)
					hostLog.Error(err.Error())
					processingTracker.mu.Lock()
					processingTracker.errors = append(processingTracker.errors, err.Error())
					processingTracker.overallSuccess = false
					processingTracker.mu.Unlock()
					if !rt.IgnoreError() { return }
				} else {
					hostLog.Infof("Host %s (%s): 'systemctl daemon-reload' completed successfully.", host.GetName(), host.GetAddress())
				}
			}

			// Continue only if daemon-reload was successful (or not run, or IgnoreError is true)
			// This check needs to be inside the goroutine, and consider processingTracker.overallSuccess for this host.
			// However, if ReloadSystemd failed and IgnoreError is true, we might still want to try enabling.
			// The current logic correctly appends errors and sets overallSuccess to false.
			// We'll proceed to enable/start unless IgnoreError is false and a previous step for this host failed.
			// The `return` statements after setting overallSuccess=false handle the !rt.IgnoreError() case.

			enableCmd := fmt.Sprintf("systemctl enable %s", s.ServiceName)
			if s.EnableNow {
				enableCmd = fmt.Sprintf("systemctl enable --now %s", s.ServiceName)
			}
			hostLog.Infof("Host %s (%s): Running '%s'", host.GetName(), host.GetAddress(), enableCmd)

			stdout, stderr, exitCode, execErr := remoteExec.SudoExecute(ctx, enableCmd)
			cmdOutput := fmt.Sprintf("Host %s (%s) '%s' stdout:\n%s\nstderr:\n%s", host.GetName(), host.GetAddress(), enableCmd, stdout, stderr)

			processingTracker.mu.Lock()
			processingTracker.outputs = append(processingTracker.outputs, cmdOutput)
			processingTracker.mu.Unlock()

			if execErr != nil {
				err := fmt.Errorf("host %s (%s): command '%s' execution failed: %w. Output:\n%s", host.GetName(), host.GetAddress(), enableCmd, execErr, cmdOutput)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
			} else if exitCode != 0 {
				err := fmt.Errorf("host %s (%s): command '%s' failed with exit code %d. Output:\n%s", host.GetName(), host.GetAddress(), enableCmd, exitCode, cmdOutput)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
			} else {
				msg := fmt.Sprintf("Host %s (%s): Command '%s' completed successfully.", host.GetName(), host.GetAddress(), enableCmd)
				hostLog.Info(msg)
				// No need to append to outputs here as cmdOutput already added
			}
		}(hostEntry)
	}
	wg.Wait()

	finalOutput := strings.Join(processingTracker.outputs, "\n")
	// Check overallSuccess which is modified by goroutines
	if !processingTracker.overallSuccess {
		errorSummary := ""
		if len(processingTracker.errors) > 0 { // Only join errors if there are any
			errorSummary = strings.Join(processingTracker.errors, "\n")
			finalOutput = fmt.Sprintf("Completed with errors.\nOutputs:\n%s\nErrors:\n%s", finalOutput, errorSummary)
			log.Errorf("EnableContainerd step completed with errors:\n%s", errorSummary)
			return finalOutput, false, fmt.Errorf("one or more errors occurred during service enable: %s", errorSummary)
		}
		// If overallSuccess is false but no specific errors were logged (should not happen with current logic)
		log.Error("EnableContainerd step failed with unreported errors.")
		return finalOutput, false, fmt.Errorf("enable containerd step failed with unreported errors")
	}

	log.Infof("EnableContainerd step completed successfully for all processed hosts.")
	return finalOutput, true, nil
}
