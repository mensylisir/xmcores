package task

import (
	"github.com/mensylisir/xmcores/pipeline/ending" // For ModuleResult (used by Task.Run)
	"github.com/mensylisir/xmcores/runtime"         // For ClusterRuntime
	"github.com/mensylisir/xmcores/step"
	// No module import needed unless using module.HookFn or module.Type directly
)

// Task represents a specific unit of work within a Module, typically composed of steps.
type Task interface {
	Name() string
	Description() string
	Slogan() string
	// Tasks generally don't have types like GoroutineModuleType, but could have their own TaskType enum if needed.

	// IsSkip allows a task to dynamically determine if it should be skipped.
	IsSkip(runtime *runtime.ClusterRuntime) (bool, error)

	// Default initializes the task with runtime context, its specific configuration (taskSpec),
	// and caches. taskSpec is type-asserted and stored by the concrete task.
	Default(runtime *runtime.ClusterRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error

	// AutoAssert performs pre-run validation and prerequisite checks.
	AutoAssert(runtime *runtime.ClusterRuntime) error

	// Init is for the task's internal setup, primarily to assemble its constituent steps.
	Init() error

	// Run executes the core logic of the task, often by running its steps.
	// It populates/updates the provided ModuleResult (from its parent module).
	Run(result *ending.ModuleResult)

	// Until is for tasks that might loop, wait for a condition, or run asynchronously.
	Until(runtime *runtime.ClusterRuntime) (done bool, err error)

	// Tasks typically don't have their own post-hooks separate from the module's.
	// If a task needs cleanup, it should handle it within its Run or be a specific step.

	// Step management methods
	AddStep(s step.Step)
	Steps() []step.Step
}
