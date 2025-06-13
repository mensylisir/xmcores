package common

import (
	"fmt"

	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd" // Assuming this is the correct path
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// RunCommandTaskSpec defines the expected spec for RunCommandTask.
// It can be a single command string or a slice of command strings.
type RunCommandTaskSpec struct {
	Name         string   // Optional name for this specific command task instance
	Description  string   // Optional description
	Commands     []string // List of commands to run
	SingleCommand string   // A single command to run (alternative to Commands)
}

// RunCommandTask executes one or more shell commands.
type RunCommandTask struct {
	task.BaseTask
	// spec RunCommandTaskSpec // Store the parsed spec if needed after Init
}

// NewRunCommandTask creates a new task that executes specified commands.
// Name and description can be provided if this task instance is generic,
// or they can be derived from taskSpec.
func NewRunCommandTask(name, description string) task.Task {
	// Use name and description from spec if provided and non-empty, else use params.
	// This part would be more dynamic if the factory itself took the spec.
	// For now, the task's own Init will handle the spec.
	return &RunCommandTask{
		BaseTask: task.NewBaseTask(name, description),
	}
}

// Init initializes the RunCommandTask.
// taskSpec can be a string (single command) or []string (multiple commands)
// or *RunCommandTaskSpec.
func (t *RunCommandTask) Init(moduleRuntime runtime.Runtime, taskSpec interface{}, logger *logrus.Entry) error {
	// Call BaseTask's Init to store runtime and logger
	// The name and description for BaseTask are already set by NewRunCommandTask.
	// If we want to override them from spec, we'd do it here.
	if err := t.BaseTask.Init(moduleRuntime, taskSpec, logger); err != nil {
		return err
	}

	var commandsToRun []string
	var taskInstanceName = t.Name() // Default to what was set by New...
	var taskInstanceDesc = t.Description()

	switch spec := taskSpec.(type) {
	case string:
		if spec == "" {
			return fmt.Errorf("taskSpec string for RunCommandTask cannot be empty")
		}
		commandsToRun = []string{spec}
		if taskInstanceName == "" { // If NewRunCommandTask was called with empty name
			taskInstanceName = fmt.Sprintf("run-%s", cmdToIdentifier(spec, 10))
		}
	case []string:
		if len(spec) == 0 {
			return fmt.Errorf("taskSpec []string for RunCommandTask cannot be empty")
		}
		commandsToRun = spec
		if taskInstanceName == "" && len(spec) > 0 {
			taskInstanceName = fmt.Sprintf("run-%s", cmdToIdentifier(spec[0], 10))
		}
	case *RunCommandTaskSpec:
		if spec.Name != "" { taskInstanceName = spec.Name }
		if spec.Description != "" { taskInstanceDesc = spec.Description }

		if spec.SingleCommand != "" {
			commandsToRun = append(commandsToRun, spec.SingleCommand)
		}
		if len(spec.Commands) > 0 {
			commandsToRun = append(commandsToRun, spec.Commands...)
		}
		if len(commandsToRun) == 0 {
			return fmt.Errorf("RunCommandTaskSpec must contain at least one command in SingleCommand or Commands")
		}
	default:
		return fmt.Errorf("invalid taskSpec type for RunCommandTask: expected string, []string, or *RunCommandTaskSpec, got %T", taskSpec)
	}

	// Update BaseTask name/description if they were derived from spec and potentially empty before
	if t.BaseTask.NameField == "" && taskInstanceName != "" {
		t.BaseTask.NameField = taskInstanceName
	}
	if t.BaseTask.DescriptionField == "" && taskInstanceDesc != "" {
		t.BaseTask.DescriptionField = taskInstanceDesc
	}


	// For each command, create a RunCommandStep
	for i, cmdStr := range commandsToRun {
		stepName := fmt.Sprintf("%s-step%d-%s", t.Name(), i+1, cmdToIdentifier(cmdStr, 10))
		stepDesc := fmt.Sprintf("Execute command: %s", cmdStr)

		cmdStep := runcmd.NewRunCommandStep(stepName, stepDesc, cmdStr)

		// Initialize the step (passing runtime and scoped logger from BaseTask)
		// BaseTask.runtime and BaseTask.logger are now set.
		stepLogger := t.BaseTask.logger.WithField("step_name", cmdStep.Name())
		if err := cmdStep.Init(t.BaseTask.runtime, stepLogger); err != nil {
			return fmt.Errorf("failed to initialize step '%s' for command '%s': %w", stepName, cmdStr, err)
		}
		t.AddStep(cmdStep) // Add initialized step to BaseTask
	}

	t.BaseTask.logger.Infof("RunCommandTask initialized with %d command(s).", len(commandsToRun))
	return nil
}

// cmdToIdentifier creates a short identifier from a command string for naming steps.
func cmdToIdentifier(cmd string, maxLength int) string {
	fs := strings.Fields(cmd)
	if len(fs) > 0 {
		name := strings.ToLower(fs[0])
		name = strings.ReplaceAll(name, "/", "_") // Avoid slashes in names
		name = strings.ReplaceAll(name, ".", "_")
		if len(name) > maxLength {
			return name[:maxLength]
		}
		return name
	}
	return "cmd"
}

// Execute method is inherited from BaseTask.
// var _ task.Task = (*RunCommandTask)(nil) // Ensured by BaseTask embedding.
