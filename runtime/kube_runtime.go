package runtime

import (
	"fmt"
	"sync"
	"time" // Required for connector.Config for Timeout

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/connector"
	"github.com/sirupsen/logrus"
)

// KubeRuntime holds all runtime information specific to a Kubernetes cluster operation,
// including the cluster configuration, parsed CLI arguments, host details, and cached connectors.
type KubeRuntime struct {
	Cluster     *config.ClusterSpec // The spec of the cluster being worked on
	Arg         *CliArgs            // Parsed CLI arguments that might affect behavior
	Log         *logrus.Entry       // Logger scoped for this runtime instance
	WorkDir     string              // Global working directory
	IgnoreError bool                // Global flag to ignore errors
	Verbose     bool                // Global verbosity flag

	AllHosts  []connector.Host            // All validated hosts for this cluster
	RoleHosts map[string][]connector.Host // Map of roles to hosts

	connectorCache     map[string]connector.Connector // Cache for active host connectors: key is host.ID()
	connectorCacheLock sync.Mutex
}

// NewKubeRuntime creates and initializes a new KubeRuntime instance.
// It processes the ClusterConfig to set up hosts and roles.
func NewKubeRuntime(
	clusterCfg *config.ClusterConfig,
	args *CliArgs,
	workDir string,
	ignoreError bool,
	verbose bool,
	baseLogger *logrus.Entry,
) (*KubeRuntime, error) {

	if clusterCfg == nil {
		return nil, fmt.Errorf("ClusterConfig cannot be nil when creating KubeRuntime")
	}
	if clusterCfg.Spec == nil {
		return nil, fmt.Errorf("ClusterConfig.Spec cannot be nil")
	}
	if args == nil {
		// Use default CliArgs if none are provided
		args = NewCliArgs()
	}
	if baseLogger == nil {
		// Fallback to a default logger if none provided, though main should always provide one.
		baseLogger = logrus.NewEntry(logrus.New()) // Should not happen in normal flow
	}

	scopedLogger := baseLogger.WithFields(logrus.Fields{
		"runtime_scope": "kube",
		"cluster_name":  clusterCfg.Metadata.Name,
	})
	scopedLogger.Info("Initializing KubeRuntime...")

	kr := &KubeRuntime{
		Cluster:            clusterCfg.Spec,
		Arg:                args,
		Log:                scopedLogger,
		WorkDir:            workDir,
		IgnoreError:        ignoreError,
		Verbose:            verbose,
		AllHosts:           make([]connector.Host, 0, len(clusterCfg.Spec.Hosts)),
		RoleHosts:          make(map[string][]connector.Host),
		connectorCache:     make(map[string]connector.Connector),
		connectorCacheLock: sync.Mutex{},
	}

	// Initialize Hosts
	hostMapByName := make(map[string]connector.Host)
	scopedLogger.Infof("Processing %d host definitions from ClusterConfig...", len(clusterCfg.Spec.Hosts))
	for i, hostSpec := range clusterCfg.Spec.Hosts {
		host := connector.NewHost() // Assumes NewHost() returns a concrete type that fits connector.Host
		host.SetName(hostSpec.Name)
		host.SetAddress(hostSpec.Address)
		host.SetInternalAddress(hostSpec.InternalAddress)
		port := hostSpec.Port
		if port == 0 {
			port = 22 // Default SSH port
		}
		host.SetPort(port)
		host.SetUser(hostSpec.User)
		host.SetPassword(hostSpec.Password)
		host.SetPrivateKeyPath(hostSpec.PrivateKeyPath)
		// host.SetArch(hostSpec.Arch) // If Arch field is added to HostSpec and Host interface

		if err := host.Validate(); err != nil {
			errMsg := fmt.Sprintf("Host %d ('%s', Address: '%s') validation failed: %v. This host will be skipped.", i+1, hostSpec.Name, hostSpec.Address, err)
			scopedLogger.Error(errMsg)
			// Depending on pipeline requirements, might return error or simply skip.
			// For now, skipping. Critical roles missing hosts will be caught later.
			continue
		}
		kr.AllHosts = append(kr.AllHosts, host)
		if host.GetName() != "" {
			hostMapByName[host.GetName()] = host
		} else {
			scopedLogger.Warnf("Host at index %d (Address: %s) has no name; it cannot be assigned to RoleGroups by name.", i, host.GetAddress())
		}
		scopedLogger.Debugf("Loaded and validated host: %s (%s)", host.GetName(), host.GetAddress())
	}
	scopedLogger.Infof("Successfully processed %d valid hosts.", len(kr.AllHosts))

	// Initialize RoleHosts
	if clusterCfg.Spec.RoleGroups != nil {
		scopedLogger.Info("Processing roleGroups...")
		for role, hostNames := range clusterCfg.Spec.RoleGroups {
			var hostsInRole []connector.Host
			for _, hostName := range hostNames {
				host, found := hostMapByName[hostName]
				if !found {
					scopedLogger.Warnf("Host '%s' defined in role '%s' not found among validated hosts. Skipping for this role.", hostName, role)
					continue
				}
				hostsInRole = append(hostsInRole, host)
			}
			if len(hostsInRole) > 0 {
				kr.RoleHosts[role] = hostsInRole
				scopedLogger.Debugf("Role '%s' assigned to hosts: %v", role, hostNames)
			} else {
				scopedLogger.Warnf("No valid hosts found or assigned for role '%s'. This role will be empty.", role)
			}
		}
	}
	definedRoles := []string{}
	for roleName, hostsInRoleList := range kr.RoleHosts {
		definedRoles = append(definedRoles, fmt.Sprintf("%s (%d hosts)", roleName, len(hostsInRoleList)))
	}
	if len(definedRoles) > 0 {
		scopedLogger.Infof("Processed roles: %s.", strings.Join(definedRoles, ", "))
	} else {
		scopedLogger.Info("No roles were effectively defined or populated with valid hosts.")
	}

	scopedLogger.Info("KubeRuntime initialized successfully.")
	return kr, nil
}

// GetConnector retrieves a cached connector for the given host, or creates a new one.
func (kr *KubeRuntime) GetConnector(host connector.Host) (connector.Connector, error) {
	if host == nil {
		return nil, fmt.Errorf("host cannot be nil")
	}
	hostID := host.ID() // Assuming Host interface has ID() for a unique key

	kr.connectorCacheLock.Lock()
	defer kr.connectorCacheLock.Unlock()

	if conn, found := kr.connectorCache[hostID]; found && conn != nil {
		// TODO: Add a a health check for cached connectors before returning
		// For now, assume cached connector is good.
		kr.Log.Debugf("Using cached connector for host: %s (%s)", host.GetName(), host.GetAddress())
		return conn, nil
	}

	kr.Log.Infof("Creating new connector for host: %s (%s)", host.GetName(), host.GetAddress())
	// Build connector.Config from connector.Host fields
	connCfg := connector.Config{
		Username:         host.GetUser(),
		Password:         host.GetPassword(),
		Address:          host.GetAddress(),
		Port:             host.GetPort(),
		PrivateKey:       host.GetPrivateKey(), // Assuming Host interface provides this
		KeyFile:          host.GetPrivateKeyPath(),
		AgentSocket:      "", // Example: get from host.GetAgentSocket() if available
		Timeout:          host.GetTimeout(),  // Example: get from host.GetTimeout()
		// Bastion details would also come from host or a global bastion config
	}

	newConn, err := connector.NewConnection(connCfg) // Uses the constructor from connector/ssh.go
	if err != nil {
		kr.Log.Errorf("Failed to create new connector for host %s (%s): %v", host.GetName(), host.GetAddress(), err)
		return nil, fmt.Errorf("failed to create connector for host %s: %w", host.GetName(), err)
	}

	kr.connectorCache[hostID] = newConn
	kr.Log.Infof("Successfully created and cached new connector for host: %s (%s)", host.GetName(), host.GetAddress())
	return newConn, nil
}

// CloseConnector closes and removes a specific connector from the cache.
func (kr *KubeRuntime) CloseConnector(host connector.Host) error {
	if host == nil {
		return fmt.Errorf("host cannot be nil when closing connector")
	}
	hostID := host.ID()

	kr.connectorCacheLock.Lock()
	defer kr.connectorCacheLock.Unlock()

	conn, found := kr.connectorCache[hostID]
	if !found || conn == nil {
		kr.Log.Debugf("No active connector found in cache for host %s (%s) to close.", host.GetName(), host.GetAddress())
		return nil
	}

	kr.Log.Infof("Closing connector for host: %s (%s)", host.GetName(), host.GetAddress())
	delete(kr.connectorCache, hostID) // Remove from cache first
	err := conn.Close()
	if err != nil {
		kr.Log.Warnf("Error closing connector for host %s (%s): %v", host.GetName(), host.GetAddress(), err)
		// Don't necessarily return error, as we've removed from cache.
	}
	return err // Return error from Close()
}

// CloseAllConnectors closes all cached connectors.
func (kr *KubeRuntime) CloseAllConnectors() {
	kr.connectorCacheLock.Lock()
	defer kr.connectorCacheLock.Unlock()

	kr.Log.Info("Closing all cached connectors...")
	for hostID, conn := range kr.connectorCache {
		kr.Log.Debugf("Closing connector for host ID: %s", hostID)
		if err := conn.Close(); err != nil {
			kr.Log.Warnf("Error closing connector for host ID %s: %v", hostID, err)
		}
	}
	kr.connectorCache = make(map[string]connector.Connector) // Clear the cache
	kr.Log.Info("All cached connectors closed and cache cleared.")
}

// GetAllHosts returns all validated hosts associated with this KubeRuntime.
func (kr *KubeRuntime) GetAllHosts() []connector.Host {
	// Return a copy to prevent external modification
	hostsCopy := make([]connector.Host, len(kr.AllHosts))
	copy(hostsCopy, kr.AllHosts)
	return hostsCopy
}

// GetRoleHosts returns hosts for a specific role.
// Returns an empty slice if the role is not defined or has no hosts.
func (kr *KubeRuntime) GetRoleHosts(role string) []connector.Host {
	if hosts, found := kr.RoleHosts[role]; found {
		hostsCopy := make([]connector.Host, len(hosts))
		copy(hostsCopy, hosts)
		return hostsCopy
	}
	return []connector.Host{} // Return empty slice, not nil, for consistency
}

// Implement runtime.Runtime interface methods (if KubeRuntime is to satisfy it directly)
// These are getters for the globally configured operational parameters.
// func (kr *KubeRuntime) WorkDir() string     { return kr.WorkDir }
// func (kr *KubeRuntime) IgnoreError() bool   { return kr.IgnoreError }
// func (kr *KubeRuntime) Verbose() bool       { return kr.Verbose }
// func (kr *KubeRuntime) ObjectName() string  { return "" } // This might need a better source if KubeRuntime is generic

// GetHostConnectorAndRunner is part of the old runtime.Runtime interface.
// KubeRuntime manages connectors via GetConnector. Runners are not directly managed here yet.
// If KubeRuntime needs to implement runtime.Runtime, this would need to be addressed.
// For now, new pipelines/modules will use KubeRuntime.GetConnector.
// func (kr *KubeRuntime) GetHostConnectorAndRunner(host connector.Host) (connector.Connector, runner.Runner, error) {
// 	conn, err := kr.GetConnector(host)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	// Runner creation logic would be needed here.
// 	// For now, returning nil for runner.
// 	return conn, nil, nil
// }

// LogEntry returns the base logger for this runtime.
// Pipelines/Modules/Tasks can create scoped loggers from this.
func (kr *KubeRuntime) LogEntry() *logrus.Entry {
	return kr.Log
}
