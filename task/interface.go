package task

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// Task represents a specific unit of work, potentially as part of a Module.
// It receives its configuration from its calling context (e.g., a Module).
type Task interface {
	// Name returns the unique name of the task.
	Name() string

	// Description provides a human-readable summary of what the task does.
	Description() string

	// Execute runs the task's logic.
	// - rt: The runtime environment.
	// - taskConfig: A map containing configuration specific to this task execution.
	// - logger: A logger entry for structured logging.
	// It returns an error if the task execution fails.
	Execute(rt runtime.Runtime, taskConfig map[string]interface{}, logger *logrus.Entry) error
}
