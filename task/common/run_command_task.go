package common

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/pipeline/ending" // For ModuleResult if Run is overridden
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// RunCommandTaskSpec defines the expected spec for RunCommandTask.
type RunCommandTaskSpec struct {
	Name         string   // Optional name for this specific command task instance
	Description  string   // Optional description
	Commands     []string // List of commands to run
	SingleCommand string   // A single command to run (alternative to Commands)
}

// RunCommandTask executes one or more shell commands.
type RunCommandTask struct {
	*task.BaseTask // Embed pointer to BaseTask
	// Store the processed commands if needed after Init, though BaseTask.steps will hold them.
	// commandsToExecute []string
}

// NewRunCommandTask creates a new task that executes specified commands.
func NewRunCommandTask(name, description string) task.Task {
	// Initialize BaseTask with name and description
	// These might be overridden in Default/Init if spec provides them.
	base := task.NewBaseTask(name, description)
	return &RunCommandTask{
		BaseTask: base,
	}
}

// Default stores runtime, logger, and the taskSpec.
func (t *RunCommandTask) Default(runtime *krt.ClusterRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	// Call BaseTask's Default to store runtime and raw taskSpec
	if err := t.BaseTask.Default(runtime, taskSpec, moduleCache, taskCache); err != nil {
		return err
	}
	// Set up a logger scoped for this specific task instance, overriding the one from BaseTask.Default
	t.Logger = runtime.Log.WithFields(logrus.Fields{"task": t.NameField, "type": "RunCommandTask"})
	t.Logger.Info("RunCommandTask Default completed: runtime, taskSpec, and logger set.")
	return nil
}

// Init processes the stored taskSpec to create and initialize steps.
func (t *RunCommandTask) Init() error {
	// Call BaseTask's Init (currently a placeholder, but good practice for future)
	if err := t.BaseTask.Init(); err != nil {
		return err
	}
	t.Logger.Info("RunCommandTask Init called - assembling steps.")

	if t.TaskSpec == nil {
		return fmt.Errorf("taskSpec not set for RunCommandTask '%s' (must be set in Default)", t.NameField)
	}

	var commandsToRun []string
	// Use NameField from BaseTask, which might have been set by New or from spec.
	taskInstanceName := t.NameField
	taskInstanceDesc := t.DescriptionField

	switch spec := t.TaskSpec.(type) {
	case string:
		if spec == "" {
			return fmt.Errorf("taskSpec string for RunCommandTask cannot be empty")
		}
		commandsToRun = []string{spec}
		if taskInstanceName == "" { taskInstanceName = fmt.Sprintf("run-%s", cmdToIdentifier(spec, 10)) }
	case []string:
		if len(spec) == 0 {
			return fmt.Errorf("taskSpec []string for RunCommandTask cannot be empty")
		}
		commandsToRun = spec
		if taskInstanceName == "" && len(spec) > 0 { taskInstanceName = fmt.Sprintf("run-%s", cmdToIdentifier(spec[0], 10)) }
	case *RunCommandTaskSpec:
		if spec.Name != "" { taskInstanceName = spec.Name }
		if spec.Description != "" { taskInstanceDesc = spec.Description }
		if spec.SingleCommand != "" { commandsToRun = append(commandsToRun, spec.SingleCommand) }
		if len(spec.Commands) > 0 { commandsToRun = append(commandsToRun, spec.Commands...) }
		if len(commandsToRun) == 0 {
			return fmt.Errorf("RunCommandTaskSpec must contain at least one command")
		}
	default:
		return fmt.Errorf("invalid taskSpec type for RunCommandTask '%s': expected string, []string, or *RunCommandTaskSpec, got %T", t.NameField, t.TaskSpec)
	}

	// Update NameField and DescriptionField if they were derived or set by spec
	t.NameField = taskInstanceName
	t.DescriptionField = taskInstanceDesc
	// Re-scope logger if name changed
	t.Logger = t.Runtime.Log.WithFields(logrus.Fields{"task": t.NameField, "type": "RunCommandTask"})


	for i, cmdStr := range commandsToRun {
		stepName := fmt.Sprintf("%s-step%d-%s", t.NameField, i+1, cmdToIdentifier(cmdStr, 10))
		stepDesc := fmt.Sprintf("Execute command: '%s'", cmdStr)

		cmdStep := runcmd.NewRunCommandStep(stepName, stepDesc, cmdStr)

		stepLogger := t.Logger.WithField("step", cmdStep.Name())
		// Initialize the step using the Runtime stored in BaseTask
		if err := cmdStep.Init(t.Runtime, stepLogger); err != nil {
			return fmt.Errorf("failed to initialize step '%s' for command '%s': %w", stepName, cmdStr, err)
		}
		t.AddStep(cmdStep) // AddStep is a method of BaseTask
	}

	t.Logger.Infof("RunCommandTask initialized with %d command(s).", len(commandsToRun))
	return nil
}

// cmdToIdentifier creates a short identifier from a command string for naming steps.
func cmdToIdentifier(cmd string, maxLength int) string {
	fs := strings.Fields(cmd)
	if len(fs) > 0 {
		name := strings.ToLower(fs[0])
		// Basic sanitization, can be expanded
		name = strings.ReplaceAll(name, "/", "_")
		name = strings.ReplaceAll(name, ".", "_")
		name = strings.ReplaceAll(name, ":", "_")
		if len(name) > maxLength {
			return name[:maxLength]
		}
		return name
	}
	return "cmd"
}

// Slogan provides a specific slogan for RunCommandTask.
// BaseTask.Slogan() can be overridden if a more specific message is desired.
// func (t *RunCommandTask) Slogan() string {
// 	return fmt.Sprintf("Executing command(s) for task: %s...", t.NameField)
// }

// Run method is inherited from BaseTask.
// IsSkip, AutoAssert, Until, Steps are inherited from BaseTask and can be overridden if needed.

var _ task.Task = (*RunCommandTask)(nil) // Verify interface implementation
