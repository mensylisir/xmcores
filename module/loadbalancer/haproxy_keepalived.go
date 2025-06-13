package loadbalancer

import (
	"fmt"

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// HAProxyKeepalivedModule sets up HAProxy and Keepalived for control plane load balancing.
type HAProxyKeepalivedModule struct{}

// Name returns the name of the module.
func (m *HAProxyKeepalivedModule) Name() string {
	return "loadbalancer-haproxy-keepalived"
}

// Description returns a human-readable description of what the module does.
func (m *HAProxyKeepalivedModule) Description() string {
	return "Sets up HAProxy and Keepalived for control plane load balancing."
}

// Execute runs the module's logic for setting up HAProxy and Keepalived.
func (m *HAProxyKeepalivedModule) Execute(pipelineRuntime runtime.Runtime, moduleSpec interface{}, logger *logrus.Entry) error {
	logger.Infof("Executing module: %s (%s)", m.Name(), m.Description())

	// The moduleSpec here is expected to be the ControlPlaneEndpointSpec,
	// as this module is primarily concerned with setting up the VIP and LB for that endpoint.
	cpEndpointSpec, ok := moduleSpec.(*config.ControlPlaneEndpointSpec)
	if !ok {
		return fmt.Errorf("invalid moduleSpec type for %s: expected *config.ControlPlaneEndpointSpec, got %T", m.Name(), moduleSpec)
	}

	vip := cpEndpointSpec.Address // This is the Virtual IP address the LB should manage
	domain := cpEndpointSpec.Domain
	lbPort := cpEndpointSpec.Port // The port HAProxy should listen on for K8s API

	logger.Infof("Key parameters: VIP: %s, Domain: %s, LB Port: %d", vip, domain, lbPort)

	lbHosts, roleFound := pipelineRuntime.RoleHosts()["loadbalancer"]
	if !roleFound || len(lbHosts) == 0 {
		logger.Error("No hosts found in 'loadbalancer' role. Cannot proceed with HAProxy/Keepalived setup.")
		// Depending on pipeline logic, this might not be a fatal error if an external LB is assumed.
		// However, if this module is explicitly called, it implies an internal LB is desired.
		return fmt.Errorf("required 'loadbalancer' role is empty or not defined for module %s", m.Name())
	}
	logger.Infof("Found %d host(s) for 'loadbalancer' role: %v", len(lbHosts), lbHosts)
	for _, host := range lbHosts {
		logger.Debugf("Load balancer host: %s (%s)", host.GetName(), host.GetAddress())
	}

	controlPlaneHosts, cpRoleFound := pipelineRuntime.RoleHosts()["control-plane"]
    if !cpRoleFound || len(controlPlaneHosts) == 0 {
        logger.Error("No hosts found in 'control-plane' role. Cannot configure HAProxy backends.")
        return fmt.Errorf("required 'control-plane' role is empty or not defined for HAProxy backend configuration")
    }
    logger.Infof("Found %d control plane host(s) to use as HAProxy backends.", len(controlPlaneHosts))


	// Conceptual steps for each host in the 'loadbalancer' role
	for _, lbHost := range lbHosts {
		logger.Infof("Conceptual steps for Load Balancer node: %s (%s)", lbHost.GetName(), lbHost.GetAddress())
		logger.Info("  Conceptual step: Install HAProxy package...")
		logger.Info("  Conceptual step: Install Keepalived package...")
		logger.Infof("  Conceptual step: Configure Keepalived (VRRP for VIP %s, health checks)...", vip)
		// Keepalived config would involve setting up VRRP instance, virtual_ipaddress, track_script, etc.
		// Other LB nodes would be peers.
		logger.Infof("  Conceptual step: Configure HAProxy (frontend listen on %s:%d, backend K8s API servers)...", vip, lbPort)
		// HAProxy config would list all controlPlaneHosts as backend servers.
		for _, cpHost := range controlPlaneHosts {
			logger.Debugf("    Conceptual HAProxy backend: %s (%s:%d)", cpHost.GetName(), cpHost.GetInternalAddress(), lbPort) // Assuming internal address for backend
		}
		logger.Info("  Conceptual step: Start Keepalived service...")
		logger.Info("  Conceptual step: Start HAProxy service...")
	}

	logger.Info("Conceptual step: Verify VIP is active and HAProxy is load balancing...")


	logger.Infof("Module %s executed successfully (conceptually).", m.Name())
	return nil
}

// Ensure HAProxyKeepalivedModule implements the module.Module interface.
var _ module.Module = (*HAProxyKeepalivedModule)(nil)
