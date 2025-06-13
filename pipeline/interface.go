package pipeline

import (
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	"github.com/mensylisir/xmcores/runtime" // For KubeRuntime in PipelineFactory
	"github.com/sirupsen/logrus"
)

// ParameterType defines the type of a parameter.
type ParameterType string

const (
	ParamTypeString  ParameterType = "string"
	ParamTypeInteger ParameterType = "integer"
	ParamTypeBoolean ParameterType = "boolean"
	ParamTypeMap     ParameterType = "map"
	ParamTypeList    ParameterType = "list"
)

// ParameterDefinition describes an expected parameter for a pipeline.
type ParameterDefinition struct {
	Name         string        `json:"name" yaml:"name"`
	Type         ParameterType `json:"type" yaml:"type"`
	Description  string        `json:"description" yaml:"description"`
	Required     bool          `json:"required" yaml:"required"`
	DefaultValue interface{}   `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"`
}

// PipelineFactory defines the function signature for creating pipeline instances.
// It now accepts KubeRuntime to allow pipelines to be initialized with their specific runtime context and config.
type PipelineFactory func(kr *runtime.KubeRuntime) (Pipeline, error)

// Pipeline represents a high-level workflow that orchestrates a series of modules.
type Pipeline interface {
	Name() string
	Description() string
	ExpectedParameters() []ParameterDefinition // For documentation or CLI help

	// Start begins the pipeline execution.
	// The pipeline instance should have been fully initialized by its factory, including its KubeRuntime.
	// logger: A logger entry scoped for this pipeline execution.
	Start(logger *logrus.Entry) error

	// RunModule executes a single module within the pipeline's context.
	// This method is typically called by the pipeline's own orchestration logic (e.g., within Start).
	RunModule(mod module.Module) *ending.ModuleResult
}
