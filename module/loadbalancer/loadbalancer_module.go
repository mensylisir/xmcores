package loadbalancer

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

type HAProxyKeepalivedModule struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *krt.KubeRuntime
	ModuleSpec       *config.ControlPlaneEndpointSpec // Stores the relevant part of ClusterConfig
	Logger           *logrus.Entry
	tasks            []task.Task
	postHooks        []module.HookFn
}

func NewHAProxyKeepalivedModule() module.Module {
	return &HAProxyKeepalivedModule{
		NameField:        "loadbalancer-haproxy-keepalived",
		DescriptionField: "Sets up HAProxy and Keepalived for control plane load balancing.",
		tasks:            make([]task.Task, 0),
		postHooks:        make([]module.HookFn, 0),
	}
}

func (m *HAProxyKeepalivedModule) Name() string { return m.NameField }
func (m *HAProxyKeepalivedModule) Description() string { return m.DescriptionField }

func (m *HAProxyKeepalivedModule) IsSkip(runtime *krt.KubeRuntime) (bool, error) {
	if m.Logger == nil {
		m.Logger = logrus.NewEntry(logrus.New()).WithField("module_early", m.NameField)
	}
	m.Logger.Debug("IsSkip called.")
	if runtime == nil || runtime.Cluster == nil {
		return false, fmt.Errorf("runtime or cluster spec is nil for IsSkip check in %s", m.NameField)
	}
	// This module should be skipped if LB is not enabled or if it's external
	if !runtime.Cluster.ControlPlaneEndpoint.LoadBalancer.Enable ||
	   runtime.Cluster.ControlPlaneEndpoint.LoadBalancer.Type != "haproxy-keepalived" {
		m.Logger.Infof("Skipping module: LoadBalancer.Enable is %v, Type is '%s'. Required: Enable=true, Type='haproxy-keepalived'.",
			runtime.Cluster.ControlPlaneEndpoint.LoadBalancer.Enable,
			runtime.Cluster.ControlPlaneEndpoint.LoadBalancer.Type)
		return true, nil
	}
	if len(runtime.RoleHosts()["loadbalancer"]) == 0 {
		m.Logger.Info("Skipping module: No hosts assigned to 'loadbalancer' role.")
		return true, nil
	}
	return false, nil
}

func (m *HAProxyKeepalivedModule) Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error {
	m.KubeRuntime = runtime
	m.Logger = runtime.Log.WithField("module", m.NameField)
	m.Logger.Info("Default called: runtime and logger set.")

	spec, ok := moduleSpec.(*config.ControlPlaneEndpointSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.ControlPlaneEndpointSpec, got %T", m.NameField, moduleSpec)
	}
	m.ModuleSpec = spec
	m.Logger.Infof("Module spec configured: CP Endpoint Domain '%s', Address '%s', LB Type '%s'",
		m.ModuleSpec.Domain, m.ModuleSpec.Address, m.ModuleSpec.LoadBalancer.Type)
	return nil
}

func (m *HAProxyKeepalivedModule) AutoAssert() error {
	m.Logger.Info("AutoAssert called.")
	if m.KubeRuntime == nil || m.ModuleSpec == nil {
		return fmt.Errorf("runtime or moduleSpec not initialized for %s", m.NameField)
	}
	if len(m.KubeRuntime.RoleHosts()["loadbalancer"]) == 0 {
		return fmt.Errorf("loadbalancer hosts are required for module %s but not found in roleGroups", m.NameField)
	}
	if len(m.KubeRuntime.RoleHosts()["control-plane"]) == 0 {
		return fmt.Errorf("control-plane hosts are required for HAProxy backends but not found in roleGroups")
	}
	if m.ModuleSpec.Address == "" && m.ModuleSpec.Domain == "" {
		return fmt.Errorf("controlPlaneEndpoint.Address or Domain (for VIP) must be specified")
	}
	if m.ModuleSpec.Port == 0 {
		return fmt.Errorf("controlPlaneEndpoint.Port must be specified")
	}
	m.Logger.Info("AutoAssert completed successfully.")
	return nil
}

func (m *HAProxyKeepalivedModule) Init() error {
	m.Logger.Info("Init called (task assembly).")
	// Example: Add tasks to install HAProxy and Keepalived on each loadbalancer node.
	// lbHosts := m.KubeRuntime.RoleHosts()["loadbalancer"]
	// for _, host := range lbHosts {
	//    installHATask := loadbalancerTasks.NewInstallHAProxyTask(host) // Task needs to be host-specific
	//    m.tasks = append(m.tasks, installHATask)
	//    installKATask := loadbalancerTasks.NewInstallKeepalivedTask(host)
	//    m.tasks = append(m.tasks, installKATask)
	// }
	m.Logger.Info("No concrete tasks assembled in this skeleton Init for HAProxyKeepalivedModule.")
	return nil
}

func (m *HAProxyKeepalivedModule) Run(result *ending.ModuleResult) {
	m.Logger.Info("Run called.")
	// Loop through m.tasks
	if len(m.tasks) == 0 {
		m.Logger.Info("No tasks to execute.")
		if result.Status == ending.ModuleResultPending {
			result.SetStatus(ending.ModuleResultSuccess) // Or Skipped if IsSkip was true and pipeline still called Run
			result.SetMessage(fmt.Sprintf("%s: No tasks to execute.", m.NameField))
		}
		return
	}
	// ... task execution loop ...
	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped {
		result.SetStatus(ending.ModuleResultSuccess)
		result.SetMessage(fmt.Sprintf("%s conceptual run completed successfully.", m.NameField))
	}
	m.Logger.Info("Run completed.")
}

func (m *HAProxyKeepalivedModule) Until(runtime *krt.KubeRuntime) (done bool, err error) {
	m.Logger.Info("Until called (placeholder, returning true).")
	return true, nil
}

func (m *HAProxyKeepalivedModule) CallPostHook(res *ending.ModuleResult) error {
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

func (m *HAProxyKeepalivedModule) Is() module.Type {
	return module.TaskModuleType
}

func (m *HAProxyKeepalivedModule) Slogan() string {
	return fmt.Sprintf("Setting up HA Load Balancer (HAProxy+Keepalived) for Control Plane on %s:%d...",
		m.ModuleSpec.Address, m.ModuleSpec.Port)
}

func (m *HAProxyKeepalivedModule) AppendPostHook(hookFn module.HookFn) {
	m.postHooks = append(m.postHooks, hookFn)
}

var _ module.Module = (*HAProxyKeepalivedModule)(nil)
