package step

import (
	"fmt"
	// Adjust path if ClusterRuntime is not directly in runtime package
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// BaseStep provides common fields and default method implementations for steps.
type BaseStep struct {
	NameField        string
	DescriptionField string
	Logger           *logrus.Entry       // Scoped logger for the step instance
	Runtime          *krt.ClusterRuntime // Runtime context for the step
}

// NewBaseStep is a helper constructor for initializing common BaseStep fields.
// Concrete steps can call this in their own constructors.
func NewBaseStep(name, description string) BaseStep {
	return BaseStep{
		NameField:        name,
		DescriptionField: description,
	}
}

// Name returns the name of the step.
func (bs *BaseStep) Name() string {
	return bs.NameField
}

// Description returns the description of the step.
func (bs *BaseStep) Description() string {
	return bs.DescriptionField
}

// Init stores the runtime and logger.
// Concrete step's Init method should call this base Init, then perform its own specific initialization.
func (bs *BaseStep) Init(rt *krt.ClusterRuntime, logger *logrus.Entry) error {
	if rt == nil {
		return fmt.Errorf("runtime cannot be nil for BaseStep.Init of step '%s'", bs.NameField)
	}
	if logger == nil {
		// Fallback, though logger should always be provided by the calling Task.
		bs.Logger = logrus.NewEntry(logrus.New()).WithField("step_base_init_fallback", bs.NameField)
		bs.Logger.Warn("Logger was nil in BaseStep.Init, using fallback.")
	} else {
		bs.Logger = logger.WithField("step_base", bs.NameField) // Scope the logger
	}
	bs.Runtime = rt
	bs.Logger.Infof("BaseStep Init completed for step [%s]", bs.NameField)
	return nil
}

// Execute is typically overridden by concrete steps.
// The base implementation returns an error indicating it's not implemented.
func (bs *BaseStep) Execute(rt *krt.ClusterRuntime, logger *logrus.Entry) (output string, success bool, err error) {
	// Ensure bs.Logger is available if called directly on BaseStep (though concrete steps should use their own)
	currentLogger := logger
	if bs.Logger != nil {
		currentLogger = bs.Logger
	}
	currentLogger.Warnf("BaseStep.Execute called directly for step [%s], this should be overridden by a concrete step implementation.", bs.NameField)
	return "", false, fmt.Errorf("Execute not implemented in BaseStep for step '%s'", bs.NameField)
}

// Post is a hook for post-execution actions. Base implementation is a no-op.
// Concrete steps can override this to perform cleanup or other actions.
func (bs *BaseStep) Post(rt *krt.ClusterRuntime, logger *logrus.Entry, stepExecuteErr error) error {
	currentLogger := logger
	if bs.Logger != nil {
		currentLogger = bs.Logger
	}
	currentLogger.Infof("BaseStep Post for step [%s]. Execute error (if any): %v", bs.NameField, stepExecuteErr)
	return nil // Default implementation does nothing and returns no error.
}
