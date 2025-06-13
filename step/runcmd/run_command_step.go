package runcmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/connector"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// RunCommandStep defines a step that executes a shell command on a target.
type RunCommandStep struct {
	*step.BaseStep                 // Embed pointer to BaseStep
	Command        string
	TargetHosts    []connector.Host // Optional: if step needs to run on specific subset of hosts
	                             // If empty, might run on a "default" or first host from runtime.
}

// NewRunCommandStep creates a new RunCommandStep.
func NewRunCommandStep(name, description, command string) *RunCommandStep {
	return &RunCommandStep{
		BaseStep: step.NewBaseStep(name, description), // Initialize embedded BaseStep
		Command:  command,
		// TargetHosts can be set by the Task after instantiation if needed
	}
}

// Init initializes the RunCommandStep.
func (s *RunCommandStep) Init(rt *krt.ClusterRuntime, logger *logrus.Entry) error {
	// Call BaseStep's Init to set up Runtime and Logger
	if err := s.BaseStep.Init(rt, logger.WithField("step", s.NameField)); err != nil {
		return err
	}

	if strings.TrimSpace(s.Command) == "" {
		return fmt.Errorf("command string cannot be empty for RunCommandStep: %s", s.NameField)
	}
	s.Logger.Infof("RunCommandStep Initialized. Command: '%s'. TargetHosts count (if set): %d", s.Command, len(s.TargetHosts))
	return nil
}

// Execute runs the command on the target host(s).
func (s *RunCommandStep) Execute(rt *krt.ClusterRuntime, logger *logrus.Entry) (output string, success bool, err error) {
	// Use the logger from BaseStep, which should have been scoped in Init
	execLogger := s.Logger
	if execLogger == nil { // Fallback if BaseStep.Init wasn't called or failed to set logger
		execLogger = logger.WithField("step_execute_fallback", s.NameField)
		execLogger.Warn("Logger not found from BaseStep, using fallback for Execute.")
	}
	if rt == nil { // Runtime should be available from BaseStep
		rt = s.Runtime
	}
	if rt == nil {
		return "", false, fmt.Errorf("runtime not available for step %s", s.NameField)
	}

	execLogger.Infof("Executing RunCommandStep: %s (Command: '%s')", s.NameField, s.Command)

	var hostsToRunOn []connector.Host
	if len(s.TargetHosts) > 0 {
		hostsToRunOn = s.TargetHosts
		execLogger.Debugf("Executing on %d specifically targeted host(s).", len(s.TargetHosts))
	} else if len(rt.GetAllHosts()) > 0 {
		// Fallback: run on the first host from the runtime if no specific target hosts are set.
		// This is a placeholder; real scenarios might require more sophisticated host selection
		// or tasks should explicitly set TargetHosts.
		hostsToRunOn = []connector.Host{rt.GetAllHosts()[0]}
		execLogger.Warnf("No TargetHosts set for RunCommandStep '%s'. Executing on first available host: %s (%s) as a default. This may not be the desired behavior for all use cases.",
			s.NameField, hostsToRunOn[0].GetName(), hostsToRunOn[0].GetAddress())
	} else {
		return "", false, fmt.Errorf("no target hosts specified and no hosts available in runtime for step %s", s.NameField)
	}

	var outputs []string
	var errors []string
	overallSuccess := true

	for _, host := range hostsToRunOn {
		hostLogger := execLogger.WithFields(logrus.Fields{
			"target_host_name":    host.GetName(),
			"target_host_address": host.GetAddress(),
		})
		hostLogger.Debugf("Attempting to get connector for host %s", host.GetName())

		conn, connErr := rt.GetConnector(host)
		if connErr != nil {
			errText := fmt.Sprintf("Failed to get connector for host %s: %v", host.GetName(), connErr)
			hostLogger.Error(errText)
			errors = append(errors, errText)
			overallSuccess = false
			if !rt.Arg.IgnoreErr { return strings.Join(outputs, "\n---\n"), false, fmt.Errorf(strings.Join(errors, "; ")) }
			continue // Try next host if ignoring errors
		}

		hostLogger.Debugf("Executing command: '%s' on host %s", s.Command, host.GetName())
		// Using context.Background() for now. Consider making context configurable.
		stdout, stderr, exitCode, execErr := conn.Exec(context.Background(), s.Command)

		currentOutput := fmt.Sprintf("Host: %s\nStdout:\n%s", host.GetName(), string(stdout))
		if stderr != "" {
			currentOutput += fmt.Sprintf("\nStderr:\n%s", string(stderr))
		}
		outputs = append(outputs, currentOutput)

		if execErr != nil {
			errText := fmt.Sprintf("Command execution error on host %s: %v (ExitCode: %d)", host.GetName(), execErr, exitCode)
			hostLogger.Error(errText)
			errors = append(errors, errText)
			overallSuccess = false
			if !rt.Arg.IgnoreErr { return strings.Join(outputs, "\n---\n"), false, fmt.Errorf(strings.Join(errors, "; ")) }
			continue
		}
		if exitCode != 0 {
			errText := fmt.Sprintf("Command on host %s exited with code %d", host.GetName(), exitCode)
			hostLogger.Error(errText)
			errors = append(errors, errText)
			overallSuccess = false
			if !rt.Arg.IgnoreErr { return strings.Join(outputs, "\n---\n"), false, fmt.Errorf(strings.Join(errors, "; ")) }
			continue
		}
		hostLogger.Infof("Command execution successful on host %s.", host.GetName())
	}

	finalOutput := strings.Join(outputs, "\n===\n")
	if !overallSuccess {
		return finalOutput, false, fmt.Errorf("one or more command executions failed: %s", strings.Join(errors, "; "))
	}

	execLogger.Infof("RunCommandStep %s completed successfully on all targeted hosts.", s.NameField)
	return finalOutput, true, nil
}

// Post calls the BaseStep's Post method.
func (s *RunCommandStep) Post(rt *krt.ClusterRuntime, logger *logrus.Entry, stepExecuteErr error) error {
	// Use the logger from BaseStep, which should have been scoped in Init
	postLogger := s.Logger
	if postLogger == nil {
		postLogger = logger.WithField("step_post_fallback", s.NameField)
	}
	if rt == nil { rt = s.Runtime } // Fallback

	postLogger.Infof("RunCommandStep Post for step [%s]. Execute error (if any): %v", s.NameField, stepExecuteErr)
	// Add any specific post-command logic here if needed.
	// Then call BaseStep's Post.
	return s.BaseStep.Post(rt, logger, stepExecuteErr) // Pass original logger for BaseStep's context
}

// Ensure RunCommandStep implements the step.Step interface.
var _ step.Step = (*RunCommandStep)(nil)
