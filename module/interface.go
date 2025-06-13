package module

import (
	"github.com/mensylisir/xmcores/pipeline/ending"
	"github.com/mensylisir/xmcores/runtime" // Specifically *runtime.ClusterRuntime
)

// HookFn defines the signature for functions that can be appended as post-hooks to a module.
type HookFn func(moduleResult *ending.ModuleResult) error

// Type defines the category or execution model of a module.
type Type int

const (
	TaskModuleType      Type = iota // Module composed of tasks that are run sequentially.
	GoroutineModuleType             // Module that might run its main logic in a goroutine.
	// Add other types as needed
)

// String returns a string representation of the Module Type.
func (t Type) String() string {
	switch t {
	case TaskModuleType:
		return "TaskModule"
	case GoroutineModuleType:
		return "GoroutineModule"
	default:
		return "UnknownModuleType"
	}
}

// Module represents a logical unit of work within a Pipeline.
type Module interface {
	Name() string        // Returns the unique name of the module.
	Description() string // A human-readable summary of what the module does.
	Slogan() string      // A brief message logged when the module starts.
	Is() Type            // Returns the module type (e.g., TaskModuleType).

	// IsSkip allows a module to dynamically determine if it should be skipped.
	// Called by the pipeline before Default/Init.
	IsSkip(runtime *runtime.ClusterRuntime) (bool, error)

	// Default initializes the module with runtime context, its specific configuration (moduleSpec),
	// and caches. The moduleSpec is type-asserted and stored by the concrete module.
	// Called by the pipeline after IsSkip (if not skipped).
	Default(runtime *runtime.ClusterRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error

	// AutoAssert performs pre-run validation and prerequisite checks using the stored runtime and spec.
	// Called by the pipeline after Default.
	AutoAssert(runtime *runtime.ClusterRuntime) error

	// Init is for the module's internal setup, primarily to assemble its constituent tasks or steps.
	// Called by the pipeline after AutoAssert.
	Init() error

	// Run executes the core logic of the module, often by running its tasks/steps.
	// It populates the provided ModuleResult based on its execution outcome.
	// Called by the pipeline after Init.
	Run(result *ending.ModuleResult)

	// Until is for modules that might loop, wait for a condition, or run asynchronously.
	// Called by the pipeline after Run. It should return true if the module's overall work is complete.
	Until(runtime *runtime.ClusterRuntime) (done bool, err error)

	// CallPostHook executes any registered post-execution hooks.
	// Called by the pipeline after Run (and Until, if applicable) completes.
	CallPostHook(res *ending.ModuleResult) error

	// AppendPostHook allows adding functions to be called after the module's main execution.
	AppendPostHook(hookFn HookFn)
}
