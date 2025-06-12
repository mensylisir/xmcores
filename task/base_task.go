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
	var stepErrors []string

	for i, currentStep := range bt.steps {
		stepLog := log.WithFields(logrus.Fields{
			common.LogFieldStepName: currentStep.Name(),
			"step_index":            fmt.Sprintf("%d/%d", i+1, len(bt.steps)),
		})
		stepLog.Infof("Executing step: %s (%s)", currentStep.Name(), currentStep.Description())
		// Standard output for user visibility (as per issue requirement)
		fmt.Printf("===> Executing Step: %s (%s)\n", currentStep.Name(), currentStep.Description())


		stepOutput, stepSuccess, stepErr := currentStep.Execute(rt, stepLog)

		if stepOutput != "" {
			// Log the detailed output from the step, which might include stdout/stderr of commands
			stepLog.Debugf("Step execution output:\n%s", stepOutput)
		}

		// Call Post for the current step, regardless of its success/failure
		postLog := stepLog.WithField("sub_phase", "post_execute")
		if postErr := currentStep.Post(rt, postLog, stepErr); postErr != nil {
			postLog.Errorf("Error during Post-Execute for step %s: %v", currentStep.Name(), postErr)
			// This error might also contribute to overall task failure if critical
			stepErrors = append(stepErrors, fmt.Sprintf("post-execute error for step %s: %v", currentStep.Name(), postErr))
			overallTaskFailed = true
		}

		if stepErr != nil {
			stepLog.Errorf("Step %s failed: %v", currentStep.Name(), stepErr)
			fmt.Printf("===> Step FAILED: %s. Error: %v\n", currentStep.Name(), stepErr) // User visibility
			stepErrors = append(stepErrors, fmt.Sprintf("step %s error: %v", currentStep.Name(), stepErr))
			overallTaskFailed = true
			if !rt.IgnoreError() {
				log.Errorf("Task %s failed at step %s due to error: %v. Halting task execution.", bt.Name(), currentStep.Name(), stepErr)
				return fmt.Errorf("task %s failed at step %s: %w", bt.Name(), currentStep.Name(), stepErr)
			}
			log.Warnf("Step %s failed but IgnoreError is true. Continuing task execution.", currentStep.Name())
		} else if !stepSuccess {
			stepLog.Warnf("Step %s completed but reported not successful (no error returned). Output: %s", currentStep.Name(), stepOutput)
			fmt.Printf("===> Step COMPLETED (Reported Not Successful): %s.\n", currentStep.Name()) // User visibility
			// This case might also be considered a failure depending on strictness.
			// For now, if no explicit error, we treat it as a warning unless IgnoreError is false.
			// If it's a failure, overallTaskFailed should be true.
			// Let's assume stepSuccess=false without an error means a soft failure or warning.
			// To make it a hard failure:
			// overallTaskFailed = true
			// stepErrors = append(stepErrors, fmt.Sprintf("step %s reported not successful", currentStep.Name()))
			// if !rt.IgnoreError() { ... return ... }
		} else {
			stepLog.Infof("Step %s completed successfully.", currentStep.Name())
			fmt.Printf("===> Step SUCCEEDED: %s.\n", currentStep.Name()) // User visibility
		}
	}

	if overallTaskFailed {
		log.Errorf("Task %s completed with one or more errors: %s", bt.Name(), strings.Join(stepErrors, "; "))
		return fmt.Errorf("task %s failed with errors: %s", bt.Name(), strings.Join(stepErrors, "; "))
	}

	log.Infof("Task %s completed successfully.", bt.Name())
	return nil
}

// Post provides a default no-op Post implementation for the task itself.
// Concrete tasks can override this for task-level cleanup.
func (bt *BaseTask) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
	log.Debugf("BaseTask.Post called for task %s.", bt.Name())
	// If task-level cleanup depends on Execute's success/failure, use executeErr.
	return nil
}
