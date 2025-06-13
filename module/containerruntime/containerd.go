package containerruntime

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

type ContainerdModule struct{}

func (m *ContainerdModule) Name() string { return "container-runtime-containerd" }
func (m *ContainerdModule) Description() string { return "Installs and configures Containerd." }

func (m *ContainerdModule) Init(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Info("ContainerdModule Init (placeholder)")
	// Example: Pipeline passes &p.clusterSpec.Kubernetes as moduleSpec
	k8sSpec, ok := moduleSpec.(*config.KubernetesSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.KubernetesSpec, got %T", m.Name(), moduleSpec)
	}
	if k8sSpec.ContainerManager != "containerd" {
		return fmt.Errorf("%s module called but ContainerManager is '%s'", m.Name(), k8sSpec.ContainerManager)
	}
	// Store necessary config from k8sSpec or pipelineRuntime if needed for Execute phase
	return nil
}

func (m *ContainerdModule) Execute(logger *logrus.Entry) error {
	logger.Info("ContainerdModule Execute (placeholder)")
	// Access stored config/runtime here
	logger.Info("Conceptual step: Install containerd packages on all relevant hosts...")
	logger.Info("Conceptual step: Configure containerd (e.g., registry mirrors)...")
	logger.Info("Conceptual step: Start containerd service...")
	return nil
}
