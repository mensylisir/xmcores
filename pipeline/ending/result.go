package ending

import (
	"fmt"
	"strings"
)

// ModuleResultStatus defines the execution status of a module or task.
type ModuleResultStatus int

const (
	ModuleResultSuccess ModuleResultStatus = iota // Operation completed successfully
	ModuleResultFailed                            // Operation failed
	ModuleResultSkipped                           // Operation was skipped
	ModuleResultPending                           // Operation is pending or in an indeterminate state (e.g., for Until loops)
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
		return fmt.Sprintf("UNKNOWN_STATUS_%d (%d)", s, int(s)) // Include numeric value for unknown
	}
}

// ModuleResult holds the outcome of a module's (or task's) execution.
type ModuleResult struct {
	Status  ModuleResultStatus
	Message string
	Errors  []error // To aggregate multiple errors if they occur
}

// NewModuleResult creates a new ModuleResult, defaulting to Pending.
func NewModuleResult() *ModuleResult {
	return &ModuleResult{
		Status: ModuleResultPending, // Default to pending; explicit status should be set by the component.
		Errors: make([]error, 0),
	}
}

// IsFailed checks if the module execution is considered failed.
// A result is failed if its Status is explicitly ModuleResultFailed OR
// if it has errors and its status is still Pending (meaning it hasn't been explicitly marked Success/Skipped despite errors).
func (r *ModuleResult) IsFailed() bool {
	if r.Status == ModuleResultFailed {
		return true
	}
	// If status is still pending but errors have accumulated, it's effectively failed.
	if len(r.Errors) > 0 && r.Status == ModuleResultPending {
		return true
	}
	return false
}

// AddError appends an error to the list of errors.
// It sets the status to ModuleResultFailed if the current status is Pending or Success,
// as adding an error implies a failure or problem.
func (r *ModuleResult) AddError(err error) {
	if err == nil {
		return
	}
	r.Errors = append(r.Errors, err)
	if r.Status == ModuleResultPending || r.Status == ModuleResultSuccess {
		r.Status = ModuleResultFailed
	}
}

// SetError is a convenience method to set a primary error message and add the error.
// This always marks the result as Failed.
func (r *ModuleResult) SetError(err error, message string) {
	r.Message = message
	if err != nil { // Only add non-nil errors to the list
		r.Errors = append(r.Errors, err)
	} else if message != "" && len(r.Errors) == 0 { // If no actual error object but message implies one
		r.Errors = append(r.Errors, fmt.Errorf(message))
	}
	r.Status = ModuleResultFailed
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
func (r *ModuleResult) SetMessage(message string) {
	r.Message = message
}

// LocalErrResult is a helper to quickly create a ModuleResult from a local error.
// It sets the status to Failed, uses the error's message, and adds the error.
func LocalErrResult(err error) *ModuleResult {
	res := NewModuleResult()
	if err != nil {
		res.SetError(err, err.Error())
	} else {
		// Should not happen if err is nil, but defensively:
		res.SetStatus(ModuleResultSuccess) // Or some other appropriate status
	}
	return res
}
