package network // Package name 'network'

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

type CalicoCNIModule struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *krt.KubeRuntime
	ModuleSpec       *config.NetworkSpec // Store the typed spec
	Logger           *logrus.Entry
	tasks            []task.Task
	postHooks        []module.HookFn
}

func NewCalicoCNIModule() module.Module {
	return &CalicoCNIModule{
		NameField:        "cni-calico",
		DescriptionField: "Installs and configures Calico CNI.",
		tasks:            make([]task.Task, 0),
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *CalicoCNIModule) Name() string { return m.NameField }
func (m *CalicoCNIModule) Description() string { return m.DescriptionField }

func (m *CalicoCNIModule) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if m.Logger == nil {
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_early", m.NameField)
	}
	m.Logger.Debug("IsSkip called.")
	if runtime == nil || runtime.Cluster == nil {
		return false, fmt.Errorf("runtime or cluster spec is nil for IsSkip check in %s", m.NameField)
	}
	if runtime.Cluster.Network.Plugin != "calico" {
		m.Logger.Infof("Skipping: Network plugin is '%s', not 'calico'.", runtime.Cluster.Network.Plugin)
		return true, nil
	}
	return false, nil
}

func (m *CalicoCNIModule) Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.KubeRuntime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	m.Logger.Info("Default called: runtime and logger set.")

	spec, ok := moduleSpec.(*config.NetworkSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.NetworkSpec, got %T", m.NameField, moduleSpec)
	}
	m.ModuleSpec = spec
	m.Logger.Infof("Module spec configured: CNI Plugin '%s', PodCIDR '%s'",
		m.ModuleSpec.Plugin, m.ModuleSpec.KubePodsCIDR)
	return nil
}

func (m *CalicoCNIModule) AutoAssert() error {
	m.Logger.Info("AutoAssert called.")
	if m.KubeRuntime == nil || m.ModuleSpec == nil {
		return fmt.Errorf("runtime or moduleSpec not initialized for %s", m.NameField)
	}
	if m.ModuleSpec.Plugin != "calico" {
		return fmt.Errorf("assertion failed: Network plugin is '%s', expected 'calico'", m.ModuleSpec.Plugin)
	}
	if m.ModuleSpec.KubePodsCIDR == "" {
		return fmt.Errorf("KubePodsCIDR is required for Calico CNI module")
	}
	// Potentially check BlockSize if it has specific constraints for Calico
	m.Logger.Info("AutoAssert completed successfully.")
	return nil
}

func (m *CalicoCNIModule) Init() error {
	m.Logger.Info("Init called (task assembly).")
	// Example: Add tasks to download Calico manifests, customize them, and apply.
	m.Logger.Info("No concrete tasks assembled in this skeleton Init for CalicoCNIModule.")
	return nil
}

func (m *CalicoCNIModule) Run(result *ending.ModuleResult) {
	m.Logger.Info("Run called.")
	// ... task execution loop ...
	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped {
		result.SetStatus(ending.ModuleResultSuccess)
		result.SetMessage(fmt.Sprintf("%s conceptual run completed successfully.", m.NameField))
	}
	m.Logger.Info("Run completed.")
}

func (m *CalicoCNIModule) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	m.Logger.Info("Until called (placeholder, returning true).")
	return true, nil
}

func (m *CalicoCNIModule) CallPostHook(res *ending.ModuleResult) error {
	m.Logger.Info("CallPostHook called.")
	var firstError error
	for i, hook := range m.postHooks {
		m.Logger.Debugf("Executing post-hook %d", i+1)
		if err := hook(res); err != nil {
			m.Logger.Errorf("Error executing post-hook %d: %v", i+1, err)
			if firstError == nil {firstError = err}
		}
	}
	m.Logger.Info("All post-hooks executed.")
	return firstError
}

func (m *CalicoCNIModule) Is() module.Type {
	return module.TaskModuleType
}

func (m *CalicoCNIModule) Slogan() string {
	if m.ModuleSpec != nil {
		return fmt.Sprintf("Installing Calico CNI (PodCIDR: %s)...", m.ModuleSpec.KubePodsCIDR)
	}
	return "Installing Calico CNI..."
}

func (m *CalicoCNIModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

var _ module.Module = (*CalicoCNIModule)(nil)
