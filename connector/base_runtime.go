package connector

import (
	"fmt"
	"sync"
	// "time" // Not directly needed by BaseRuntime methods shown, but Host might use it for timeout

	// "github.com/mensylisir/xmcores/common/logger" // Example path if logger is centralized
	"github.com/sirupsen/logrus"
)

// BaseRuntime provides common runtime functionalities, including host management and connector caching.
// It's intended to be embedded or composed into more specific runtime structures like ClusterRuntime.
type BaseRuntime struct {
	ClusterName    string
	Dialer         Dialer // Dialer for creating new connections
	Logger         *logrus.Entry
	Debug          bool
	IgnoreErr      bool // Should this be IgnoreError for consistency? Using KubeKey's IgnoreErr for now.

	allHosts       []Host // Uses the connector.Host interface
	roleMap        map[string][]Host
	hostsMu        sync.RWMutex

	ConnectorCache     map[string]Connector
	connectorCacheLock sync.Mutex
}

// NewBaseRuntime is the constructor for BaseRuntime.
// baseLogger is the root logger from which BaseRuntime's logger will be derived.
func NewBaseRuntime(clusterName string, dialer Dialer, debug bool, ignoreErr bool, baseLogger *logrus.Entry) *BaseRuntime {
	if baseLogger == nil {
		// Fallback, though a logger should always be provided.
		baseLogger = logrus.NewEntry(logrus.New())
	}
	return &BaseRuntime{
		ClusterName:    clusterName,
		Dialer:         dialer,
		Debug:          debug,
		IgnoreErr:      ignoreErr,
		Logger:         baseLogger.WithField("runtime_base", clusterName),
		allHosts:       make([]Host, 0),
		roleMap:        make(map[string][]Host),
		ConnectorCache: make(map[string]Connector),
	}
}

// AppendHost adds a host to the BaseRuntime's list of all hosts.
func (br *BaseRuntime) AppendHost(host Host) {
	if host == nil {
		br.Logger.Warn("Attempted to append a nil host.")
		return
	}
	br.hostsMu.Lock()
	defer br.hostsMu.Unlock()
	br.allHosts = append(br.allHosts, host)
	br.Logger.Debugf("Appended host '%s' (%s) to BaseRuntime.", host.GetName(), host.GetAddress())
}

// SetAllHosts allows setting all hosts at once, replacing any existing.
// Useful when hosts are processed externally (e.g. by SetDefaultClusterSpec).
func (br *BaseRuntime) SetAllHosts(hosts []Host) {
	br.hostsMu.Lock()
	defer br.hostsMu.Unlock()
	br.allHosts = hosts
	br.Logger.Debugf("Set %d hosts in BaseRuntime.", len(hosts))
}


// SetRoleMap allows setting the entire role to host mapping.
// This is useful if role processing is done externally.
func (br *BaseRuntime) SetRoleMap(roleMap map[string][]Host) {
	br.hostsMu.Lock()
	defer br.hostsMu.Unlock()
	br.roleMap = roleMap
	br.Logger.Debugf("Set %d roles in BaseRuntime's role map.", len(roleMap))
}


// AppendRoleMap adds a host to a given role.
// This method is more aligned with how KubeKey's example seemed to imply usage with host.SetRole first.
// However, a more direct SetRoleMap or AddHostToRole(role, host) might be more common.
// The current implementation assumes roles are already on the host object via host.GetRoles().
// This was based on the prompt's example `baseRt.AppendRoleMap(host, host.GetRoles())`.
func (br *BaseRuntime) AppendRoleMap(host Host, roles []string) {
	if host == nil {
		br.Logger.Warn("Attempted to append roles for a nil host.")
		return
	}
	br.hostsMu.Lock()
	defer br.hostsMu.Unlock()
	for _, role := range roles {
		br.roleMap[role] = append(br.roleMap[role], host)
		br.Logger.Debugf("Appended host '%s' to role '%s' in BaseRuntime.", host.GetName(), role)
	}
}

// GetAllHosts returns all hosts known to the BaseRuntime.
// Returns a copy for safety.
func (br *BaseRuntime) GetAllHosts() []Host {
	br.hostsMu.RLock()
	defer br.hostsMu.RUnlock()
	listCopy := make([]Host, len(br.allHosts))
	copy(listCopy, br.allHosts)
	return listCopy
}

// GetRoleHosts returns hosts belonging to a specific role.
// Returns an empty slice if the role is not found. Returns a copy.
func (br *BaseRuntime) GetRoleHosts(role string) []Host {
	br.hostsMu.RLock()
	defer br.hostsMu.RUnlock()
	if hosts, found := br.roleMap[role]; found {
		listCopy := make([]Host, len(hosts))
		copy(listCopy, hosts)
		return listCopy
	}
	return []Host{}
}

// GetConnector retrieves a cached connector for the given host, or creates a new one using the Dialer.
func (br *BaseRuntime) GetConnector(host Host) (Connector, error) {
	if host == nil {
		return nil, fmt.Errorf("host cannot be nil for GetConnector")
	}
	if br.Dialer == nil {
		return nil, fmt.Errorf("BaseRuntime has no Dialer configured")
	}
	hostID := host.ID() // Host interface needs ID() method

	br.connectorCacheLock.Lock()
	if conn, found := br.ConnectorCache[hostID]; found && conn != nil {
		// TODO: Add health check for cached connectors
		br.connectorCacheLock.Unlock()
		br.Logger.Debugf("Using cached connector for host: %s (%s)", host.GetName(), host.GetAddress())
		return conn, nil
	}
	// Unlock before dialing to prevent holding lock during potentially long operation
	br.connectorCacheLock.Unlock()

	br.Logger.Infof("Creating new connector for host: %s (%s) via Dialer.", host.GetName(), host.GetAddress())
	newConn, err := br.Dialer.Dial(host)
	if err != nil {
		br.Logger.Errorf("Failed to dial host %s (%s): %v", host.GetName(), host.GetAddress(), err)
		return nil, err
	}

	br.connectorCacheLock.Lock()
	defer br.connectorCacheLock.Unlock()
	// Double-check if another goroutine created it in the meantime
	if conn, found := br.ConnectorCache[hostID]; found && conn != nil {
	    br.Logger.Warnf("Connector for host %s (%s) was created concurrently. Closing new one and using existing.", host.GetName(), host.GetAddress())
	    go newConn.Close() // Close the newly created one in a goroutine
	    return conn, nil
	}
	br.ConnectorCache[hostID] = newConn
	br.Logger.Infof("Successfully created and cached new connector for host: %s (%s)", host.GetName(), host.GetAddress())
	return newConn, nil
}

// CloseConnector closes and removes a connector for a specific host from the cache.
func (br *BaseRuntime) CloseConnector(host Host) error {
    if host == nil {
        return fmt.Errorf("host cannot be nil")
    }
    hostID := host.ID()

    br.connectorCacheLock.Lock()
    conn, found := br.ConnectorCache[hostID]
    if found {
        delete(br.ConnectorCache, hostID)
    }
    br.connectorCacheLock.Unlock()

    if !found || conn == nil {
        br.Logger.Debugf("No active connector found in cache for host %s (%s) to close.", host.GetName(), host.GetAddress())
        return nil
    }

    br.Logger.Infof("Closing connector for host: %s (%s)", host.GetName(), host.GetAddress())
    return conn.Close()
}


// CloseAllConnectors closes all connectors in the cache.
func (br *BaseRuntime) CloseAllConnectors() {
	br.connectorCacheLock.Lock()
	// Copy map to avoid holding lock while closing connections
	cacheCopy := make(map[string]Connector, len(br.ConnectorCache))
	for id, conn := range br.ConnectorCache {
		cacheCopy[id] = conn
		delete(br.ConnectorCache, id) // Clear original cache immediately under lock
	}
	br.connectorCacheLock.Unlock()

	br.Logger.Info("Closing all cached connectors...")
	for hostID, conn := range cacheCopy {
		br.Logger.Debugf("Closing connector for host ID: %s", hostID)
		if err := conn.Close(); err != nil {
			br.Logger.Warnf("Error closing connector for host ID %s: %v", hostID, err)
		}
	}
	br.Logger.Info("All cached connectors closed and cache cleared.")
}

// GetLogger returns the base logger for this runtime.
func (br *BaseRuntime) GetLogger() *logrus.Entry {
    return br.Logger
}

// GetIgnoreErr returns the IgnoreErr flag for this runtime.
func (br *BaseRuntime) GetIgnoreErr() bool {
    return br.IgnoreErr
}

// GetDebug returns the Debug flag for this runtime.
func (br *BaseRuntime) GetDebug() bool {
    return br.Debug
}

// GetClusterName returns the cluster name.
func (br *BaseRuntime) GetClusterName() string {
    return br.ClusterName
}

// GetRoleNames returns a slice of defined role names.
func (br *BaseRuntime) GetRoleNames() []string {
	br.hostsMu.RLock()
	defer br.hostsMu.RUnlock()
	names := make([]string, 0, len(br.roleMap))
	for name := range br.roleMap {
		names = append(names, name)
	}
	return names
}
