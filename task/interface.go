package task

import (
	// Adjust path to KubeRuntime if it's not in "github.com/mensylisir/xmcores/runtime"
	krt "github.com/mensylisir/xmcores/runtime" // Assuming KubeRuntime is the runtime tasks will interact with
	"github.com/mensylisir/xmcores/pipeline/ending" // For ModuleResult, or define a TaskResult
	"github.com/mensylisir/xmcores/step"
	// "github.com/mensylisir/xmcores/module" // Not importing module.HookFn for now
)

// TaskResult could be defined similar to ModuleResult if tasks need distinct result structures.
// For now, tasks will populate the ModuleResult passed from their parent module's Run method.
// type TaskResult struct {
// Status ending.ModuleResultStatus // Can reuse status enum
// Message string
// Errors  []error
// }

// TaskHookFn could be defined if tasks have their own hook system.
// type TaskHookFn func(taskResult *TaskResult) error

// Task represents a specific unit of work within a Module, typically composed of steps.
type Task interface {
	Name() string
	Description() string // Optional, but useful for logging/UI.

	// IsSkip allows a task to dynamically determine if it should be skipped.
	// It might check conditions in the runtime or its specific configuration.
	IsSkip(runtime *krt.KubeRuntime) (bool, error)

	// Default is called to set up the task with necessary runtime, its specific configuration (taskSpec),
	// and potentially module/task caches.
	// moduleCache and taskCache are placeholders for future caching mechanisms.
	Default(runtime *krt.KubeRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error

	// AutoAssert is for pre-run checks or assertions the task needs to validate based on its spec and runtime.
	AutoAssert() error

	// Init is for the task's internal initialization, primarily for setting up its steps.
	// This is called after Default and AutoAssert.
	Init() error

	// Run executes the core logic of the task (typically its steps).
	// It takes a ModuleResult pointer (from its parent module), which it should populate or update
	// based on its execution outcome. If a task has sub-tasks, it might create a temporary
	// result for them and then consolidate.
	Run(result *ending.ModuleResult) // Task contributes to the Module's result

	// Until is for tasks that might loop, wait for a condition, or run asynchronously.
	// (Similar to Module.Until)
	Until(runtime *krt.KubeRuntime) (done bool, err error) // Runtime access

	// Slogan provides a short, catchy phrase or log line for UI/logging when the task starts.
	Slogan() string

	// Methods for step management (from previous interface, still relevant)
	AddStep(s step.Step)
	Steps() []step.Step
}
