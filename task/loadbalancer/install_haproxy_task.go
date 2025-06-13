package loadbalancer

import (
	"fmt"

	"github.com/mensylisir/xmcores/config" // For potential taskSpec type assertion
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// InstallHAProxyTask is responsible for installing HAProxy on load balancer nodes.
type InstallHAProxyTask struct {
	task.BaseTask
	// TaskSpec is inherited from BaseTask.
	// It might be *config.LoadBalancerConfigSpec or part of ControlPlaneEndpointSpec.
}

// NewInstallHAProxyTask creates a new InstallHAProxyTask.
func NewInstallHAProxyTask() task.Task {
	t := &InstallHAProxyTask{}
	t.NameField = "lb-install-haproxy"
	t.DescriptionField = "Installs HAProxy service on load balancer nodes."
	t.BaseTask = task.NewBaseTask(t.NameField, t.DescriptionField)
	return t
}

// Default stores runtime, logger, and taskSpec.
func (t *InstallHAProxyTask) Default(runtime *krt.KubeRuntime, taskSpec interface{}, moduleCache interface{}, taskCache interface{}) error {
	if err := t.BaseTask.Default(runtime, taskSpec, moduleCache, taskCache); err != nil {
		return err
	}
	t.Logger = runtime.Log.WithFields(logrus.Fields{"task": t.Name(), "type": "InstallHAProxyTask"})

	// Example: Type assert taskSpec if specific LB parameters are needed for installation.
	// For this task, the module might pass &ControlPlaneEndpointSpec.LoadBalancer
	if spec, ok := t.TaskSpec.(*config.LoadBalancerConfigSpec); ok {
		t.Logger.Infof("InstallHAProxyTask Default: Received LoadBalancerConfigSpec (Enable: %v, Type: %s)", spec.Enable, spec.Type)
	} else if t.TaskSpec != nil {
		t.Logger.Warnf("InstallHAProxyTask Default: taskSpec is not *config.LoadBalancerConfigSpec (type: %T), proceeding with generic install logic if possible.", t.TaskSpec)
	} else {
		t.Logger.Info("InstallHAProxyTask Default: taskSpec is nil, proceeding with generic install logic.")
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
		t.Logger.Warn("No hosts in 'loadbalancer' role, InstallHAProxyTask will have no steps.")
		return nil
	}

	t.Logger.Infof("Initializing HAProxy installation steps for %d load balancer node(s).", len(lbHosts))

	for _, host := range lbHosts {
		// Conceptual: Determine package manager and install command based on host OS
		installCmd := "apt-get update && apt-get install -y haproxy" // Generic example

		installStep := runcmd.NewRunCommandStep(
			fmt.Sprintf("InstallHAProxy-%s", host.GetName()),
			fmt.Sprintf("Install HAProxy on node %s", host.GetName()),
			installCmd,
		)

		// Limitation: RunCommandStep needs to be made host-aware or run via a host-specific runtime.
		// For now, this step will run on the default host of t.Runtime.
		t.Logger.Warnf("RunCommandStep for host %s will run on default host of current runtime due to step limitations.", host.GetName())

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
	return fmt.Sprintf("Installing HAProxy on load balancer nodes for task: %s...", t.Name())
}

// Run, IsSkip, AutoAssert, Until, Steps are inherited from BaseTask.
var _ task.Task = (*InstallHAProxyTask)(nil)
