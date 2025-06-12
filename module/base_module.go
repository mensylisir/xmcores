package module

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/common" // For logger field constants
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// BaseModule provides a basic implementation for the Module interface.
// It can be embedded in concrete module implementations.
type BaseModule struct {
	name        string
	description string
	tasks       []task.Task
}

// NewBaseModule creates a new BaseModule.
func NewBaseModule(name, description string) BaseModule {
	return BaseModule{
		name:        name,
		description: description,
		tasks:       make([]task.Task, 0),
	}
}

// Name returns the name of the module.
func (bm *BaseModule) Name() string {
	return bm.name
}

// SetName sets the name of the module.
func (bm *BaseModule) SetName(name string) {
	bm.name = name
}

// Description returns the description of the module.
func (bm *BaseModule) Description() string {
	return bm.description
}

// SetDescription sets the description of the module.
func (bm *BaseModule) SetDescription(desc string) {
	bm.description = desc
}

// Tasks returns the list of tasks in the module.
func (bm *BaseModule) Tasks() []task.Task {
	// Return a copy to prevent external modification
	t := make([]task.Task, len(bm.tasks))
	copy(t, bm.tasks)
	return t
}

// AddTask adds a task to the module's execution list.
func (bm *BaseModule) AddTask(t task.Task) {
	bm.tasks = append(bm.tasks, t)
}

// SetTasks sets the list of tasks for the module.
func (bm *BaseModule) SetTasks(tasks []task.Task) {
	bm.tasks = make([]task.Task, len(tasks))
	copy(bm.tasks, tasks)
}

// Init provides a default Init implementation that initializes all added tasks.
func (bm *BaseModule) Init(rt runtime.Runtime, log *logrus.Entry) error {
	log.Debugf("BaseModule.Init called for module: %s. Initializing %d tasks.", bm.Name(), len(bm.tasks))
	if len(bm.tasks) == 0 {
		log.Warn("No tasks defined for this module.")
		// Depending on requirements, this could be an error.
	}
	for i, t := range bm.tasks {
		taskLog := log.WithFields(logrus.Fields{
			common.LogFieldTaskName: t.Name(),
			"task_index":            fmt.Sprintf("%d/%d", i+1, len(bm.tasks)),
		})
		taskLog.Infof("Initializing task: %s (%s)", t.Name(), t.Description())
		if err := t.Init(rt, taskLog); err != nil {
			taskLog.Errorf("Failed to initialize task %s: %v", t.Name(), err)
			return fmt.Errorf("failed to initialize task %s (index %d) in module %s: %w", t.Name(), i, bm.Name(), err)
		}
	}
	log.Infof("All %d tasks for module %s initialized successfully.", len(bm.tasks), bm.Name())
	return nil
}

// Execute provides a default Execute implementation that runs all tasks sequentially.
func (bm *BaseModule) Execute(rt runtime.Runtime, log *logrus.Entry) error {
	log.Infof("Executing module: %s (%s)", bm.Name(), bm.Description())
	if len(bm.tasks) == 0 {
		log.Warnf("Module %s has no tasks to execute.", bm.Name())
		return nil // Or an error if modules must have tasks
	}

	var overallModuleFailed bool
	var taskErrors []string

	for i, currentTask := range bm.tasks {
		taskLog := log.WithFields(logrus.Fields{
			common.LogFieldTaskName: currentTask.Name(),
			"task_index":            fmt.Sprintf("%d/%d", i+1, len(bm.tasks)),
		})
		taskLog.Infof("Executing task: %s (%s)", currentTask.Name(), currentTask.Description())
		fmt.Printf("===> Executing Task: %s (%s)\n", currentTask.Name(), currentTask.Description())


		taskErr := currentTask.Execute(rt, taskLog)

		// Call Post for the current task, regardless of its success/failure
		postLog := taskLog.WithField("sub_phase", "post_execute")
		if postErr := currentTask.Post(rt, postLog, taskErr); postErr != nil {
			postLog.Errorf("Error during Post-Execute for task %s: %v", currentTask.Name(), postErr)
			taskErrors = append(taskErrors, fmt.Sprintf("post-execute error for task %s: %v", currentTask.Name(), postErr))
			overallModuleFailed = true
		}

		if taskErr != nil {
			taskLog.Errorf("Task %s failed: %v", currentTask.Name(), taskErr)
			fmt.Printf("===> Task FAILED: %s. Error: %v\n", currentTask.Name(), taskErr)
			taskErrors = append(taskErrors, fmt.Sprintf("task %s error: %v", currentTask.Name(), taskErr))
			overallModuleFailed = true
			if !rt.IgnoreError() {
				log.Errorf("Module %s failed at task %s due to error: %v. Halting module execution.", bm.Name(), currentTask.Name(), taskErr)
				return fmt.Errorf("module %s failed at task %s: %w", bm.Name(), currentTask.Name(), taskErr)
			}
			log.Warnf("Task %s failed but IgnoreError is true. Continuing module execution.", currentTask.Name())
		} else {
			taskLog.Infof("Task %s completed successfully.", currentTask.Name())
			fmt.Printf("===> Task SUCCEEDED: %s.\n", currentTask.Name())
		}
	}

	if overallModuleFailed {
		log.Errorf("Module %s completed with one or more errors: %s", bm.Name(), strings.Join(taskErrors, "; "))
		return fmt.Errorf("module %s failed with errors: %s", bm.Name(), strings.Join(taskErrors, "; "))
	}

	log.Infof("Module %s completed successfully.", bm.Name())
	return nil
}

// Post provides a default no-op Post implementation for the module itself.
func (bm *BaseModule) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
	log.Debugf("BaseModule.Post called for module %s.", bm.Name())
	return nil
}
