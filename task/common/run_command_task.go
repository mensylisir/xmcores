package common

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/pipeline/ending"
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
	task.BaseTask // Embed BaseTask
	// taskSpec is inherited from BaseTask and will be type-asserted
}

// NewRunCommandTask creates a new task that executes specified commands.
// The actual commands are provided via taskSpec to the Init/Default methods.
// Name and description can be default and overridden by spec.
func NewRunCommandTask(name, description string) task.Task {
	t := &RunCommandTask{}
	t.NameField = name
	t.DescriptionField = description
	t.steps = make([]task.Step, 0) // Initialize steps slice from BaseTask
	return t
}

// Default stores runtime, logger, and the taskSpec.
func (t *RunCommandTask) Default(runtime *krt.KubeRuntime, moduleCache interface{}, taskCache interface{}) error {
	if err := t.BaseTask.Default(runtime, moduleCache, taskCache); err != nil {
		return err
	}
	// Logger and Runtime are set by BaseTask.Default
	// Concrete task needs to set its own logger for proper scoping if BaseTask.Default doesn't.
	// The current BaseTask.Default sets a basic logger. We can refine it here.
	t.Logger = runtime.Log.WithFields(logrus.Fields{"task": t.Name(), "type": "RunCommandTask"})
	t.Logger.Info("RunCommandTask Default completed.")
	return nil
}

// Init processes the taskSpec to create and initialize steps.
// It's called by the Module after Default and AutoAssert.
func (t *RunCommandTask) Init() error {
	// Call BaseTask's Init (currently a placeholder, but good practice)
	if err := t.BaseTask.Init(); err != nil {
		return err
	}
	t.Logger.Info("RunCommandTask Init called - assembling steps.")

	if t.TaskSpec == nil {
		return fmt.Errorf("taskSpec not set for RunCommandTask '%s' (must be set in module's Init after task's Default)", t.Name())
	}

	var commandsToRun []string
	taskInstanceName := t.Name()
	taskInstanceDesc := t.Description()

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
		return fmt.Errorf("invalid taskSpec type for RunCommandTask: expected string, []string, or *RunCommandTaskSpec, got %T", t.TaskSpec)
	}

	// Update NameField and DescriptionField if they were derived or set by spec
	t.NameField = taskInstanceName
	t.DescriptionField = taskInstanceDesc
	// Re-scope logger if name changed
	t.Logger = t.Runtime.Log.WithFields(logrus.Fields{"task": t.Name(), "type": "RunCommandTask"})


	for i, cmdStr := range commandsToRun {
		stepName := fmt.Sprintf("%s-step%d-%s", t.Name(), i+1, cmdToIdentifier(cmdStr, 10))
		stepDesc := fmt.Sprintf("Execute command: %s", cmdStr)

		cmdStep := runcmd.NewRunCommandStep(stepName, stepDesc, cmdStr)

		stepLogger := t.Logger.WithField("step", cmdStep.Name())
		if err := cmdStep.Init(t.Runtime, stepLogger); err != nil { // Pass runtime from BaseTask
			return fmt.Errorf("failed to initialize step '%s' for command '%s': %w", stepName, cmdStr, err)
		}
		t.AddStep(cmdStep)
	}

	t.Logger.Infof("RunCommandTask initialized with %d command(s).", len(commandsToRun))
	return nil
}

// cmdToIdentifier creates a short identifier from a command string.
func cmdToIdentifier(cmd string, maxLength int) string {
	fs := strings.Fields(cmd)
	if len(fs) > 0 {
		name := strings.ToLower(fs[0])
		name = strings.ReplaceAll(name, "/", "_")
		name = strings.ReplaceAll(name, ".", "_")
		if len(name) > maxLength {
			return name[:maxLength]
		}
		return name
	}
	return "cmd"
}

// Slogan provides a specific slogan for RunCommandTask.
func (t *RunCommandTask) Slogan() string {
	return fmt.Sprintf("Executing command(s) for task: %s...", t.Name())
}

// Run method is inherited from BaseTask.
// Other methods like IsSkip, AutoAssert, Until, Steps, AddStep are inherited or overridden if needed.
// Ensure this struct satisfies the task.Task interface by virtue of BaseTask + any overrides.
var _ task.Task = (*RunCommandTask)(nil)
