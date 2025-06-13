package loadbalancer

import (
	"fmt"

	// "github.com/mensylisir/xmcores/config" // Might not be needed if spec is generic
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step/runcmd"
	"github.com/mensylisir/xmcores/task"
	"github.com/sirupsen/logrus"
)

// InstallHAProxyTask is responsible for installing HAProxy on load balancer nodes.
type InstallHAProxyTask struct {
	task.BaseTask
	// taskSpec can be used to pass specific versions or configurations if needed.
	// For now, assuming it's generic or nil.
}

// NewInstallHAProxyTask creates a new InstallHAProxyTask.
func NewInstallHAProxyTask() task.Task {
	return &InstallHAProxyTask{
		BaseTask: task.NewBaseTask("lb-install-haproxy", "Installs HAProxy service on load balancer nodes."),
	}
}

// Init initializes the InstallHAProxyTask.
// taskSpec could be *config.ControlPlaneEndpointSpec.LoadBalancer or a dedicated LB config struct.
func (t *InstallHAProxyTask) Init(moduleRuntime runtime.Runtime, taskSpec interface{}, logger *logrus.Entry) error {
	if err := t.BaseTask.Init(moduleRuntime, taskSpec, logger); err != nil {
		return err
	}

	// Example: Type assert taskSpec if specific LB parameters are needed for installation.
	// lbSpec, ok := taskSpec.(*config.LoadBalancerConfigSpec)
	// if !ok {
	//    t.BaseTask.logger.Warnf("taskSpec for %s is not *config.LoadBalancerConfigSpec (type: %T), proceeding with generic install", t.Name(), taskSpec)
	// }

	lbHosts := t.BaseTask.runtime.RoleHosts()["loadbalancer"]
	if len(lbHosts) == 0 {
		// This task should only be called by a module if LB hosts are defined and internal LB is enabled.
		// However, good to have a safeguard.
		t.BaseTask.logger.Warn("No hosts in 'loadbalancer' role, InstallHAProxyTask will have no steps.")
		return nil // Or return an error if this task mandates LB hosts.
	}

	t.BaseTask.logger.Infof("Initializing InstallHAProxyTask for %d load balancer node(s).", len(lbHosts))

	for _, host := range lbHosts {
		// Conceptual: Determine package manager and install command based on host OS (if known via runtime/host properties)
		// For now, using a generic apt-get example.
		installCmd := "apt-get update && apt-get install -y haproxy"

		installStep := runcmd.NewRunCommandStep(
			fmt.Sprintf("InstallHAProxy-%s", host.GetName()),
			fmt.Sprintf("Install HAProxy on node %s", host.GetName()),
			installCmd,
		)

		// Important: Steps that run commands on specific hosts need that host context.
		// RunCommandStep currently runs on the *first* host in the runtime's AllHosts.
		// This needs refinement: either RunCommandStep takes a host target,
		// or we need a way to get a runtime scoped to a single host for the step.
		// For this skeleton, we'll log this limitation.
		t.BaseTask.logger.Warnf("RunCommandStep used by InstallHAProxyTask currently doesn't support per-host targeting easily. Step for host %s will run on default host of runtime.", host.GetName())

		stepLogger := t.BaseTask.logger.WithField("step_name", installStep.Name()).WithField("target_host", host.GetName())
		if err := installStep.Init(t.BaseTask.runtime, stepLogger); err != nil { // Pass runtime from BaseTask
			return fmt.Errorf("failed to init step '%s' for host '%s': %w", installStep.Name(), host.GetName(), err)
		}
		t.AddStep(installStep)
		t.BaseTask.logger.Debugf("Added step: %s for host %s", installStep.Name(), host.GetName())
	}

	t.BaseTask.logger.Info("InstallHAProxyTask initialized.")
	return nil
}

// Execute method is inherited from BaseTask.
// var _ task.Task = (*InstallHAProxyTask)(nil) // Ensured by BaseTask embedding.
