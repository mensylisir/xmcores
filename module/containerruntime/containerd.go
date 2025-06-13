package containerruntime

import (
	"fmt"

	"github.com/mensylisir/xmcores/config" // Assuming KubernetesSpec contains ContainerManager
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// ContainerdModule installs and configures Containerd.
type ContainerdModule struct{}

// Name returns the name of the module.
func (m *ContainerdModule) Name() string {
	return "container-runtime-containerd"
}

// Description returns a human-readable description of what the module does.
func (m *ContainerdModule) Description() string {
	return "Installs and configures Containerd as the container runtime."
}

// Execute runs the module's logic for setting up Containerd.
func (m *ContainerdModule) Execute(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Infof("Executing module: %s (%s)", m.Name(), m.Description())

	// The moduleSpec could be more specific, e.g., a dedicated config.ContainerdSpec
	// if it exists, or it could be the general config.KubernetesSpec to check ContainerManager.
	// For this skeleton, let's assume it receives KubernetesSpec to check the manager type,
	// and would receive a more specific ContainerdSpec if we had one for version, etc.
	// Or, the pipeline could pass cfg.Spec.ContainerRuntime from ClusterConfig (if that field existed)
	// For now, using KubernetesSpec as a placeholder for where ContainerManager is defined.

	k8sSpec, ok := moduleSpec.(*config.KubernetesSpec)
	if !ok {
		// Alternative: if pipeline passes cfg.Spec.ContainerRuntime (from ClusterConfig)
		// crSpec, okCr := moduleSpec.(*config.ContainerRuntimeSpec) // Assuming such a struct exists
		// if !okCr { ... }
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.KubernetesSpec (or specific CR spec), got %T", m.Name(), moduleSpec)
	}

	if k8sSpec.ContainerManager != "containerd" {
		logger.Warnf("Container manager in spec is '%s', but this is ContainerdModule. Skipping execution.", k8sSpec.ContainerManager)
		// Or return an error: return fmt.Errorf("expected container manager 'containerd', got '%s'", k8sSpec.ContainerManager)
		return nil // Assuming pipeline calls the correct module based on ContainerManager field.
	}

	// Version might come from a dedicated ContainerdSpec if we add it to ClusterConfig.
	// For example: cfg.Spec.ContainerRuntime.Containerd.Version
	containerdVersion := "N/A (version not directly in k8sSpec, would need dedicated ContainerdSpec)"
	// If config.RegistrySpec is available (e.g. passed as another part of moduleSpec or via runtime context),
	// it would be used for configuring registry mirrors in containerd.

	logger.Infof("Preparing to install container manager: %s. Version: %s", k8sSpec.ContainerManager, containerdVersion)

	// Conceptual steps - these would typically run on all hosts or specific roles (control-plane, worker)
	allHosts := pipelineRuntime.AllHosts()
	if len(allHosts) == 0 {
		return fmt.Errorf("no hosts available in runtime to install containerd on")
	}
	logger.Infof("Targeting %d hosts for containerd installation/configuration.", len(allHosts))

	logger.Info("Conceptual step: Check/Install prerequisite packages (e.g., libseccomp) on all hosts...")
	for _, host := range allHosts {
		logger.Debugf("Conceptual step: Install containerd package version '%s' on host %s (%s)...", containerdVersion, host.GetName(), host.GetAddress())
	}
	logger.Info("Conceptual step: Configure containerd (e.g., /etc/containerd/config.toml) on all hosts...")
	logger.Info("  Conceptual sub-step: Configure registry mirrors and insecure registries...")
	logger.Info("  Conceptual sub-step: Configure systemd cgroup driver...")
	for _, host := range allHosts {
		logger.Debugf("Conceptual step: Enable and start containerd service on host %s (%s)...", host.GetName(), host.GetAddress())
	}
	logger.Info("Conceptual step: Verify containerd status on all hosts...")


	logger.Infof("Module %s executed successfully (conceptually).", m.Name())
	return nil
}

// Ensure ContainerdModule implements the module.Module interface.
var _ module.Module = (*ContainerdModule)(nil)
