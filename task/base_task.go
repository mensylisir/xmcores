package task

import (
	"fmt"
	// "strings" // For error aggregation if re-implementing complex summary

	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// BaseTask provides a foundational implementation for the Task interface.
// It manages step execution and basic state.
type BaseTask struct {
	NameField        string // Using NameField to avoid conflict with Name() method
	DescriptionField string
	logger           *logrus.Entry
	runtime          runtime.Runtime // Runtime passed from module during Init
	steps            []step.Step
	// taskSpec         interface{} // Optionally store the raw taskSpec if needed by BaseTask logic beyond Init
}

// NewBaseTask creates a new BaseTask with a name and description.
func NewBaseTask(name, description string) BaseTask {
	return BaseTask{
		NameField:        name,
		DescriptionField: description,
		steps:            make([]step.Step, 0),
	}
}

// Name returns the name of the task.
func (bt *BaseTask) Name() string {
	return bt.NameField
}

// Description returns the description of the task.
func (bt *BaseTask) Description() string {
	return bt.DescriptionField
}

// Init stores the provided moduleRuntime and logger.
// Concrete task implementations should call this BaseTask.Init, then assert their specific
// taskSpec, and then use AddStep to populate their steps.
func (bt *BaseTask) Init(moduleRuntime runtime.Runtime, taskSpec interface{}, logger *logrus.Entry) error {
	if logger == nil {
		return fmt.Errorf("logger cannot be nil for BaseTask Init")
	}
	bt.logger = logger.WithField("task_name", bt.NameField) // Scope logger
	if moduleRuntime == nil {
		return fmt.Errorf("moduleRuntime cannot be nil for BaseTask Init")
	}
	bt.runtime = moduleRuntime
	// bt.taskSpec = taskSpec // Concrete task will handle and use the taskSpec for its step setup.
	bt.logger.Info("BaseTask initialized (runtime and logger stored). Steps should be added by concrete task's Init.")
	return nil
}

// AddStep adds a step to the task's execution sequence.
// This should be called by the concrete task's Init method after steps are created and initialized.
func (bt *BaseTask) AddStep(s step.Step) {
	if s == nil {
		bt.logger.Warn("Attempted to add a nil step.")
		return
	}
	bt.steps = append(bt.steps, s)
}

// Steps returns the list of steps configured for this task.
func (bt *BaseTask) Steps() []step.Step {
	// Return a copy to prevent external modification, good practice.
	s := make([]step.Step, len(bt.steps))
	copy(s, bt.steps)
	return s
}

// Execute iterates through the configured steps and executes them.
// It uses the logger and runtime stored during BaseTask.Init.
func (bt *BaseTask) Execute(loggerConvenienceParam *logrus.Entry) error {
	// Use the logger initialized in BaseTask.Init. The loggerConvenienceParam is mostly to satisfy
	// the interface if we want modules/pipelines to pass their logger directly, but BaseTask
	// should ideally use its own initialized and scoped logger (bt.logger).
	taskExecLogger := bt.logger
	if taskExecLogger == nil { // Safeguard if Init wasn't called properly or logger wasn't set
		if loggerConvenienceParam != nil {
			taskExecLogger = loggerConvenienceParam.WithField("task_execute_safeguard", bt.NameField)
		} else {
			// Fallback to a default logger if absolutely necessary, though this indicates an issue.
			// This shouldn't happen if Init is called correctly.
			return fmt.Errorf("task '%s' logger not initialized before Execute", bt.NameField)
		}
	}

	taskExecLogger.Infof("Executing task: %s (%s)", bt.Name(), bt.Description())
	if len(bt.steps) == 0 {
		taskExecLogger.Warn("Task has no steps to execute.")
		return nil
	}

	if bt.runtime == nil { // Safeguard
		return fmt.Errorf("task '%s' runtime not initialized before Execute", bt.NameField)
	}


	var overallTaskFailed bool
	// Summary report and detailed error aggregation can be added here later,
	// similar to the previous BaseTask.Execute that was refactored out.
	// For now, focusing on the Init/Execute pattern for steps.

	for i, currentStep := range bt.steps {
		stepLogger := taskExecLogger.WithFields(logrus.Fields{
			"step_name": currentStep.Name(),
			"step_index": fmt.Sprintf("%d/%d", i+1, len(bt.steps)),
		})

		// Step Init:
		// It's assumed that the concrete Task's Init method is responsible for initializing its steps
		// *before* adding them to BaseTask. If a step requires runtime/logger for its own Init,
		// the concrete Task's Init should provide that.
		// The "guard Init" here is a fallback, but steps should ideally be fully ready.
		// Let's remove the guard init for now, to enforce that concrete tasks handle step init.
		// stepLogger.Infof("Initializing step (guard): %s (%s)", currentStep.Name(), currentStep.Description())
		// if err := currentStep.Init(bt.runtime, stepLogger.WithField("phase", "step_init_guard")); err != nil {
		//      stepLogger.Errorf("Guard Init for step %s failed: %v", currentStep.Name(), err)
		//      if !bt.runtime.IgnoreError() {
		//          return fmt.Errorf("guard Init for step %s failed: %w", currentStep.Name(), err)
		//      }
		//      overallTaskFailed = true // Mark task as failed
		//      continue // Skip Execute and Post if Init guard fails
		// }


		// Step Execute:
		stepLogger.Infof("Executing step: %s (%s)", currentStep.Name(), currentStep.Description())
		output, success, execErr := currentStep.Execute(bt.runtime, stepLogger.WithField("phase", "step_execute"))

		// Step Post: (Using the error from Execute)
		postErr := currentStep.Post(bt.runtime, stepLogger.WithField("phase", "step_post"), execErr)

		if execErr != nil {
			stepLogger.Errorf("Step %s execution failed: %v. Output: %s", currentStep.Name(), execErr, output)
			overallTaskFailed = true
			if !bt.runtime.IgnoreError() {
				return fmt.Errorf("step '%s' execution failed: %w", currentStep.Name(), execErr)
			}
		} else if !success {
			stepLogger.Warnf("Step %s completed but reported not successful (no error from Execute). Output: %s", currentStep.Name(), output)
			overallTaskFailed = true // Treat non-success as a failure for the task
			if !bt.runtime.IgnoreError() {
				return fmt.Errorf("step '%s' reported not successful", currentStep.Name())
			}
		} else {
			stepLogger.Infof("Step %s execution phase completed successfully. Output: %s", currentStep.Name(), output)
		}

		if postErr != nil {
			stepLogger.Errorf("Step %s post-execution failed: %v", currentStep.Name(), postErr)
			overallTaskFailed = true
			if !bt.runtime.IgnoreError() {
				// If Execute was successful but Post failed, we still need to signal task failure.
				return fmt.Errorf("step '%s' post-execution failed: %w", currentStep.Name(), postErr)
			}
		}
	}

	if overallTaskFailed {
		taskExecLogger.Errorf("Task '%s' completed with one or more errors.", bt.Name())
		if !bt.runtime.IgnoreError() { // Check IgnoreError for the final task return
			return fmt.Errorf("task %s completed with one or more errors", bt.Name())
		}
		taskExecLogger.Warnf("Task '%s' had errors, but IgnoreError is true. Task considered non-failed overall.", bt.Name())
	} else {
		taskExecLogger.Infof("Task '%s' and all its steps completed successfully.", bt.Name())
	}
	return nil
}

// Ensure BaseTask implements the Task interface.
var _ Task = (*BaseTask)(nil)
