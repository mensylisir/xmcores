package connector

import (
	"context"
	"io"
	"os"
)

// sshConnector implements the Connector interface using the existing ssh connection logic.
type sshConnector struct {
	// conn is the underlying connection object, assumed to be of the Connection interface type
	// returned by NewConnection (from ssh.go).
	conn Connection
	cfg  Config     // Store the config for the GetConfig() method
}

// NewSSHConnector creates a new Connector that uses the SSH protocol.
// It initializes an SSH connection based on the provided configuration.
func NewSSHConnector(cfg Config) (Connector, error) {
	c, err := NewConnection(cfg) // NewConnection is the existing function from ssh.go
	if err != nil {
		return nil, err
	}
	return &sshConnector{conn: c, cfg: cfg}, nil
}

func (s *sshConnector) Exec(ctx context.Context, cmd string) (stdout []byte, stderr []byte, exitCode int, err error) {
	return s.conn.Exec(ctx, cmd)
}

func (s *sshConnector) PExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (exitCode int, err error) {
	return s.conn.PExec(ctx, cmd, stdin, stdout, stderr)
}

func (s *sshConnector) UploadFile(ctx context.Context, localPath string, remotePath string) error {
	return s.conn.UploadFile(ctx, localPath, remotePath)
}

func (s *sshConnector) DownloadFile(ctx context.Context, remotePath string, localPath string) error {
	return s.conn.DownloadFile(ctx, remotePath, localPath)
}

func (s *sshConnector) Fetch(ctx context.Context, remotePath string) (io.ReadCloser, error) {
	return s.conn.Fetch(ctx, remotePath)
}

func (s *sshConnector) Scp(ctx context.Context, localReader io.Reader, remotePath string, sizeHint int64, mode os.FileMode) error {
	return s.conn.Scp(ctx, localReader, remotePath, sizeHint, mode)
}

func (s *sshConnector) StatRemote(ctx context.Context, remotePath string) (os.FileInfo, error) {
	return s.conn.StatRemote(ctx, remotePath)
}

func (s *sshConnector) RemoteFileExist(ctx context.Context, remotePath string) (bool, error) {
	return s.conn.RemoteFileExist(ctx, remotePath)
}

func (s *sshConnector) RemoteDirExist(ctx context.Context, remotePath string) (bool, error) {
	return s.conn.RemoteDirExist(ctx, remotePath)
}

func (s *sshConnector) MkDirAll(ctx context.Context, remotePath string, mode os.FileMode) error {
	return s.conn.MkDirAll(ctx, remotePath, mode)
}

func (s *sshConnector) Chmod(ctx context.Context, remotePath string, mode os.FileMode) error {
	return s.conn.Chmod(ctx, remotePath, mode)
}

func (s *sshConnector) Close() error {
	return s.conn.Close()
}

func (s *sshConnector) GetConfig() Config {
	return s.cfg
}
