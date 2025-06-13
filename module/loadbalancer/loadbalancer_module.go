package loadbalancer

import (
	"fmt"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

type HAProxyKeepalivedModule struct{}

func (m *HAProxyKeepalivedModule) Name() string { return "loadbalancer-haproxy-keepalived" }
func (m *HAProxyKeepalivedModule) Description() string { return "Manages HAProxy and Keepalived for CP load balancing." }

func (m *HAProxyKeepalivedModule) Init(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Info("HAProxyKeepalivedModule Init (placeholder)")

	// moduleSpec for this module was &p.clusterSpec.ControlPlaneEndpoint (which is *config.ControlPlaneEndpointSpec)
	// or &p.clusterSpec.ControlPlaneEndpoint.LoadBalancer (*config.LoadBalancerConfigSpec)
	// The pipeline currently passes &p.clusterSpec.ControlPlaneEndpoint
	_, ok := moduleSpec.(*config.ControlPlaneEndpointSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.ControlPlaneEndpointSpec, got %T", m.Name(), moduleSpec)
	}
	return nil
}
func (m *HAProxyKeepalivedModule) Execute(logger *logrus.Entry) error {
	logger.Info("HAProxyKeepalivedModule Execute (placeholder)")
	return nil
}
