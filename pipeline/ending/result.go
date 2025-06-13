package ending

import (
	"fmt"
	"strings"
)

// ModuleResultStatus defines the execution status of a module.
type ModuleResultStatus int

const (
	ModuleResultSuccess ModuleResultStatus = iota // Operation completed successfully
	ModuleResultFailed                            // Operation failed
	ModuleResultSkipped                           // Operation was skipped
	ModuleResultPending                           // Operation is pending or in an indeterminate state
)

// String returns a string representation of the ModuleResultStatus.
func (s ModuleResultStatus) String() string {
	switch s {
	case ModuleResultSuccess:
		return "SUCCESS"
	case ModuleResultFailed:
		return "FAILED"
	case ModuleResultSkipped:
		return "SKIPPED"
	case ModuleResultPending:
		return "PENDING"
	default:
		return fmt.Sprintf("UNKNOWN_STATUS_%d", s)
	}
}

// ModuleResult holds the outcome of a module's execution.
type ModuleResult struct {
	Status        ModuleResultStatus
	Message       string
	Errors        []error // Changed CombineResult to a slice of errors for more granular error reporting
	StepResults   []interface{} // Placeholder for potential step-level results if needed
}

// NewModuleResult creates a new ModuleResult, defaulting to Success.
func NewModuleResult() *ModuleResult {
	return &ModuleResult{
		Status: ModuleResultSuccess, // Default to success
		Errors: make([]error, 0),
	}
}

// IsFailed checks if the module execution is considered failed.
func (r *ModuleResult) IsFailed() bool {
	return r.Status == ModuleResultFailed || len(r.Errors) > 0
}

// SetError marks the module result as failed and records an error.
// Multiple errors can be added.
func (r *ModuleResult) SetError(err error, msg ...string) {
	r.Status = ModuleResultFailed
	if len(msg) > 0 {
		if r.Message != "" {
			r.Message = fmt.Sprintf("%s; %s: %v", r.Message, strings.Join(msg, " "), err)
		} else {
			r.Message = fmt.Sprintf("%s: %v", strings.Join(msg, " "), err)
		}
	} else if err != nil {
		if r.Message != "" {
			r.Message = fmt.Sprintf("%s; error: %v", r.Message, err)
		} else {
			r.Message = fmt.Sprintf("error: %v", err)
		}
	}

	if err != nil {
		r.Errors = append(r.Errors, err)
	}
}

// AddError appends an error to the list of errors.
// If the module status is not already Failed, this does not automatically set it to Failed,
// allowing for accumulation of non-critical errors or warnings if needed,
// but SetError or directly setting Status is preferred for marking failure.
func (r *ModuleResult) AddError(err error) {
	if err != nil {
		r.Errors = append(r.Errors, err)
	}
}

// CombinedError returns a single error object that aggregates all recorded errors.
// Returns nil if there are no errors.
func (r *ModuleResult) CombinedError() error {
	if len(r.Errors) == 0 {
		return nil
	}
	if len(r.Errors) == 1 {
		return r.Errors[0]
	}
	// Simple concatenation for now; more sophisticated error joining could be used.
	var errorStrings []string
	for _, e := range r.Errors {
		errorStrings = append(errorStrings, e.Error())
	}
	return fmt.Errorf("multiple errors occurred: %s", strings.Join(errorStrings, "; "))
}

// SetStatus sets the module's result status.
func (r *ModuleResult) SetStatus(status ModuleResultStatus) {
	r.Status = status
}

// SetMessage sets a descriptive message for the result.
func (r *ModuleResult) SetMessage(msg string) {
	r.Message = msg
}
