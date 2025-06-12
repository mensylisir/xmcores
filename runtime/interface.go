package runtime

import (
	"github.com/mensylisir/xmcores/connector" // Assuming this is the module path
	"github.com/mensylisir/xmcores/runner"    // Assuming this is the module path
)

// Runtime defines an interface for accessing overall execution context and configuration.
type Runtime interface {
	// GetPrimaryConnector returns the primary connector instance, if configured.
	// This might be a connector for local operations or a default remote.
	GetPrimaryConnector() connector.Connector

	// GetPrimaryRunner returns the primary runner instance, if configured.
	GetPrimaryRunner() runner.Runner

	WorkDir() string
	ObjectName() string
	Verbose() bool
	IgnoreError() bool

	// AllHosts returns a list of all target hosts for the current operation.
	AllHosts() []connector.Host // Using connector.Host interface

	// RoleHosts returns a map of roles to lists of hosts belonging to those roles.
	RoleHosts() map[string][]connector.Host // Using connector.Host interface

	// DeprecatedHosts returns a list of hosts that are marked as deprecated.
	DeprecatedHosts() []connector.Host // Using connector.Host interface

	// GetHostConnectorAndRunner returns a connector and runner for a specific host.
	// Implementations might cache these.
	GetHostConnectorAndRunner(host connector.Host) (c connector.Connector, r runner.Runner, err error)
}
