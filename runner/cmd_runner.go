package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/connector" // Assuming this is the correct module path
)

// cmdRunner implements the Runner interface using a connector.Connector.
type cmdRunner struct {
	conn connector.Connector
}

// NewCmdRunner creates a new Runner that uses the given connector for command execution.
func NewCmdRunner(conn connector.Connector) Runner {
	return &cmdRunner{conn: conn}
}

// sudoPrefixCommand is a helper function to wrap a command with sudo.
// This is similar to SudoPrefix in existing connector/ssh.go.
// We might want to make that one public and reuse it, or keep this internal to runner.
// For now, define it here for clarity.
func sudoPrefixCommand(command string) string {
	escapedCommand := strings.ReplaceAll(command, `\`, `\\`) // Escape backslashes first
	escapedCommand = strings.ReplaceAll(escapedCommand, `"`, `\"`) // Escape double quotes
	// Using sudo -E to preserve environment variables, /bin/bash -c to execute the command string.
	finalSudoCommand := fmt.Sprintf("sudo -E /bin/bash -c \"%s\"", escapedCommand)
	return finalSudoCommand
}

// Run executes a command using the underlying connector.
func (r *cmdRunner) Run(ctx context.Context, command string) (stdoutStr string, stderrStr string, exitCode int, err error) {
	stdoutBytes, stderrBytes, code, execErr := r.conn.Exec(ctx, command)
	return string(stdoutBytes), string(stderrBytes), code, execErr
}

// SudoRun executes a command with superuser privileges using the underlying connector.
func (r *cmdRunner) SudoRun(ctx context.Context, command string) (stdoutStr string, stderrStr string, exitCode int, err error) {
	sudoCmd := sudoPrefixCommand(command)
	stdoutBytes, stderrBytes, code, execErr := r.conn.Exec(ctx, sudoCmd)
	// Note: The connector's Exec method already handles potential sudo password prompts if PTY is used
	// and the connector is configured for it (as seen in the existing connector/ssh.go).
	return string(stdoutBytes), string(stderrBytes), code, execErr
}
