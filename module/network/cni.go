package network

import (
	"fmt"

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// CalicoCNIModule installs and configures Calico CNI.
// Note: The filename is cni.go, but the struct is CalicoCNIModule.
// This is fine, but sometimes naming them consistently (e.g. calico.go for CalicoCNIModule) can be clearer.
type CalicoCNIModule struct{}

// Name returns the name of the module.
func (m *CalicoCNIModule) Name() string {
	return "cni-calico"
}

// Description returns a human-readable description of what the module does.
func (m *CalicoCNIModule) Description() string {
	return "Installs and configures Calico CNI."
}

// Execute runs the module's logic for setting up Calico CNI.
func (m *CalicoCNIModule) Execute(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Infof("Executing module: %s (%s)", m.Name(), m.Description())

	// Type assert moduleSpec to *config.NetworkSpec
	networkSpec, ok := moduleSpec.(*config.NetworkSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.NetworkSpec, got %T", m.Name(), moduleSpec)
	}

	if networkSpec.Plugin != "calico" {
		logger.Warnf("CNI plugin in spec is '%s', but this is CalicoCNIModule. Skipping execution.", networkSpec.Plugin)
		// Or return an error: return fmt.Errorf("expected CNI plugin 'calico', got '%s'", networkSpec.Plugin)
		return nil // Assuming for now that pipeline logic would call the correct CNI module.
	}

	blockSizeStr := "N/A (not set in config)"
	if networkSpec.BlockSize != nil {
		blockSizeStr = fmt.Sprintf("%d", *networkSpec.BlockSize)
	}

	logger.Infof("Key parameters: CNI Plugin: %s, Pod CIDR: %s, Service CIDR: %s, BlockSize: %s",
		networkSpec.Plugin, networkSpec.KubePodsCIDR, networkSpec.KubeServiceCIDR, blockSizeStr)

	// Conceptual steps
	logger.Info("Conceptual step: Determine Calico version based on Kubernetes version or configuration...")
	logger.Info("Conceptual step: Download Calico manifest (e.g., calico.yaml or operator manifests)...")
	logger.Info("Conceptual step: Customize Calico manifest with PodCIDR if necessary...")
	logger.Infof("Conceptual step: Customize Calico manifest with BlockSize (%s) if applicable...", blockSizeStr)
	logger.Info("Conceptual step: Customize Calico manifest with Calico mode (e.g., from cni.calico.mode in full config)...")
	logger.Info("Conceptual step: Apply Calico manifests to the cluster (typically on a control plane node)...")
	logger.Info("Conceptual step: Wait for Calico pods to be ready...")


	logger.Infof("Module %s executed successfully (conceptually).", m.Name())
	return nil
}

// Ensure CalicoCNIModule implements the module.Module interface.
var _ module.Module = (*CalicoCNIModule)(nil)
