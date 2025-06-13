package pipeline

import (
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	"github.com/mensylisir/xmcores/runtime" // For ClusterRuntime in PipelineFactory
	"github.com/sirupsen/logrus"
)

// PipelineFactory defines the function signature for creating pipeline instances.
// It accepts ClusterRuntime to allow pipelines to be initialized with their specific runtime context and config.
type PipelineFactory func(cr *runtime.ClusterRuntime) (Pipeline, error)

// Pipeline represents a high-level workflow that orchestrates a series of modules.
type Pipeline interface {
	// Name returns the unique name of the pipeline.
	Name() string

	// Description provides a human-readable summary of what the pipeline does.
	Description() string

	// Start begins the pipeline execution.
	// The pipeline instance should have been fully initialized by its factory, including its ClusterRuntime.
	// logger: A logger entry scoped for this pipeline execution, typically derived from ClusterRuntime.Log.
	Start(logger *logrus.Entry) error

	// RunModule executes a single module within the pipeline's context.
	// This method is typically called by the pipeline's own orchestration logic (e.g., within Start).
	// It should manage the full lifecycle of the module: IsSkip, Default, AutoAssert, Init, Run, Until, CallPostHook.
	RunModule(mod module.Module) *ending.ModuleResult
}
