package module

import (
	// Adjust path to KubeRuntime if it's not in "github.com/mensylisir/xmcores/runtime"
	// It is, so the import below is correct.
	krt "github.com/mensylisir/xmcores/runtime" // Alias to avoid conflict if KubeRuntime was in package runtime
	"github.com/mensylisir/xmcores/pipeline/ending"
)

// HookFn defines the signature for functions that can be appended as post-hooks to a module.
type HookFn func(moduleResult *ending.ModuleResult) error

// Type defines the category or execution model of a module.
type Type int // Or string constants

const (
	TaskModuleType      Type = iota // Module composed of tasks that are run sequentially.
	GoroutineModuleType             // Module that might run its main logic in a goroutine (e.g., for monitoring or parallel tasks).
	// Add other types like "ConditionalModuleType", "IterativeModuleType" etc. as needed.
)

// Module represents a logical unit of work within a Pipeline.
// It has a richer lifecycle including hooks and different potential types.
type Module interface {
	Name() string       // Returns the unique name of the module.
	Description() string // Optional: A human-readable description. Can be removed if not used.

	// IsSkip allows a module to dynamically determine if it should be skipped.
	// It might check conditions in the runtime or its specific configuration.
	IsSkip(runtime *krt.KubeRuntime) (bool, error) // Added runtime access for skip logic

	// Default is called to set up the module with necessary runtime, caches, and potentially
	// to allow the module to pull default configurations or prepare itself.
	// pipelineCache and moduleCache are placeholders for future caching mechanisms.
	Default(runtime *krt.KubeRuntime, pipelineCache interface{}, moduleCache interface{}) error

	// AutoAssert is intended for pre-run checks or assertions that the module needs to validate
	// before its main execution logic. Could use the stored runtime/config.
	AutoAssert() error // Changed to return error for assertion failures

	// Init is for the module's internal initialization, such as setting up its tasks or steps.
	// This is called after Default and AutoAssert.
	Init() error

	// Run executes the core logic of the module.
	// It takes a ModuleResult pointer, which it should populate based on its execution outcome.
	Run(result *ending.ModuleResult)

	// Until is for modules that might loop, wait for a condition, or run asynchronously.
	// It should return true if the module's condition is met or its execution is complete.
	// An error can be returned if the waiting/checking process itself fails.
	// If module is not asynchronous or long-running, it can return true immediately or not implement meaningfully.
	Until(runtime *krt.KubeRuntime) (done bool, err error) // Added runtime access

	// CallPostHook executes any registered post-execution hooks.
	// It is typically called by the pipeline after the module's Run (and potentially Until) completes.
	CallPostHook(res *ending.ModuleResult) error

	// Is returns the type of the module (e.g., TaskModuleType, GoroutineModuleType).
	Is() Type

	// Slogan provides a short, catchy phrase or log line for UI/logging when the module starts.
	Slogan() string // Changed to return string

	// AppendPostHook allows adding functions to be called after the module's main execution.
	AppendPostHook(hookFn HookFn)
}
