package pipeline

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// ParameterType defines the type of a parameter.
// Using string for simplicity, could be an iota-based enum for stricter type checking.
type ParameterType string

const (
	ParamTypeString  ParameterType = "string"
	ParamTypeInteger ParameterType = "integer"
	ParamTypeBoolean ParameterType = "boolean"
	ParamTypeMap     ParameterType = "map"
	ParamTypeList    ParameterType = "list" // Could be list of strings, integers, etc.
)

// ParameterDefinition describes an expected parameter for a pipeline.
type ParameterDefinition struct {
	Name         string        `json:"name" yaml:"name"`
	Type         ParameterType `json:"type" yaml:"type"`
	Description  string        `json:"description" yaml:"description"`
	Required     bool          `json:"required" yaml:"required"`
	DefaultValue interface{}   `json:"defaultValue,omitempty" yaml:"defaultValue,omitempty"` // Optional default value
}

// Pipeline represents a high-level workflow consisting of multiple modules or tasks.
// It defines its expected input parameters.
type Pipeline interface {
	// Name returns the unique name of the pipeline.
	Name() string

	// Description provides a human-readable summary of what the pipeline does.
	Description() string

	// ExpectedParameters returns a list of parameter definitions that this pipeline expects as input.
	// This allows for validation, UI generation, and clear documentation of pipeline inputs.
	ExpectedParameters() []ParameterDefinition

	// Execute runs the pipeline's workflow.
	// - rt: The runtime environment providing access to hosts, runners, etc.
	// - configData: A map containing the actual parameter values provided for this pipeline execution,
	//               matching the definitions from ExpectedParameters().
	// - logger: A logger entry for structured logging within the pipeline.
	// It returns an error if the pipeline execution fails.
	Execute(rt runtime.Runtime, configData map[string]interface{}, logger *logrus.Entry) error
}
