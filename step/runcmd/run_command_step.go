package runcmd

import (
	"context"
	"fmt"

	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// RunCommandStep defines a step that executes a shell command on the target.
type RunCommandStep struct {
	step.BaseStep
	Command string
}

// NewRunCommandStep creates a new RunCommandStep.
func NewRunCommandStep(name, description, command string) *RunCommandStep {
	return &RunCommandStep{
		BaseStep: step.BaseStep{
			StepName:        name,
			StepDescription: description,
		},
		Command: command,
	}
}

// Init performs any initialization or validation required before execution.
func (s *RunCommandStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	log.Debugf("Initializing RunCommandStep: %s", s.Name())
	if s.Command == "" {
		return fmt.Errorf("command string cannot be empty for RunCommandStep: %s", s.Name())
	}
	// Potentially validate access to the host here, though Execute will do it too.
	if len(rt.AllHosts()) == 0 {
		return fmt.Errorf("no hosts configured in runtime for step %s", s.Name())
	}
	return nil
}

// Execute runs the command on the target host.
// It assumes a single host scenario for fetching the runner, as per current design.
func (s *RunCommandStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Executing RunCommandStep: %s (Command: '%s')", s.Name(), s.Command)

	if len(rt.AllHosts()) == 0 {
		return "", false, fmt.Errorf("no hosts configured in runtime for step %s", s.Name())
	}
	targetHost := rt.AllHosts()[0] // Assuming the first host

	_, runner, err := rt.GetHostConnectorAndRunner(targetHost)
	if err != nil {
		return "", false, fmt.Errorf("failed to get runner for host %s for step %s: %w", targetHost.GetName(), s.Name(), err)
	}

	log.Debugf("Executing command: '%s' on host %s", s.Command, targetHost.GetName())
	// Using context.Background() for now. Consider making context configurable or passed down.
	stdout, stderr, exitCode, execErr := runner.Run(context.Background(), s.Command)

	if execErr != nil {
		log.Errorf("Command execution error for step %s on host %s: %v. Exit code: %d. Stderr: %s. Stdout: %s",
			s.Name(), targetHost.GetName(), execErr, exitCode, stderr, stdout)
		// Return stdout as output even on error, as it might contain useful info
		return stdout, false, fmt.Errorf("command '%s' execution error on host %s: %w (exit code: %d, stderr: %s)",
			s.Command, targetHost.GetName(), execErr, exitCode, stderr)
	}

	if exitCode != 0 {
		log.Warnf("Command execution non-zero exit code for step %s on host %s: %d. Stderr: %s. Stdout: %s",
			s.Name(), targetHost.GetName(), exitCode, stderr, stdout)
		// Return stdout as output, but mark as failure due to non-zero exit code
		return stdout, false, fmt.Errorf("command '%s' failed on host %s with exit code %d (stderr: %s)",
			s.Command, targetHost.GetName(), exitCode, stderr)
	}

	log.Infof("Command execution successful for step %s on host %s. Output:\n%s", s.Name(), targetHost.GetName(), stdout)
	if stderr != "" {
		log.Debugf("Stderr from command execution for step %s on host %s:\n%s", s.Name(), targetHost.GetName(), stderr)
	}
	return stdout, true, nil
}

// Post performs any cleanup or final actions after Execute has completed.
func (s *RunCommandStep) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
	log.Debugf("Post-execution for RunCommandStep: %s", s.Name())
	if executeErr != nil {
		log.Warnf("RunCommandStep %s completed with error: %v", s.Name(), executeErr)
	} else {
		log.Infof("RunCommandStep %s completed successfully.", s.Name())
	}
	return nil // No specific cleanup action for this step
}

// Ensure RunCommandStep implements the Step interface
var _ step.Step = (*RunCommandStep)(nil)
