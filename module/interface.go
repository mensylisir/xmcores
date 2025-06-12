package module

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"   // Assuming module path
	"github.com/sirupsen/logrus"
)

// Module represents a collection of tasks that form a larger functional unit,
// e.g., "RuntimeInstallationModule" which might include tasks for installing containerd, docker, etc.
type Module interface {
	// Name returns the short name of the module.
	Name() string

	// Description returns a human-readable description of what the module does.
	Description() string

	// Tasks returns the list of tasks that this module will execute.
	Tasks() []task.Task

	// Init performs any initialization or validation required before execution.
	// This typically involves initializing all its tasks.
	// The logger entry is pre-configured with module context.
	Init(rt runtime.Runtime, logger *logrus.Entry) error

	// Execute performs the primary actions of the module by executing its tasks.
	// Returns an error if the module execution failed critically.
	// The logger entry is pre-configured with module context.
	Execute(rt runtime.Runtime, logger *logrus.Entry) error

	// Post performs any cleanup or final actions after Execute has completed.
	// It receives the error (if any) from the Execute phase.
	// The logger entry is pre-configured with module context.
	Post(rt runtime.Runtime, logger *logrus.Entry, executeErr error) error
}
