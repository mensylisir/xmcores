package step

import (
	"github.com/mensylisir/xmcores/runtime" // Assuming module path
	"github.com/sirupsen/logrus"         // For structured logging
)

// Step represents an individual unit of work within a Task.
type Step interface {
	// Name returns the short name of the step.
	Name() string

	// Description returns a human-readable description of what the step does.
	Description() string

	// Init performs any initialization or validation required before execution.
	// It can use the runtime to access configuration or context.
	Init(rt runtime.Runtime, logger *logrus.Entry) error

	// Execute performs the primary action of the step.
	// It returns an output string (e.g., command output, summary), a boolean indicating success,
	// and an error if the execution failed critically.
	// The logger entry is pre-configured with step context.
	Execute(rt runtime.Runtime, logger *logrus.Entry) (output string, success bool, err error)

	// Post performs any cleanup or final actions after Execute has completed.
	// It receives the error (if any) from the Execute phase.
	// The logger entry is pre-configured with step context.
	Post(rt runtime.Runtime, logger *logrus.Entry, executeErr error) error
}
