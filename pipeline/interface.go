package pipeline

import (
	"github.com/mensylisir/xmcores/module"  // Assuming module path
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// Pipeline represents the highest level of execution, orchestrating a series of modules
// to achieve a complex end-to-end goal, e.g., "DeployKubernetesClusterPipeline".
type Pipeline interface {
	// Name returns the short name of the pipeline.
	Name() string

	// Description returns a human-readable description of what the pipeline does.
	Description() string

	// Modules returns the list of modules that this pipeline will execute.
	Modules() []module.Module

	// Init performs any initialization or validation required before execution.
	// This typically involves initializing all its modules.
	// The logger entry is pre-configured with pipeline context.
	Init(rt runtime.Runtime, logger *logrus.Entry) error

	// Execute performs the primary actions of the pipeline by executing its modules.
	// Returns an error if the pipeline execution failed critically.
	// The logger entry is pre-configured with pipeline context.
	Execute(rt runtime.Runtime, logger *logrus.Entry) error

	// Post performs any cleanup or final actions after Execute has completed.
	// It receives the error (if any) from the Execute phase.
	// The logger entry is pre-configured with pipeline context.
	Post(rt runtime.Runtime, logger *logrus.Entry, executeErr error) error
}
