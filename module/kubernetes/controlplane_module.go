package controlplane // Changed package name to 'controlplane' for clarity

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// ControlPlaneModuleSpec defines the expected spec structure for ControlPlaneModule.
// This matches what InstallPipeline's factory prepares.
type ControlPlaneModuleSpec struct {
	Kubernetes           config.KubernetesSpec
	ControlPlaneEndpoint config.ControlPlaneEndpointSpec
}

type ControlPlaneModule struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *krt.KubeRuntime
	ModuleSpec       *ControlPlaneModuleSpec // Store the typed spec
	Logger           *logrus.Entry
	tasks            []task.Task
	postHooks        []module.HookFn
}

func NewControlPlaneModule() module.Module {
	return &ControlPlaneModule{
		NameField:        "kubernetes-controlplane",
		DescriptionField: "Manages the Kubernetes control plane setup.",
		tasks:            make([]task.Task, 0),
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *ControlPlaneModule) Name() string { return m.NameField }
func (m *ControlPlaneModule) Description() string { return m.DescriptionField }

func (m *ControlPlaneModule) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if m.Logger == nil {
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_early", m.NameField)
	}
	m.Logger.Debug("IsSkip called.")
	if runtime == nil || runtime.Cluster == nil {
		return false, fmt.Errorf("runtime or cluster spec is nil for IsSkip check in %s", m.NameField)
	}
	// Example: Skip if no control-plane hosts defined
	if len(runtime.RoleHosts()["control-plane"]) == 0 {
		m.Logger.Info("Skipping: No hosts assigned to 'control-plane' role.")
		return true, nil
	}
	return false, nil
}

func (m *ControlPlaneModule) Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.KubeRuntime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	m.Logger.Info("Default called: runtime and logger set.")

	spec, ok := moduleSpec.(*ControlPlaneModuleSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *ControlPlaneModuleSpec, got %T", m.NameField, moduleSpec)
	}
	m.ModuleSpec = spec
	m.Logger.Infof("Module spec configured: K8s Version '%s', CP Endpoint '%s:%d'",
		m.ModuleSpec.Kubernetes.Version, m.ModuleSpec.ControlPlaneEndpoint.Address, m.ModuleSpec.ControlPlaneEndpoint.Port)
	return nil
}

func (m *ControlPlaneModule) AutoAssert() error {
	m.Logger.Info("AutoAssert called.")
	if m.KubeRuntime == nil || m.ModuleSpec == nil {
		return fmt.Errorf("runtime or moduleSpec not initialized for %s", m.NameField)
	}
	if len(m.KubeRuntime.RoleHosts()["control-plane"]) == 0 {
		return fmt.Errorf("control-plane hosts are required for module %s", m.NameField)
	}
	// Add more assertions, e.g. Kubernetes version format, non-empty endpoint address if not domain, etc.
	m.Logger.Info("AutoAssert completed successfully.")
	return nil
}

func (m *ControlPlaneModule) Init() error {
	m.Logger.Info("Init called (task assembly).")
	// Example: Add tasks for kubeadm init, kubeadm join for control planes, applying addons etc.
	m.Logger.Info("No concrete tasks assembled in this skeleton Init for ControlPlaneModule.")
	return nil
}

func (m *ControlPlaneModule) Run(result *ending.ModuleResult) {
	m.Logger.Info("Run called.")
	// ... task execution loop ...
	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped {
		result.SetStatus(ending.ModuleResultSuccess)
		result.SetMessage(fmt.Sprintf("%s conceptual run completed successfully.", m.NameField))
	}
	m.Logger.Info("Run completed.")
}

func (m *ControlPlaneModule) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	m.Logger.Info("Until called (placeholder, returning true).")
	return true, nil
}

func (m *ControlPlaneModule) CallPostHook(res *ending.ModuleResult) error {
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

func (m *ControlPlaneModule) Is() module.Type {
	return module.TaskModuleType
}

func (m *ControlPlaneModule) Slogan() string {
	if m.ModuleSpec != nil {
		return fmt.Sprintf("Setting up Kubernetes Control Plane (v%s)...", m.ModuleSpec.Kubernetes.Version)
	}
	return "Setting up Kubernetes Control Plane..."
}

func (m *ControlPlaneModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

var _ module.Module = (*ControlPlaneModule)(nil)
