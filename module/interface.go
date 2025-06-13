package module

import (
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// Module represents a logical unit of work within a Pipeline.
// It is responsible for a specific domain of operations (e.g., setting up Kubernetes control plane,
// installing CNI, configuring a load balancer). Modules are orchestrated by a Pipeline.
type Module interface {
	// Name returns the unique name of the module (e.g., "kubernetes-controlplane", "cni-calico").
	Name() string

	// Description provides a human-readable summary of what the module does.
	Description() string

	// Init prepares the module for execution.
	// - pipelineRuntime: The runtime environment scoped for the current pipeline execution.
	//                    It provides access to hosts, roles, and execution capabilities.
	// - moduleSpec: The configuration specific to this module's execution. This is typically
	//               a part of the overall ClusterConfig (e.g., config.KubernetesSpec, config.NetworkSpec),
	//               passed down by the pipeline. The module will type-assert this to its expected struct.
	// - logger: A logger entry pre-configured with pipeline and module context.
	// The module should store necessary state from moduleSpec and pipelineRuntime internally if needed for Execute.
	Init(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error

	// Execute runs the main logic of the module.
	// It should use the operational runtime and configurations prepared during Init.
	// - logger: A logger entry pre-configured for this module's execution phase.
	Execute(logger *logrus.Entry) error
}
