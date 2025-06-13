package task

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/common" // For logger field constants
	"github.com/mensylisir/xmcores/logger"  // For the global logger if needed, or pass entry
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// StepExecutionResult holds the outcome of a single step's execution.
type StepExecutionResult struct {
	StepName string
	Success  bool
	Error    error
	Output   string // Optional: To store primary output for summary
}

// BaseTask provides a basic implementation for the Task interface.
// It can be embedded in concrete task implementations.
type BaseTask struct {
	name        string
	description string
	steps       []step.Step
	// We can add fields for overall status if needed, e.g.,
	// succeeded bool
	// output    string
}

// NewBaseTask creates a new BaseTask.
// Steps should be added via AddStep or SetSteps.
func NewBaseTask(name, description string) BaseTask {
	return BaseTask{
		name:        name,
		description: description,
		steps:       make([]step.Step, 0),
	}
}

// Name returns the name of the task.
func (bt *BaseTask) Name() string {
	return bt.name
}

// SetName sets the name of the task.
func (bt *BaseTask) SetName(name string) {
	bt.name = name
}

// Description returns the description of the task.
func (bt *BaseTask) Description() string {
	return bt.description
}

// SetDescription sets the description of the task.
func (bt *BaseTask) SetDescription(desc string) {
	bt.description = desc
}


// Steps returns the list of steps in the task.
func (bt *BaseTask) Steps() []step.Step {
	// Return a copy to prevent external modification
	s := make([]step.Step, len(bt.steps))
	copy(s, bt.steps)
	return s
}

// AddStep adds a step to the task's execution list.
func (bt *BaseTask) AddStep(s step.Step) {
	bt.steps = append(bt.steps, s)
}

// SetSteps sets the list of steps for the task.
func (bt *BaseTask) SetSteps(steps []step.Step) {
	bt.steps = make([]step.Step, len(steps))
	copy(bt.steps, steps)
}

// Init provides a default Init implementation that initializes all added steps.
// Concrete tasks can override this if they have more complex initialization logic.
func (bt *BaseTask) Init(rt runtime.Runtime, log *logrus.Entry) error {
	log.Debugf("BaseTask.Init called for task: %s. Initializing %d steps.", bt.Name(), len(bt.steps))
	if len(bt.steps) == 0 {
		log.Warn("No steps defined for this task.")
		// Depending on requirements, this could be an error:
		// return fmt.Errorf("no steps defined for task %s", bt.Name())
	}
	for i, s := range bt.steps {
		stepLog := log.WithFields(logrus.Fields{
			common.LogFieldStepName: s.Name(),
			"step_index":            fmt.Sprintf("%d/%d", i+1, len(bt.steps)),
		})
		stepLog.Infof("Initializing step: %s (%s)", s.Name(), s.Description())
		if err := s.Init(rt, stepLog); err != nil {
			stepLog.Errorf("Failed to initialize step %s: %v", s.Name(), err)
			return fmt.Errorf("failed to initialize step %s (index %d) in task %s: %w", s.Name(), i, bt.Name(), err)
		}
	}
	log.Infof("All %d steps for task %s initialized successfully.", len(bt.steps), bt.Name())
	return nil
}

// Execute provides a default Execute implementation that runs all steps sequentially.
func (bt *BaseTask) Execute(rt runtime.Runtime, log *logrus.Entry) error {
	log.Infof("Executing task: %s (%s)", bt.Name(), bt.Description())
	if len(bt.steps) == 0 {
		log.Warnf("Task %s has no steps to execute.", bt.Name())
		return nil // Or an error if tasks must have steps
	}

	var overallTaskFailed bool
	stepResults := make([]StepExecutionResult, 0, len(bt.steps))

	for i, currentStep := range bt.steps {
		stepLog := log.WithFields(logrus.Fields{
			common.LogFieldStepName: currentStep.Name(),
			"step_index":            fmt.Sprintf("%d/%d", i+1, len(bt.steps)),
		})
		stepLog.Infof("Executing step: %s (%s)", currentStep.Name(), currentStep.Description())
		fmt.Printf("===> Executing Step: %s (%s)\n", currentStep.Name(), currentStep.Description())

		stepOutput, stepSuccess, stepErr := currentStep.Execute(rt, stepLog)

		if stepOutput != "" {
			stepLog.Debugf("Step execution output:\n%s", stepOutput)
		}

		// Call Post for the current step, regardless of its success/failure from Execute
		postLog := stepLog.WithField("sub_phase", "post_execute")
		postErr := currentStep.Post(rt, postLog, stepErr) // Pass Execute's error to Post

		currentStepSuccess := stepSuccess && stepErr == nil && postErr == nil
		var combinedErr error
		errorMessages := []string{}

		if stepErr != nil {
			errorMessages = append(errorMessages, stepErr.Error())
		}
		if postErr != nil {
			postLog.Errorf("Error during Post-Execute for step %s: %v", currentStep.Name(), postErr)
			errorMessages = append(errorMessages, fmt.Sprintf("post-execute error: %v", postErr))
		}

		if len(errorMessages) > 0 {
			combinedErr = fmt.Errorf(strings.Join(errorMessages, "; "))
		}

		stepResults = append(stepResults, StepExecutionResult{
			StepName: currentStep.Name(),
			Success:  currentStepSuccess,
			Error:    combinedErr,
			Output:   stepOutput, // Or a summary if needed
		})

		if !currentStepSuccess {
			overallTaskFailed = true
			stepLog.Errorf("Step %s determined as FAILED. Success: %v, CombinedError: %v", currentStep.Name(), currentStepSuccess, combinedErr)
			// User visibility message is now part of the summary
		} else {
			stepLog.Infof("Step %s determined as SUCCEEDED.", currentStep.Name())
			// User visibility message is now part of the summary
		}
	}

	// Print Summary Report
	fmt.Printf("\n--- Task Execution Summary for '%s' ---\n", bt.Name())
	log.Info("--- Task Execution Summary ---") // Log summary start as well
	for _, result := range stepResults {
		status := "SUCCEEDED"
		errMsg := ""
		if !result.Success {
			status = "FAILED"
			if result.Error != nil {
				errMsg = fmt.Sprintf(" (Error: %v)", result.Error)
			} else {
				errMsg = " (Reason: reported not successful)" // Should ideally have an error if !Success
			}
		}
		summaryLine := fmt.Sprintf("Step '%s': %s%s", result.StepName, status, errMsg)
		fmt.Println(summaryLine)
		if result.Success {
			log.Infof(summaryLine)
		} else {
			log.Errorf(summaryLine)
		}
	}
	fmt.Println("------------------------------------")
	log.Info("------------------------------------")


	if overallTaskFailed {
		log.Errorf("Task '%s' completed with one or more errors.", bt.Name())
		if !rt.IgnoreError() {
			return fmt.Errorf("task '%s' completed with one or more errors. See summary above for details.", bt.Name())
		}
		log.Warnf("Task '%s' had errors, but IgnoreError is true. Overall task considered non-failed due to IgnoreError.", bt.Name())
		return nil // Return nil because errors are being ignored at the task level
	}

	log.Infof("Task '%s' completed successfully with all steps successful.", bt.Name())
	return nil
}

// Post provides a default no-op Post implementation for the task itself.
// Concrete tasks can override this for task-level cleanup.
func (bt *BaseTask) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
	log.Debugf("BaseTask.Post called for task %s.", bt.Name())
	// If task-level cleanup depends on Execute's success/failure, use executeErr.
	return nil
}
