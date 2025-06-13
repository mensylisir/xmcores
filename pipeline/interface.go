package pipeline

import (
	"github.com/mensylisir/xmcores/config" // For ClusterConfig
	"github.com/mensylisir/xmcores/runtime"
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
// This might be used for CLI help generation or documentation, even if ClusterConfig is primary.
type ParameterDefinition struct {
	Name         string        `json:"name" yaml:"name"`
	Type         ParameterType `json:"type" yaml:"type"`
	Description  string        `json:"description" yaml:"description"`
	Required     bool          `json:"required" yaml:"required"`
	DefaultValue interface{}   `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"`
}

// Pipeline represents a high-level workflow.
type Pipeline interface {
	// Name returns the unique name of the pipeline.
	Name() string

	// Description provides a human-readable summary of what the pipeline does.
	Description() string

	// ExpectedParameters returns a list of parameter definitions that this pipeline expects as input.
	// This can be used for documentation, CLI help, or preliminary validation.
	// It can return nil if parameters are solely defined by a typed config struct.
	ExpectedParameters() []pipeline.ParameterDefinition // Retained for now

	// Init prepares the pipeline for execution using the loaded ClusterConfig and initial runtime settings.
	// It should validate parameters and can set up an operational runtime for the pipeline.
	// - cfg: The fully parsed cluster configuration.
	// - initialRuntime: Provides access to global settings like WorkDir, IgnoreError, Verbose, and the base logger.
	// - logger: A logger entry pre-configured for this pipeline.
	Init(cfg *config.ClusterConfig, initialRuntime runtime.Runtime, logger *logrus.Entry) error

	// Execute runs the main logic of the pipeline.
	// It should use the operational runtime and configurations prepared during Init.
	// - logger: A logger entry pre-configured for this pipeline's execution phase.
	Execute(logger *logrus.Entry) error
}
