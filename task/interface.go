package task

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"   // Assuming module path
	"github.com/sirupsen/logrus"
)

// Task represents a collection of steps to achieve a significant goal,
// like installing a piece of software.
type Task interface {
	// Name returns the short name of the task.
	Name() string

	// Description returns a human-readable description of what the task does.
	Description() string

	// Steps returns the list of steps that this task will execute.
	Steps() []step.Step

	// Init performs any initialization or validation required before execution.
	// This typically involves initializing all its steps.
	// The logger entry is pre-configured with task context.
	Init(rt runtime.Runtime, logger *logrus.Entry) error

	// Execute performs the primary actions of the task by executing its steps.
	// Returns an error if the task execution failed critically.
	// The logger entry is pre-configured with task context.
	Execute(rt runtime.Runtime, logger *logrus.Entry) error

	// Post performs any cleanup or final actions after Execute has completed.
	// It receives the error (if any) from the Execute phase.
	// The logger entry is pre-configured with task context.
	Post(rt runtime.Runtime, logger *logrus.Entry, executeErr error) error
}
