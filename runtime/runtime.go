package runtime

import (
	"fmt"
	"sync"

	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/runner"
)

// baseRuntime implements the Runtime interface.
type baseRuntime struct {
	primaryConnector connector.Connector
	primaryRunner    runner.Runner
	workDir          string
	objectName       string
	verbose          bool
	ignoreError      bool
	allHosts         []connector.Host
	roleHosts        map[string][]connector.Host
	deprecatedHosts  []connector.Host

	// For GetHostConnectorAndRunner caching
	hostResourcesLock sync.Mutex
	hostResources map[string]struct {
		conn connector.Connector
		run  runner.Runner
	}
}

// Config for creating a new baseRuntime.
type Config struct {
	PrimaryConnector connector.Connector
	PrimaryRunner    runner.Runner
	WorkDir          string
	ObjectName       string
	Verbose          bool
	IgnoreError      bool
	AllHosts         []connector.Host
	RoleHosts        map[string][]connector.Host
	DeprecatedHosts  []connector.Host
}

// NewRuntime creates a new instance of Runtime.
func NewRuntime(cfg Config) (Runtime, error) {
	if cfg.WorkDir == "" {
		cfg.WorkDir = "./.xm_workdir"
	}
	if cfg.AllHosts == nil {
		cfg.AllHosts = []connector.Host{}
	}
	if cfg.RoleHosts == nil {
		cfg.RoleHosts = make(map[string][]connector.Host)
	}
	if cfg.DeprecatedHosts == nil {
		cfg.DeprecatedHosts = []connector.Host{}
	}

	return &baseRuntime{
		primaryConnector: cfg.PrimaryConnector,
		primaryRunner:    cfg.PrimaryRunner,
		workDir:          cfg.WorkDir,
		objectName:       cfg.ObjectName,
		verbose:          cfg.Verbose,
		ignoreError:      cfg.IgnoreError,
		allHosts:         cfg.AllHosts,
		roleHosts:        cfg.RoleHosts,
		deprecatedHosts:  cfg.DeprecatedHosts,
		hostResources: make(map[string]struct {
			conn connector.Connector
			run  runner.Runner
		}),
	}, nil
}

func (r *baseRuntime) GetPrimaryConnector() connector.Connector {
	return r.primaryConnector
}

func (r *baseRuntime) GetPrimaryRunner() runner.Runner {
	return r.primaryRunner
}

func (r *baseRuntime) WorkDir() string {
	return r.workDir
}

func (r *baseRuntime) ObjectName() string {
	return r.objectName
}

func (r *baseRuntime) Verbose() bool {
	return r.verbose
}

func (r *baseRuntime) IgnoreError() bool {
	return r.ignoreError
}

func (r *baseRuntime) AllHosts() []connector.Host {
	listCopy := make([]connector.Host, len(r.allHosts))
	copy(listCopy, r.allHosts)
	return listCopy
}

func (r *baseRuntime) RoleHosts() map[string][]connector.Host {
	rhCopy := make(map[string][]connector.Host, len(r.roleHosts))
	for role, hosts := range r.roleHosts {
		hostsCopy := make([]connector.Host, len(hosts))
		copy(hostsCopy, hosts)
		rhCopy[role] = hostsCopy
	}
	return rhCopy
}

func (r *baseRuntime) DeprecatedHosts() []connector.Host {
	listCopy := make([]connector.Host, len(r.deprecatedHosts))
	copy(listCopy, r.deprecatedHosts)
	return listCopy
}

func (r *baseRuntime) GetHostConnectorAndRunner(host connector.Host) (connector.Connector, runner.Runner, error) {
	if host == nil {
		return nil, nil, fmt.Errorf("runtime: host cannot be nil when getting host connector and runner")
	}

	hostID := host.ID()
	if hostID == "" {
		return nil, nil, fmt.Errorf("runtime: host ID is empty for host %s, cannot cache resources", host.GetName())
	}

	r.hostResourcesLock.Lock()
	res, found := r.hostResources[hostID]
	if found {
		r.hostResourcesLock.Unlock()
		return res.conn, res.run, nil
	}
	r.hostResourcesLock.Unlock() // Unlock before potentially long operation

	// Create new connector and runner
	// Ensure all necessary fields from host are used for connector.Config
	// connector.BaseHost has: Name, Address, InternalAddress, Port, User, Password, PrivateKey, PrivateKeyPath, HostArch, ConnectionTimeout
	connCfg := connector.Config{
		Username:    host.GetUser(),
		Password:    host.GetPassword(),
		Address:     host.GetAddress(), // Or GetInternalIPv4Address() if preferred
		Port:        host.GetPort(),
		PrivateKey:  host.GetPrivateKey(),
		KeyFile:     host.GetPrivateKeyPath(),
		Timeout:     host.GetTimeout(),
		// AgentSocket, Bastion details etc., would need to be sourced if required
		// For now, this creates a direct connection config based on host fields.
		UseSudoForFileOps:  false, // Default, can be configurable if needed
		UserForSudoFileOps: host.GetUser(), // Default for chown if sudo is used for files
	}

	newConn, err := connector.NewSSHConnector(connCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: failed to create new SSH connector for host %s (%s): %w", host.GetName(), host.GetAddress(), err)
	}

	newRun := runner.NewCmdRunner(newConn)

	r.hostResourcesLock.Lock()
	defer r.hostResourcesLock.Unlock()

	// Double check if another goroutine created it while we were unlocked
	if res, found = r.hostResources[hostID]; found {
	    // If another goroutine created it, close the one we just made and use the cached one.
        _ = newConn.Close() // Attempt to close the newly created connection as it's not needed.
		return res.conn, res.run, nil
	}

	r.hostResources[hostID] = struct {
		conn connector.Connector
		run  runner.Runner
	}{conn: newConn, run: newRun}

	return newConn, newRun, nil
}
