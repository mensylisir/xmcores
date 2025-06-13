package task

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step" // Retained for step management
	"github.com/sirupsen/logrus"
)

// Task represents a specific unit of work, potentially as part of a Module,
// composed of multiple steps.
type Task interface {
	// Name returns the unique name of the task.
	Name() string

	// Description provides a human-readable summary of what the task does.
	Description() string

	// Init prepares the task for execution.
	// - moduleRuntime: The runtime environment scoped from the parent module.
	// - taskSpec: Configuration specific to this task, passed from the module.
	// - logger: A logger entry pre-configured with module and task context.
	// This method is where steps should be added to the task using AddStep.
	Init(moduleRuntime runtime.Runtime, taskSpec interface{}, logger *logrus.Entry) error

	// Execute runs the main logic of the task, typically by executing its defined steps.
	// It should use the runtime and configurations prepared during Init.
	// - logger: A logger entry pre-configured for this task's execution phase.
	Execute(logger *logrus.Entry) error // Note: BaseTask.Execute takes runtime from its stored state after Init.

	// AddStep allows adding a step to the task's execution sequence, typically called during Init.
	AddStep(s step.Step)

	// Steps returns the list of steps configured for this task.
	// Used by BaseTask's Execute method to iterate through steps.
	Steps() []step.Step
}
