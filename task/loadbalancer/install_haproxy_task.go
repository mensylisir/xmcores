package loadbalancer

import (
	"fmt"

	"github.com/mensylisir/xmcores/config"
	// "github.com/mensylisir/xmcores/pipeline/ending" // Not directly used by this task's methods
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// InstallHAProxyTask is responsible for installing HAProxy on load balancer nodes.
type InstallHAProxyTask struct {
	*task.BaseTask
	// TaskSpec is inherited. Assumed to be *config.LoadBalancerConfigSpec or similar.
}

// NewInstallHAProxyTask creates a new InstallHAProxyTask.
func NewInstallHAProxyTask() task.Task {
	base := task.NewBaseTask("lb-install-haproxy", "Installs HAProxy service on load balancer nodes.")
	return &InstallHAProxyTask{
		BaseTask: base,
	}
}

// Default stores runtime, logger, and the taskSpec.
func (t *InstallHAProxyTask) Default(runtime *krt.ClusterRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	if err := t.BaseTask.Default(runtime, taskSpec, moduleCache, taskCache); err != nil {
		return err
	}
	t.Logger = runtime.Log.WithFields(logrus.Fields{"task": t.NameField, "type": "InstallHAProxyTask"})

	// The pipeline passes &p.clusterSpec.ControlPlaneEndpoint to the HAProxyKeepalivedModule,
	// which in turn would pass parts of it or a dedicated LB spec to this task.
	// For now, let's assume taskSpec might be *config.LoadBalancerConfigSpec.
	if spec, ok := t.TaskSpec.(*config.LoadBalancerConfigSpec); ok {
		t.Logger.Infof("InstallHAProxyTask Default: Received LoadBalancerConfigSpec (Enable: %v, Type: %s)", spec.Enable, spec.Type)
	} else if t.TaskSpec != nil {
		t.Logger.Warnf("InstallHAProxyTask Default: taskSpec is not *config.LoadBalancerConfigSpec (type: %T). Generic install logic will apply if possible.", t.TaskSpec)
	} else {
		t.Logger.Info("InstallHAProxyTask Default: taskSpec is nil. Generic install logic will apply.")
	}
	t.Logger.Info("InstallHAProxyTask Default completed.")
	return nil
}

// Init assembles steps for installing HAProxy.
func (t *InstallHAProxyTask) Init() error {
	if err := t.BaseTask.Init(); err != nil {
		return err
	}
	t.Logger.Info("InstallHAProxyTask Init called - assembling steps.")

	lbHosts := t.Runtime.RoleHosts()["loadbalancer"]
	if len(lbHosts) == 0 {
		t.Logger.Warn("No hosts in 'loadbalancer' role, InstallHAProxyTask will have no steps. This task should typically be skipped by its parent module if this is the case.")
		return nil
	}

	t.Logger.Infof("Initializing HAProxy installation steps for %d load balancer node(s).", len(lbHosts))

	for _, host := range lbHosts {
		installCmd := "apt-get update && apt-get install -y haproxy" // Generic example

		installStep := runcmd.NewRunCommandStep(
			fmt.Sprintf("InstallHAProxy-%s", host.GetName()),
			fmt.Sprintf("Install HAProxy on node %s", host.GetName()),
			installCmd,
		)

		t.Logger.Warnf("RunCommandStep for host %s will run on default host of current runtime due to step limitations. Host-specific targeting for steps needs further implementation.", host.GetName())

		stepLogger := t.Logger.WithField("step", installStep.Name()).WithField("target_host_conceptual", host.GetName())
		if err := installStep.Init(t.Runtime, stepLogger); err != nil {
			return fmt.Errorf("failed to init step '%s' for host '%s': %w", installStep.Name(), host.GetName(), err)
		}
		t.AddStep(installStep)
		t.Logger.Debugf("Added step: %s for conceptual target host %s", installStep.Name(), host.GetName())
	}

	t.Logger.Info("InstallHAProxyTask initialized.")
	return nil
}

// Slogan provides a specific slogan for InstallHAProxyTask.
func (t *InstallHAProxyTask) Slogan() string {
	return fmt.Sprintf("Installing HAProxy for task: %s...", t.NameField)
}

var _ task.Task = (*InstallHAProxyTask)(nil)
