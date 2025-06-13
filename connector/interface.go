package connector

import (
	"context"
	"io"
	"os"
	"time" // Added for Config.Timeout
)

// Config is expected to be the same or compatible with the Config struct in ssh.go
// If it's not defined here, it implies it's either defined in another file in this package
// or this interface might be used with a Config struct passed from outside.
// For now, let's mirror the fields from the existing connector.Config for completeness if needed directly by the interface users.
// However, it's better if the implementation (ssh_connector.go) handles its own config detail.
// So, the interface's Config() method might return the specific config struct of the implementation.
// Let's assume the existing Config from ssh.go is the one to be returned.

// HostInfo represents basic information about a host.
// This might be defined elsewhere (e.g., common package) or kept simple here.
type HostInfo struct {
	Name       string
	Address    string
	Port       int
	User       string
	Password   string
	PrivateKey string
	KeyFile    string
	// Add other relevant fields like Bastion details if the interface needs to expose them,
	// but typically the Connector is configured with these and doesn't expose them again.
}

type Connector interface {
	// Exec executes a command on the remote host.
	// It returns the standard output, standard error, exit code, and any error encountered.
	Exec(ctx context.Context, cmd string) (stdout []byte, stderr []byte, exitCode int, err error)

	// PExec executes a command with connected stdin, stdout, and stderr.
	// Useful for interactive commands or when precise stream control is needed.
	// Note: PTY behavior (merging stdout/stderr) might affect how stderr is captured.
	PExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (exitCode int, err error)

	// UploadFile copies a local file to a remote path.
	UploadFile(ctx context.Context, localPath string, remotePath string) error

	// DownloadFile copies a remote file to a local path.
	DownloadFile(ctx context.Context, remotePath string, localPath string) error

	// Fetch retrieves a remote file as an io.ReadCloser.
	// The caller is responsible for closing the reader.
	Fetch(ctx context.Context, remotePath string) (io.ReadCloser, error)

	// Scp copies data from an io.Reader to a remote file path with specified mode.
	// sizeHint can be used by some implementations for optimization.
	Scp(ctx context.Context, localReader io.Reader, remotePath string, sizeHint int64, mode os.FileMode) error

	// StatRemote retrieves information about a remote file or directory.
	// Returns os.FileInfo (or a compatible interface) and an error.
	StatRemote(ctx context.Context, remotePath string) (os.FileInfo, error)

	// RemoteFileExist checks if a remote file (not directory) exists.
	RemoteFileExist(ctx context.Context, remotePath string) (bool, error)

	// RemoteDirExist checks if a remote directory exists.
	RemoteDirExist(ctx context.Context, remotePath string) (bool, error)

	// MkDirAll creates a directory (and any necessary parents) on the remote host.
	// It applies the specified file mode.
	MkDirAll(ctx context.Context, remotePath string, mode os.FileMode) error

	// Chmod changes the permissions of a remote file or directory.
	Chmod(ctx context.Context, remotePath string, mode os.FileMode) error

	// Close terminates the connection to the remote host.
	Close() error

	// GetConfig returns the configuration used to set up this connector.
	// This should return the actual Config struct from ssh.go (or a compatible one).
	// For now, we refer to the existing Config struct in the connector package.
	GetConfig() Config
}

// Note: The 'Config' struct referenced by GetConfig() is the one from ssh.go.
// Ensure that ssh.go's Config is accessible within the package if this interface
// is to be implemented by types in the same package.
// If ssh.go's Config is not public or intended for direct use by interface consumers,
// GetConfig() might need to return a different, more abstract config representation.
// Given the existing code, it's likely the ssh.go Config.


// Host defines the interface for a target host in the cluster.
// It encapsulates connection details and host-specific properties.
type Host interface {
	GetName() string
	SetName(name string)
	GetAddress() string
	SetAddress(addr string)
	GetInternalAddress() string
	SetInternalAddress(addr string)
	GetInternalIPv4Address() string
	GetInternalIPv6Address() string
	SetInternalAddresses(ipv4, ipv6 string)
	GetPort() int
	SetPort(port int)
	GetUser() string
	SetUser(u string)
	GetPassword() string
	SetPassword(password string)
	GetPrivateKey() string
	SetPrivateKey(privateKey string)
	GetPrivateKeyPath() string
	SetPrivateKeyPath(path string)
	GetArch() string
	SetArch(arch string)
	GetTimeout() time.Duration
	SetTimeout(timeout time.Duration)
	GetRoles() []string
	SetRoles(roles []string)
	AddRole(role string)
	RemoveRole(role string)
	IsRole(role string) bool
	GetVars() map[string]interface{}
	SetVars(vars map[string]interface{})
	GetVar(key string) (value interface{}, exists bool)
	SetVar(key string, value interface{})
	Validate() error
	ID() string
}
