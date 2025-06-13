package task

import (
	"fmt"

	"github.com/mensylisir/xmcores/pipeline/ending" // For ModuleResult
	krt "github.com/mensylisir/xmcores/runtime"    // Alias for ClusterRuntime
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// BaseTask provides a foundational implementation for the Task interface.
type BaseTask struct {
	NameField        string
	DescriptionField string
	Logger           *logrus.Entry
	Runtime          *krt.ClusterRuntime // Runtime passed from module
	TaskSpec         interface{}         // Store the raw taskSpec, set by concrete task's Default
	steps            []step.Step
	// moduleCache      interface{} // Cache from parent module, if tasks need it
	// taskCache        interface{} // Task's own cache, if needed
}

// NewBaseTask creates a new BaseTask with a name and description.
// It now returns a pointer to BaseTask.
func NewBaseTask(name, description string) *BaseTask {
	return &BaseTask{
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

func (bt *BaseTask) Slogan() string {
	return fmt.Sprintf("Executing Task: %s", bt.NameField)
}

func (bt *BaseTask) IsSkip(runtime *krt.ClusterRuntime) (bool, error) {
	if bt.Logger == nil && runtime != nil && runtime.Log != nil { // Ensure logger exists for this message
		bt.Logger = runtime.Log.WithField("task_early_skip", bt.NameField)
	}
	if bt.Logger != nil {
		bt.Logger.Debugf("BaseTask.IsSkip called for %s (defaulting to false)", bt.NameField)
	}
	return false, nil
}

func (bt *BaseTask) Default(runtime *krt.ClusterRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	if runtime == nil {
		return fmt.Errorf("runtime cannot be nil for BaseTask.Default of task '%s'", bt.NameField)
	}
	bt.Runtime = runtime
	bt.TaskSpec = taskSpec

	// Set a default logger. Concrete tasks should override this with a more specific scope.
	bt.Logger = runtime.Log.WithFields(logrus.Fields{"task_base_default": bt.NameField, "task_name": bt.NameField})
	bt.Logger.Info("BaseTask.Default called (runtime, taskSpec, base logger set)")
	return nil
}

func (bt *BaseTask) AutoAssert(runtime *krt.ClusterRuntime) error {
	if bt.Logger == nil { return fmt.Errorf("logger not initialized for task '%s' in AutoAssert", bt.NameField) }
	if bt.Runtime == nil { return fmt.Errorf("runtime not initialized for task '%s' in AutoAssert", bt.NameField) }
	// TaskSpec presence might be asserted by concrete task's AutoAssert
	bt.Logger.Debug("BaseTask.AutoAssert called (no base assertions by default).")
	return nil
}

// Init is for the BaseTask's own internal initialization.
// Concrete task's Init method is responsible for step assembly using the stored TaskSpec.
func (bt *BaseTask) Init() error {
	if bt.Logger == nil { return fmt.Errorf("logger not initialized for task '%s' in Init", bt.NameField) }
	bt.Logger.Debug("BaseTask.Init called (placeholder for base task init logic, if any).")
	return nil
}

func (bt *BaseTask) Run(result *ending.ModuleResult) {
	if bt.Logger == nil {
		// Attempt to create a fallback logger if absolutely necessary
		if bt.Runtime != nil && bt.Runtime.Log != nil {
			bt.Logger = bt.Runtime.Log.WithField("task_run_fallback", bt.NameField)
		} else {
			tempLogger := logrus.New() // Should not happen in normal flow
			bt.Logger = logrus.NewEntry(tempLogger).WithField("task_run_absolute_fallback", bt.NameField)
		}
		bt.Logger.Error("Task logger was not set by concrete task's Default method prior to Run. This is a bug in the concrete task's Default method.")
		// Do not return early, try to proceed with a fallback logger.
	}

	bt.Logger.Infof("BaseTask.Run executing task: %s (%s)", bt.NameField, bt.DescriptionField)
	if len(bt.steps) == 0 {
		bt.Logger.Warnf("Task [%s] has no steps to execute.", bt.NameField)
		if result.Status == ending.ModuleResultPending { // Only set success if not already failed/skipped
			result.SetStatus(ending.ModuleResultSuccess)
			result.SetMessage(fmt.Sprintf("Task '%s': No steps to execute, considered successful.", bt.NameField))
		}
		return
	}

	if bt.Runtime == nil {
		bt.Logger.Error("Task runtime not initialized before Run.")
		result.SetError(fmt.Errorf("task '%s' runtime not initialized", bt.NameField), "Runtime not initialized")
		return
	}
	if bt.Runtime.Arg == nil { // CliArgs might be needed for IgnoreErr
		bt.Logger.Error("Task runtime.Arg (CliArgs) not initialized before Run.")
		result.SetError(fmt.Errorf("task '%s' runtime.Arg (CliArgs) not initialized", bt.NameField), "CliArgs not initialized in runtime")
		return
	}


	taskFailedOverall := false
	for i, currentStep := range bt.steps {
		stepLogger := bt.Logger.WithFields(logrus.Fields{
			"step_name":  currentStep.Name(),
			"step_index": fmt.Sprintf("%d/%d", i+1, len(bt.steps)),
		})

		// Step Init is assumed to be called by the concrete Task's Init when the step was added/configured.
		stepLogger.Infof("Executing step: %s (%s)", currentStep.Name(), currentStep.Description())
		output, success, execErr := currentStep.Execute(bt.Runtime, stepLogger.WithField("phase", "execute"))

		stepFailed := false
		if execErr != nil {
			stepLogger.Errorf("Step [%s] execution failed: %v. Output: %s", currentStep.Name(), execErr, output)
			result.AddError(fmt.Errorf("step [%s] failed: %w. Output: %s", currentStep.Name(), execErr, output))
			stepFailed = true
		} else if !success {
			stepLogger.Warnf("Step [%s] reported not successful. Output: %s", currentStep.Name(), output)
			result.AddError(fmt.Errorf("step [%s] reported not successful. Output: %s", currentStep.Name(), output))
			stepFailed = true
		} else {
			stepLogger.Infof("Step [%s] completed successfully. Output: %s", currentStep.Name(), output)
		}

		// Call Step's Post method
		postErr := currentStep.Post(bt.Runtime, stepLogger.WithField("phase", "post"), execErr)
		if postErr != nil {
			stepLogger.Errorf("Step [%s] post-execution failed: %v", currentStep.Name(), postErr)
			result.AddError(fmt.Errorf("step [%s] post-execution failed: %w", currentStep.Name(), postErr))
			stepFailed = true // Failure in Post also makes the step (and task) failed
		}

		if stepFailed {
			taskFailedOverall = true
			if !bt.Runtime.Arg.IgnoreErr {
				bt.Logger.Errorf("Task [%s] failed at step [%s] and IgnoreErr is false. Halting task.", bt.NameField, currentStep.Name())
				// result.Status is already set to Failed by AddError
				if result.Message == "" {
					result.SetMessage(fmt.Sprintf("Task '%s' failed at step '%s'", bt.NameField, currentStep.Name()))
				}
				return // Stop processing further steps in this task
			}
			bt.Logger.Warnf("Step '%s' in task '%s' failed, but IgnoreErr is true. Continuing task.", currentStep.Name(), bt.NameField)
		}
	}

	if taskFailedOverall {
		bt.Logger.Warnf("Task [%s] completed with errors (IgnoreErr: %v). See result.Errors for details.", bt.NameField, bt.Runtime.Arg.IgnoreErr)
		// If we are here and taskFailedOverall is true, it means all errors were ignorable.
		// The result.Status would have been set to ModuleResultFailed by AddError calls.
		// If it's somehow still Pending, mark it Failed.
		if result.Status == ending.ModuleResultPending {
		    result.SetStatus(ending.ModuleResultFailed)
		}
		if result.Message == "" {
			result.SetMessage(fmt.Sprintf("Task '%s' completed with ignorable errors.", bt.NameField))
		}
	} else {
		bt.Logger.Infof("Task [%s] completed successfully.", bt.NameField)
		if result.Status == ending.ModuleResultPending { // Ensure success if no errors occurred and not already set
			result.SetStatus(ending.ModuleResultSuccess)
			result.SetMessage(fmt.Sprintf("Task '%s' completed successfully.", bt.NameField))
		}
	}
}

func (bt *BaseTask) Until(runtime *krt.ClusterRuntime) (done bool, err error) {
	if bt.Logger == nil && runtime != nil && runtime.Log != nil {
		bt.Logger = runtime.Log.WithField("task_early_until", bt.NameField)
	}
	if bt.Logger != nil {
		bt.Logger.Debugf("BaseTask.Until called for %s (defaulting to true)", bt.NameField)
	}
	return true, nil
}

func (bt *BaseTask) AddStep(s step.Step) {
	bt.steps = append(bt.steps, s)
}

func (bt *BaseTask) Steps() []step.Step {
	return bt.steps
}

// Ensure BaseTask implements the Task interface.
var _ Task = (*BaseTask)(nil)
