package containerruntime

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

type ContainerdModule struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *krt.KubeRuntime
	ModuleSpec       *config.KubernetesSpec // Assuming this spec contains ContainerManager info
	Logger           *logrus.Entry
	tasks            []task.Task
	postHooks        []module.HookFn
}

func NewContainerdModule() module.Module {
	return &ContainerdModule{
		NameField:        "container-runtime-containerd",
		DescriptionField: "Installs and configures Containerd.",
		tasks:            make([]task.Task, 0),
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *ContainerdModule) Name() string { return m.NameField }
func (m *ContainerdModule) Description() string { return m.DescriptionField }

func (m *ContainerdModule) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if m.Logger == nil {
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_early", m.NameField)
	}
	m.Logger.Debug("IsSkip called.")
	if runtime == nil || runtime.Cluster == nil {
		return false, fmt.Errorf("runtime or cluster spec is nil for IsSkip check in %s", m.NameField)
	}
	if runtime.Cluster.Kubernetes.ContainerManager != "containerd" {
		m.Logger.Infof("Skipping: ContainerManager is '%s', not 'containerd'.", runtime.Cluster.Kubernetes.ContainerManager)
		return true, nil
	}
	return false, nil
}

func (m *ContainerdModule) Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.KubeRuntime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	m.Logger.Info("Default called: runtime and logger set.")

	spec, ok := moduleSpec.(*config.KubernetesSpec) // Pipeline passes KubernetesSpec
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.KubernetesSpec, got %T", m.NameField, moduleSpec)
	}
	m.ModuleSpec = spec
	m.Logger.Infof("Module spec configured: ContainerManager '%s'", m.ModuleSpec.ContainerManager)
	return nil
}

func (m *ContainerdModule) AutoAssert() error {
	m.Logger.Info("AutoAssert called.")
	if m.KubeRuntime == nil || m.ModuleSpec == nil {
		return fmt.Errorf("runtime or moduleSpec not initialized for %s", m.NameField)
	}
	if m.ModuleSpec.ContainerManager != "containerd" {
		return fmt.Errorf("assertion failed: ContainerManager is '%s', expected 'containerd'", m.ModuleSpec.ContainerManager)
	}
	if len(m.KubeRuntime.AllHosts()) == 0 { // Containerd typically installed on all nodes
		return fmt.Errorf("no hosts available in runtime to install containerd on")
	}
	m.Logger.Info("AutoAssert completed successfully.")
	return nil
}

func (m *ContainerdModule) Init() error {
	m.Logger.Info("Init called (task assembly).")
	// Example: Add tasks to install and configure containerd on all hosts.
	// These tasks would use m.KubeRuntime to iterate over AllHosts()
	// and m.ModuleSpec (or a more specific Containerd config part from ClusterSpec)
	// for version, registry mirror settings etc.
	m.Logger.Info("No concrete tasks assembled in this skeleton Init for ContainerdModule.")
	return nil
}

func (m *ContainerdModule) Run(result *ending.ModuleResult) {
	m.Logger.Info("Run called.")
	// ... task execution loop ...
	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped {
		result.SetStatus(ending.ModuleResultSuccess)
		result.SetMessage(fmt.Sprintf("%s conceptual run completed successfully.", m.NameField))
	}
	m.Logger.Info("Run completed.")
}

func (m *ContainerdModule) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	m.Logger.Info("Until called (placeholder, returning true).")
	return true, nil
}

func (m *ContainerdModule) CallPostHook(res *ending.ModuleResult) error {
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

func (m *ContainerdModule) Is() module.Type {
	return module.TaskModuleType
}

func (m *ContainerdModule) Slogan() string {
	version := "default" // Placeholder, real version would come from spec
	if m.ModuleSpec != nil {
		// If Containerd version was part of KubernetesSpec or a dedicated ContainerdSpec
		// version = m.ModuleSpec.ContainerdVersionField (example)
	}
	return fmt.Sprintf("Setting up Containerd runtime (version %s)...", version)
}

func (m *ContainerdModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

var _ module.Module = (*ContainerdModule)(nil)
