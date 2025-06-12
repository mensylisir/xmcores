package step

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// BaseStep provides a basic implementation of some Step methods.
// It can be embedded in concrete step implementations.
type BaseStep struct {
	name        string
	description string
	// Concrete steps can add their specific parameters here
}

// NewBaseStep creates a new BaseStep.
func NewBaseStep(name, description string) BaseStep {
	return BaseStep{
		name:        name,
		description: description,
	}
}

// Name returns the name of the step.
func (s *BaseStep) Name() string {
	return s.name
}

// Description returns the description of the step.
func (s *BaseStep) Description() string {
	return s.description
}

// Init provides a default no-op Init implementation.
// Concrete steps should override this if they need initialization.
func (s *BaseStep) Init(rt runtime.Runtime, logger *logrus.Entry) error {
	logger.Debug("Default BaseStep.Init called, no action taken.")
	return nil
}

// Post provides a default no-op Post implementation.
// Concrete steps should override this if they need cleanup.
func (s *BaseStep) Post(rt runtime.Runtime, logger *logrus.Entry, executeErr error) error {
	logger.Debug("Default BaseStep.Post called, no action taken.")
	return nil
}
