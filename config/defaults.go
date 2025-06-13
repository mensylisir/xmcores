package config

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/connector"
	// "github.com/sirupsen/logrus" // For logging within defaults if needed
)

// Define default constants
const (
	DefaultKubeVersion        = "v1.28.0" // Example default, should be updated regularly
	DefaultClusterName        = "xm-cluster"
	DefaultMaxPods            = 110 // Kubelet default
	DefaultPodsCIDR           = "10.244.0.0/16"
	DefaultServiceCIDR        = "10.96.0.0/12"
	DefaultNetworkPlugin      = "calico"
	DefaultEtcdType           = "kubeadm" // Stacked etcd managed by kubeadm
	DefaultContainerManager   = "containerd"
	DefaultLBType             = "haproxy-keepalived"
	DefaultControlPlanePort   = 6443
)

// SetDefaultClusterSpec applies default values to the ClusterSpec and processes host roles.
// IMPORTANT: This function currently modifies cfgToProcess directly. A DeepCopy mechanism
// should be implemented for safer operations if the original parsed config needs to be preserved.
// It returns the (potentially modified) spec, a map of role names to host objects,
// a list of all processed host objects, and any error encountered.
func SetDefaultClusterSpec(cfgToProcess *ClusterSpec, allHostSpecsFromYAML []HostSpec) (
	defaultedSpec *ClusterSpec,
	roleMap map[string][]connector.Host,
	allKubeHosts []connector.Host,
	err error,
) {
	if cfgToProcess == nil {
		return nil, nil, nil, fmt.Errorf("input ClusterSpec to SetDefaultClusterSpec cannot be nil")
	}

	defaultedSpec = cfgToProcess // Direct modification for now

	// Initialize maps and slices
	allKubeHosts = make([]connector.Host, 0, len(allHostSpecsFromYAML))
	hostMapByName := make(map[string]connector.Host) // For quick lookup by name
	roleMap = make(map[string][]connector.Host)

	// Process Hosts from YAML HostSpec entries
	for _, hostSpec := range allHostSpecsFromYAML {
		host := connector.NewHost() // This creates *connector.BaseHost
		host.SetName(hostSpec.Name)
		host.SetAddress(hostSpec.Address)
		host.SetInternalAddress(hostSpec.InternalAddress)
		if hostSpec.Port == 0 {
			host.SetPort(22) // Default SSH port
		} else {
			host.SetPort(hostSpec.Port)
		}
		host.SetUser(hostSpec.User)
		host.SetPassword(hostSpec.Password)
		host.SetPrivateKeyPath(hostSpec.PrivateKeyPath)
		// host.SetArch(string(hostSpec.HostArch)) // Assuming HostSpec.HostArch is common.Arch or string

		if err := host.Validate(); err != nil {
			// Log or collect errors? For now, let's return on first host validation error.
			return nil, nil, nil, fmt.Errorf("host '%s' validation failed: %w", hostSpec.Name, err)
		}
		allKubeHosts = append(allKubeHosts, host)
		if host.GetName() != "" {
			hostMapByName[host.GetName()] = host
		}
	}

	// Apply defaults to KubernetesSpec
	if defaultedSpec.Kubernetes == nil {
		defaultedSpec.Kubernetes = &KubernetesSpec{} // Initialize if nil
	}
	if defaultedSpec.Kubernetes.Version == "" {
		defaultedSpec.Kubernetes.Version = DefaultKubeVersion
	}
	if defaultedSpec.Kubernetes.ClusterName == "" {
		// defaultedSpec.Kubernetes.ClusterName = DefaultClusterName // metadata.name is primary
	}
	if defaultedSpec.Kubernetes.ContainerManager == "" {
		defaultedSpec.Kubernetes.ContainerManager = DefaultContainerManager
	}
	// Assuming Type is mandatory and should be set in YAML, or handled by higher-level validation.
	// if defaultedSpec.Kubernetes.Type == "" { ... }


	// Apply defaults to NetworkSpec
	if defaultedSpec.Network == nil {
		defaultedSpec.Network = &NetworkSpec{} // Initialize if nil
	}
	if defaultedSpec.Network.Plugin == "" {
		defaultedSpec.Network.Plugin = DefaultNetworkPlugin
	}
	if defaultedSpec.Network.KubePodsCIDR == "" {
		defaultedSpec.Network.KubePodsCIDR = DefaultPodsCIDR
	}
	if defaultedSpec.Network.KubeServiceCIDR == "" {
		defaultedSpec.Network.KubeServiceCIDR = DefaultServiceCIDR
	}
	// BlockSize is a pointer, so nil means not set; pipeline can interpret default.

	// Apply defaults to EtcdSpec
	if defaultedSpec.Etcd == nil {
		defaultedSpec.Etcd = &EtcdSpec{} // Initialize if nil
	}
	if defaultedSpec.Etcd.Type == "" {
		defaultedSpec.Etcd.Type = DefaultEtcdType
	}

	// Process RoleGroups and assign roles to connector.Host objects
	if defaultedSpec.RoleGroups != nil {
		for roleName, hostNameList := range defaultedSpec.RoleGroups {
			roleName = strings.ToLower(strings.TrimSpace(roleName)) // Normalize role name
			var hostsInCurrentRole []connector.Host
			for _, hostName := range hostNameList {
				hostObj, found := hostMapByName[hostName]
				if found {
					hostObj.AddRole(roleName) // Add the specific role
					// Add general "k8s" role if it's a master or worker type role
					if roleName == connector.RoleMaster || roleName == connector.RoleControlPlane || roleName == connector.RoleWorker {
						hostObj.AddRole(connector.RoleK8s)
					}
					hostsInCurrentRole = append(hostsInCurrentRole, hostObj)
				} else {
					// Log warning: host name in role group not found in allHostSpecsFromYAML
					// This could be an error depending on strictness.
					fmt.Printf("Warning: Host '%s' assigned to role '%s' not defined in spec.hosts.\n", hostName, roleName)
				}
			}
			if len(hostsInCurrentRole) > 0 {
				roleMap[roleName] = hostsInCurrentRole
			}
		}
	}

	// Apply defaults to ControlPlaneEndpoint
	if defaultedSpec.ControlPlaneEndpoint == nil {
		defaultedSpec.ControlPlaneEndpoint = &ControlPlaneEndpointSpec{}
	}
	if defaultedSpec.ControlPlaneEndpoint.Port == 0 {
		defaultedSpec.ControlPlaneEndpoint.Port = DefaultControlPlanePort
	}
	// Default ControlPlaneEndpoint.Address if load balancing is disabled and address is empty
	if !defaultedSpec.ControlPlaneEndpoint.LoadBalancer.Enable && defaultedSpec.ControlPlaneEndpoint.Address == "" {
		cpHosts := roleMap[connector.RoleControlPlane]
		if len(cpHosts) == 0 {
			cpHosts = roleMap[connector.RoleMaster] // Fallback to "master" role
		}
		if len(cpHosts) > 0 {
			// Use the first control plane/master node's address
			defaultedSpec.ControlPlaneEndpoint.Address = cpHosts[0].GetAddress()
			// Potentially log this defaulting action
			fmt.Printf("Defaulting ControlPlaneEndpoint.Address to first control-plane/master host: %s\n", cpHosts[0].GetAddress())
		} else {
			// This would be a critical error, should be caught by validation later if address remains empty.
			fmt.Println("Warning: ControlPlaneEndpoint.Address is empty and no control-plane/master hosts found to default from.")
		}
	}
	// Default LB type if LB enabled but type is empty
	if defaultedSpec.ControlPlaneEndpoint.LoadBalancer.Enable && defaultedSpec.ControlPlaneEndpoint.LoadBalancer.Type == "" {
		defaultedSpec.ControlPlaneEndpoint.LoadBalancer.Type = DefaultLBType
	}


	// Default Registry settings (if any common defaults are needed)
	if defaultedSpec.Registry == nil {
		defaultedSpec.Registry = &RegistrySpec{}
	}
	// Example: if defaultedSpec.Registry.Type == "" { defaultedSpec.Registry.Type = "docker" }

	return defaultedSpec, roleMap, allKubeHosts, nil
}
