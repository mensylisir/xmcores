package precheck

import (
	"fmt"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime" // Alias for ClusterRuntime
	"github.com/sirupsen/logrus"
)

type GreetingsModule struct {
	NameField        string
	DescriptionField string
	Logger           *logrus.Entry
	Runtime          *krt.ClusterRuntime
	// No specific spec needed for Greetings
	postHooks []module.HookFn
}

// NewGreetingsModule is the constructor for GreetingsModule.
func NewGreetingsModule() module.Module {
	return &GreetingsModule{
		NameField:        "greetings",
		DescriptionField: "Displays greetings and initial information.",
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *GreetingsModule) Name() string { return m.NameField }
func (m *GreetingsModule) Description() string { return m.DescriptionField }
func (m *GreetingsModule) Slogan() string { return "Hello from xmcores! Initial precheck module." }
func (m *GreetingsModule) Is() module.Type { return module.TaskModuleType } // Or a simpler type if no tasks

func (m *GreetingsModule) IsSkip(runtime *krt.ClusterRuntime) (bool, error) {
	if m.Logger == nil { // Ensure logger is available for IsSkip messages
		m.Logger = runtime.Log.WithField("module_early_skip", m.NameField)
	}
	m.Logger.Info("IsSkip called for GreetingsModule, returning false.")
	return false, nil
}

func (m *GreetingsModule) Default(runtime *krt.ClusterRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.Runtime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	// No moduleSpec to type-assert or store for this simple module
	m.Logger.Info("Default called: runtime and logger set.")
	return nil
}

func (m *GreetingsModule) AutoAssert(runtime *krt.ClusterRuntime) error {
	if m.Logger == nil { // Ensure logger from Default is used
		return fmt.Errorf("logger not initialized for AutoAssert in %s", m.NameField)
	}
	m.Logger.Info("AutoAssert called: No specific assertions for GreetingsModule.")
	if m.Runtime == nil {
		return fmt.Errorf("runtime not initialized for %s (AutoAssert)", m.NameField)
	}
	return nil
}

func (m *GreetingsModule) Init() error {
	if m.Logger == nil {
		return fmt.Errorf("logger not initialized for Init in %s", m.NameField)
	}
	m.Logger.Info("Init called: No tasks to initialize for GreetingsModule.")
	return nil
}

func (m *GreetingsModule) Run(result *ending.ModuleResult) {
	if m.Logger == nil { // Should have been set in Default
		// Fallback, but indicates an issue in setup
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_run_fallback", m.NameField)
		m.Logger.Warn("Logger was not initialized via Default method prior to Run.")
	}
	m.Logger.Info("Run called: Displaying greetings!")

	// Simulate some action
	fmt.Printf("\n*****************************************************************\n")
	fmt.Printf("* Welcome to xmcores! Cluster: %s\n", m.Runtime.Cluster.Kubernetes.ClusterName)
	fmt.Printf("* Pipeline: %s - Module: %s\n", m.Runtime.Base.ClusterName, m.Name()) // Example, assuming Base has ClusterName
	fmt.Printf("* Kubernetes Version Target: %s\n", m.Runtime.Cluster.Kubernetes.Version)
	fmt.Printf("*****************************************************************\n\n")

	result.SetMessage("Greetings displayed successfully.")
	result.SetStatus(ending.ModuleResultSuccess)
	m.Logger.Info("Run completed: Greetings displayed.")
}

func (m *GreetingsModule) Until(runtime *krt.ClusterRuntime) (done bool, err error) {
	m.Logger.Info("Until called: GreetingsModule completes immediately.")
	return true, nil
}

func (m *GreetingsModule) CallPostHook(res *ending.ModuleResult) error {
	m.Logger.Info("CallPostHook called.")
	var firstError error
	for i, hook := range m.postHooks {
		m.Logger.Debugf("Executing post-hook %d for module %s", i+1, m.NameField)
		if err := hook(res); err != nil {
			m.Logger.Errorf("Error executing post-hook %d for module %s: %v", i+1, m.NameField, err)
			if firstError == nil {
				firstError = err
			}
		}
	}
	m.Logger.Info("All post-hooks executed for GreetingsModule.")
	return firstError
}

func (m *GreetingsModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

// Ensure GreetingsModule implements module.Module
var _ module.Module = (*GreetingsModule)(nil)
