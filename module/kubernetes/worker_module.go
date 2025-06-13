package worker // Package name 'worker'

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// WorkerModuleSpec defines the expected spec structure for WorkerModule.
// This matches what InstallPipeline's factory prepares.
type WorkerModuleSpec struct {
	Kubernetes           config.KubernetesSpec
	ControlPlaneEndpoint config.ControlPlaneEndpointSpec
	// Potentially other fields like specific worker configurations, taints, labels etc.
}

type WorkerModule struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *krt.KubeRuntime
	ModuleSpec       *WorkerModuleSpec // Store the typed spec
	Logger           *logrus.Entry
	tasks            []task.Task
	postHooks        []module.HookFn
}

func NewWorkerModule() module.Module {
	return &WorkerModule{
		NameField:        "kubernetes-worker",
		DescriptionField: "Manages Kubernetes worker node setup and joining the cluster.",
		tasks:            make([]task.Task, 0),
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *WorkerModule) Name() string { return m.NameField }
func (m *WorkerModule) Description() string { return m.DescriptionField }

func (m *WorkerModule) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if m.Logger == nil {
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_early", m.NameField)
	}
	m.Logger.Debug("IsSkip called.")
	if runtime == nil || runtime.Cluster == nil {
		return false, fmt.Errorf("runtime or cluster spec is nil for IsSkip check in %s", m.NameField)
	}
	if len(runtime.RoleHosts()["worker"]) == 0 {
		m.Logger.Info("Skipping: No hosts assigned to 'worker' role.")
		return true, nil
	}
	return false, nil
}

func (m *WorkerModule) Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.KubeRuntime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	m.Logger.Info("Default called: runtime and logger set.")

	spec, ok := moduleSpec.(*WorkerModuleSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *WorkerModuleSpec, got %T", m.NameField, moduleSpec)
	}
	m.ModuleSpec = spec
	m.Logger.Infof("Module spec configured: K8s Version '%s' for workers, joining CP at '%s'",
		m.ModuleSpec.Kubernetes.Version, m.ModuleSpec.ControlPlaneEndpoint.Address)
	return nil
}

func (m *WorkerModule) AutoAssert() error {
	m.Logger.Info("AutoAssert called.")
	if m.KubeRuntime == nil || m.ModuleSpec == nil {
		return fmt.Errorf("runtime or moduleSpec not initialized for %s", m.NameField)
	}
	if len(m.KubeRuntime.RoleHosts()["worker"]) == 0 {
		return fmt.Errorf("worker hosts are required for module %s but not found in roleGroup 'worker'", m.NameField)
	}
	if m.ModuleSpec.ControlPlaneEndpoint.Address == "" && m.ModuleSpec.ControlPlaneEndpoint.Domain == "" {
		return fmt.Errorf("controlPlaneEndpoint address or domain must be specified for workers to join")
	}
	m.Logger.Info("AutoAssert completed successfully.")
	return nil
}

func (m *WorkerModule) Init() error {
	m.Logger.Info("Init called (task assembly).")
	// Example: Add tasks for kubeadm join on each worker node.
	// workerHosts := m.KubeRuntime.RoleHosts()["worker"]
	// for _, host := range workerHosts {
	//    joinTask := workerTasks.NewJoinNodeTask(host, m.ModuleSpec.ControlPlaneEndpoint, m.ModuleSpec.Kubernetes.Version)
	//    m.tasks = append(m.tasks, joinTask)
	// }
	m.Logger.Info("No concrete tasks assembled in this skeleton Init for WorkerModule.")
	return nil
}

func (m *WorkerModule) Run(result *ending.ModuleResult) {
	m.Logger.Info("Run called.")
	// ... task execution loop ...
	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped {
		result.SetStatus(ending.ModuleResultSuccess)
		result.SetMessage(fmt.Sprintf("%s conceptual run completed successfully.", m.NameField))
	}
	m.Logger.Info("Run completed.")
}

func (m *WorkerModule) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	m.Logger.Info("Until called (placeholder, returning true).")
	return true, nil
}

func (m *WorkerModule) CallPostHook(res *ending.ModuleResult) error {
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

func (m *WorkerModule) Is() module.Type {
	return module.TaskModuleType
}

func (m *WorkerModule) Slogan() string {
	if m.ModuleSpec != nil {
		return fmt.Sprintf("Configuring Kubernetes Worker Nodes (for K8s v%s)...", m.ModuleSpec.Kubernetes.Version)
	}
	return "Configuring Kubernetes Worker Nodes..."
}

func (m *WorkerModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

var _ module.Module = (*WorkerModule)(nil)
