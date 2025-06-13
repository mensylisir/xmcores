package connector

import (
	"fmt"
	// ssh.go's NewConnection will be used by the concrete sshDialer.
	// Ensure necessary imports are in ssh.go or here if directly used.
)

// Dialer defines an interface for creating connections to hosts.
type Dialer interface {
	// Dial attempts to establish a connection to the given host.
	// It uses the host's configuration (address, port, auth details) to make the connection.
	Dial(host Host) (Connector, error)
}

// sshDialer is a concrete implementation of Dialer using SSH.
type sshDialer struct {
	// Potential fields for sshDialer if it needs global config not on Host object,
	// e.g. global proxy, default timeouts if not on host.
}

// NewDialer creates a new default dialer (currently SSHDialer).
// This function provides a concrete implementation of the Dialer interface.
func NewDialer() Dialer {
	return &sshDialer{}
}

// Dial establishes an SSH connection to the host.
// This wraps the logic previously in connector.NewConnection (from ssh.go),
// adapting it to use the Host interface and return a Connector interface.
func (d *sshDialer) Dial(host Host) (Connector, error) {
	if host == nil {
		return nil, fmt.Errorf("host cannot be nil for Dial")
	}

	// Construct connector.Config from connector.Host fields.
	// This assumes Host interface has all necessary getters like GetUser, GetAddress, etc.
	cfg := Config{ // This Config is from ssh.go (package connector)
		Username:         host.GetUser(),
		Password:         host.GetPassword(),
		Address:          host.GetAddress(),
		Port:             host.GetPort(),
		PrivateKey:       host.GetPrivateKey(),
		KeyFile:          host.GetPrivateKeyPath(),
		Timeout:          host.GetTimeout(),
		AgentSocket:      "", // TODO: Add GetAgentSocket() to Host interface if this feature is needed
		// Bastion details would also come from the Host object if it stores them,
		// or from a global bastion configuration if the Dialer is made aware of it.
		// Example:
		// Bastion:            host.GetBastionAddress(),
		// BastionPort:        host.GetBastionPort(),
		// BastionUser:        host.GetBastionUser(),
		// BastionPassword:    host.GetBastionPassword(),
		// BastionPrivateKey:  host.GetBastionPrivateKey(),
		// BastionKeyFile:     host.GetBastionKeyFile(),
	}

	// Call the NewConnection function (defined in ssh.go).
	// NewConnection is already refactored to return (Connector, error).
	return NewConnection(cfg)
}

// Ensure sshDialer implements Dialer.
var _ Dialer = (*sshDialer)(nil)
