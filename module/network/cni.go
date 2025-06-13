package network

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

type CalicoCNIModule struct{} // Assuming pipeline specifically calls this for "calico"

func (m *CalicoCNIModule) Name() string { return "cni-calico" }
func (m *CalicoCNIModule) Description() string { return "Installs and configures Calico CNI." }

func (m *CalicoCNIModule) Init(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Info("CalicoCNIModule Init (placeholder)")

	spec, ok := moduleSpec.(*config.NetworkSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.NetworkSpec, got %T", m.Name(), moduleSpec)
	}
	if spec.Plugin != "calico" {
		return fmt.Errorf("%s module initialized, but network plugin specified is '%s', not 'calico'", m.Name(), spec.Plugin)
	}
	// Store spec, runtime if needed for Execute
	return nil
}

func (m *CalicoCNIModule) Execute(logger *logrus.Entry) error {
	logger.Info("CalicoCNIModule Execute (placeholder)")
	logger.Info("Conceptual step: Download Calico manifests...")
	logger.Info("Conceptual step: Customize manifests (PodCIDR, BlockSize)...")
	logger.Info("Conceptual step: Apply manifests to cluster...")
	return nil
}
