package task

import (
	"fmt"

	"github.com/mensylisir/xmcores/pipeline/ending" // For ModuleResult
	krt "github.com/mensylisir/xmcores/runtime"    // Alias for KubeRuntime
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// BaseTask provides a foundational implementation for the Task interface.
type BaseTask struct {
	NameField        string
	DescriptionField string
	Logger           *logrus.Entry    // Made public for concrete tasks to use for step logger scoping
	Runtime          *krt.KubeRuntime // Runtime passed from module, made public
	TaskSpec         interface{}      // Store the raw taskSpec, set by concrete task's Default/Init
	steps            []step.Step
	// moduleCache      interface{}
	// taskCache        interface{}
}

// NewBaseTask creates a new BaseTask with a name and description.
func NewBaseTask(name, description string) BaseTask {
	return BaseTask{
		NameField:        name,
		DescriptionField: description,
		steps:            make([]step.Step, 0),
	}
}

func (bt *BaseTask) Name() string {
	return bt.NameField
}

func (bt *BaseTask) Description() string {
	return bt.DescriptionField
}

// IsSkip provides a default IsSkip behavior. Concrete tasks can override.
func (bt *BaseTask) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if bt.Logger == nil { // Ensure logger is available
		bt.Logger = runtime.Log.WithField("task_early_skip_check", bt.NameField)
	}
	bt.Logger.Debugf("BaseTask.IsSkip called for %s (defaulting to false)", bt.NameField)
	return false, nil
}

// Default stores the runtime, taskSpec, and caches.
// Concrete tasks will call this, then set their own scoped logger.
func (bt *BaseTask) Default(runtime *krt.KubeRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	if runtime == nil {
		return fmt.Errorf("runtime cannot be nil for BaseTask.Default of task '%s'", bt.NameField)
	}
	bt.Runtime = runtime
	bt.TaskSpec = taskSpec // Store the raw taskSpec
	// bt.moduleCache = moduleCache
	// bt.taskCache = taskCache

	// Logger will be set by the concrete task's Default method, after it has its name.
	// For now, BaseTask's logger can be nil here, or set to a temporary one.
	// It will be overridden or must be set by concrete task's Default.
	// Let's ensure it's at least not nil for BaseTask's own methods if called early.
	if bt.Logger == nil {
		bt.Logger = runtime.Log.WithField("task_base_default", bt.NameField)
	}
	bt.Logger.Infof("BaseTask.Default called for '%s': runtime and taskSpec stored.", bt.NameField)
	return nil
}

// AutoAssert provides a default placeholder. Concrete tasks should override.
// It's called after Default, so runtime and logger should be available.
func (bt *BaseTask) AutoAssert() error {
	if bt.Logger == nil {
		return fmt.Errorf("logger not initialized for task '%s' in AutoAssert (ensure concrete Default sets it)", bt.NameField)
	}
	if bt.Runtime == nil {
		return fmt.Errorf("runtime not initialized for task '%s' in AutoAssert", bt.NameField)
	}
	// TaskSpec is not checked here as its specific validation is up to the concrete task.
	bt.Logger.Debug("BaseTask.AutoAssert called (no base assertions by default).")
	return nil
}

// Init is for the BaseTask's own internal initialization (if any).
// The primary role of a concrete task's Init() is to assemble its steps.
// BaseTask.Init() itself doesn't do much beyond ensuring logger is set.
func (bt *BaseTask) Init() error {
	if bt.Logger == nil {
		if bt.Runtime != nil && bt.Runtime.Log != nil {
			bt.Logger = bt.Runtime.Log.WithField("task_init_fallback", bt.NameField)
		} else {
			return fmt.Errorf("logger not initialized for task '%s' in Init and no runtime logger available", bt.NameField)
		}
		bt.Logger.Warn("Task logger was not set by concrete task's Default, using fallback for Init.")
	}
	bt.Logger.Debug("BaseTask.Init called.")
	return nil
}

// AddStep adds a step to the task's execution sequence.
// Steps should be fully initialized by the concrete task before being added.
func (bt *BaseTask) AddStep(s step.Step) {
	if s == nil {
		if bt.Logger != nil {
			bt.Logger.Warn("Attempted to add a nil step.")
		}
		return
	}
	bt.steps = append(bt.steps, s)
}

// Steps returns the list of steps configured for this task.
func (bt *BaseTask) Steps() []step.Step {
	return bt.steps // Returning direct slice for now; copy if external modification is a concern
}

// Run executes the task by iterating through its steps.
// It uses the ModuleResult passed from the parent module to report its outcome.
func (bt *BaseTask) Run(result *ending.ModuleResult) {
	if bt.Logger == nil {
		// Fallback if concrete task's Default didn't set logger
		if bt.Runtime != nil && bt.Runtime.Log != nil {
			bt.Logger = bt.Runtime.Log.WithField("task_run_fallback", bt.NameField)
		} else { // Absolute fallback
			tempLogger := logrus.New()
			bt.Logger = logrus.NewEntry(tempLogger).WithField("task_run_absolute_fallback", bt.NameField)
		}
		bt.Logger.Warn("Task logger was not set by concrete task's Default/Init, using fallback for Run.")
	}

	bt.Logger.Infof("Executing task: %s (%s)", bt.Name(), bt.Description())
	if len(bt.steps) == 0 {
		bt.Logger.Warn("Task has no steps to execute.")
		if result.Status == ending.ModuleResultPending { // Only update if not already failed/skipped
			result.SetStatus(ending.ModuleResultSuccess)
			result.SetMessage(fmt.Sprintf("Task '%s': No steps to execute.", bt.NameField))
		}
		return
	}

	if bt.Runtime == nil {
		bt.Logger.Error("Task runtime not initialized before Execute.")
		result.SetError(fmt.Errorf("task '%s' runtime not initialized", bt.NameField))
		return
	}

	taskFailed := false
	for i, currentStep := range bt.steps {
		stepLogger := bt.Logger.WithFields(logrus.Fields{
			"step_name": currentStep.Name(),
			"step_idx":  fmt.Sprintf("%d/%d", i+1, len(bt.steps)),
		})

		// Step Init (called by concrete task when adding step)
		// No guard Init here in BaseTask.Execute for steps; assumes concrete task did it.

		stepLogger.Infof("Executing step: %s (%s)", currentStep.Name(), currentStep.Description())
		output, success, execErr := currentStep.Execute(bt.Runtime, stepLogger.WithField("phase", "step_execute"))

		stepPostLogger := stepLogger.WithField("phase", "step_post")
		postErr := currentStep.Post(bt.Runtime, stepPostLogger, execErr)

		if execErr != nil {
			stepLogger.Errorf("Step execution failed: %v. Output: %s", execErr, output)
			result.AddError(fmt.Errorf("step '%s': %w", currentStep.Name(), execErr))
			taskFailed = true
		} else if !success {
			stepLogger.Warnf("Step reported not successful (no error from Execute). Output: %s", output)
			errMsg := fmt.Errorf("step '%s' reported not successful", currentStep.Name())
			result.AddError(errMsg)
			taskFailed = true
		} else {
			stepLogger.Infof("Step execution phase completed successfully. Output: %s", output)
		}

		if postErr != nil {
			stepLogger.Errorf("Step post-execution failed: %v", postErr)
			result.AddError(fmt.Errorf("step '%s' post-execution: %w", currentStep.Name(), postErr))
			taskFailed = true
		}

		if taskFailed && !bt.Runtime.IgnoreError() {
			bt.Logger.Errorf("Task '%s' failed at step '%s' and IgnoreError is false. Halting task.", bt.NameField, currentStep.Name())
			result.SetStatus(ending.ModuleResultFailed) // Ensure overall status is Failed
			if result.Message == "" { // Set a generic message if not already set by specific error
				result.SetMessage(fmt.Sprintf("Task '%s' failed at step '%s'", bt.NameField, currentStep.Name()))
			}
			return // Stop processing further steps in this task
		}
	}

	if taskFailed { // Errors occurred but were ignored, or this is the final summary point
		bt.Logger.Warnf("Task '%s' completed with one or more errors (IgnoreError: %v).", bt.NameField, bt.Runtime.IgnoreError())
		// ModuleResult status is already Failed if IgnoreError is false.
		// If IgnoreError is true, the errors are in result.Errors but status might still be Pending.
		// The module calling this task's Run will decide the final ModuleResult.Status based on this.
		// For now, if taskFailed is true, ensure the result reflects a failure occurred, even if ignored.
		if result.Status != ending.ModuleResultFailed { // If not already set to failed by a non-ignorable error
			result.SetStatus(ending.ModuleResultFailed) // Indicate failure, even if ignored by caller
			if result.Message == "" {
				result.SetMessage(fmt.Sprintf("Task '%s' encountered errors (ignored).", bt.NameField))
			}
		}
	} else {
		bt.Logger.Infof("Task '%s' and all its steps completed successfully.", bt.NameField)
		if result.Status == ending.ModuleResultPending {
			result.SetStatus(ending.ModuleResultSuccess)
			result.SetMessage(fmt.Sprintf("Task '%s' completed successfully.", bt.NameField))
		}
	}
}

// Until provides a default behavior. Concrete tasks can override.
func (bt *BaseTask) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	if bt.Logger == nil {
		bt.Logger = runtime.Log.WithField("task_early_until_check", bt.NameField)
	}
	bt.Logger.Debugf("BaseTask.Until called for %s (defaulting to true, no error).", bt.NameField)
	return true, nil
}

// Slogan provides a default slogan. Concrete tasks can override.
func (bt *BaseTask) Slogan() string {
	return fmt.Sprintf("Starting task: %s...", bt.NameField)
}

// Ensure BaseTask implements the Task interface.
var _ Task = (*BaseTask)(nil)
