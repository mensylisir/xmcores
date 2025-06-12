package runner

import (
	"context"
)

// Runner defines an interface for executing commands, potentially on different targets
// (local or remote, though the initial implementation will focus on remote via a connector).
type Runner interface {
	// Run executes a command.
	// Returns stdout, stderr, exit code, and error.
	Run(ctx context.Context, command string) (stdout string, stderr string, exitCode int, err error)

	// SudoRun executes a command with superuser privileges.
	// How sudo is achieved (e.g., passwordless sudo, password prompt via connector)
	// depends on the underlying connector and system configuration.
	// Returns stdout, stderr, exit code, and error.
	SudoRun(ctx context.Context, command string) (stdout string, stderr string, exitCode int, err error)
}
