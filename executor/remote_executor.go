package executor

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mensylisir/xmcores/connector" // Assuming module path
	"github.com/mensylisir/xmcores/runner"    // Assuming module path
)

// remoteExecutor implements the Executor interface for remote machine operations.
type remoteExecutor struct {
	conn connector.Connector
	run  runner.Runner
}

// NewRemoteExecutor creates a new Executor for remote operations using the given connector and runner.
func NewRemoteExecutor(conn connector.Connector, run runner.Runner) (Executor, error) {
	if conn == nil {
		return nil, fmt.Errorf("connector cannot be nil for remote executor")
	}
	if run == nil {
		return nil, fmt.Errorf("runner cannot be nil for remote executor")
	}
	return &remoteExecutor{conn: conn, run: run}, nil
}

func (r *remoteExecutor) Execute(ctx context.Context, command string) (string, string, int, error) {
	return r.run.Run(ctx, command)
}

func (r *remoteExecutor) SudoExecute(ctx context.Context, command string) (string, string, int, error) {
	return r.run.SudoRun(ctx, command)
}

func (r *remoteExecutor) CopyFileToRemote(ctx context.Context, localPath, remotePath string, permissions os.FileMode) error {
	// The connector.UploadFile doesn't take permissions directly.
	// Permissions might need to be set with a subsequent ChmodRemote call.
	// Or, if connector.Scp is more appropriate and takes mode:
	// f, err := os.Open(localPath)
	// if err != nil { return err }
	// defer f.Close()
	// stat, err := f.Stat()
	// if err != nil { return err }
	// return r.conn.Scp(ctx, f, remotePath, stat.Size(), permissions)
	err := r.conn.UploadFile(ctx, localPath, remotePath)
	if err != nil {
		return err
	}
	return r.conn.Chmod(ctx, remotePath, permissions)
}

func (r *remoteExecutor) CopyFileFromRemote(ctx context.Context, remotePath, localPath string) error {
	return r.conn.DownloadFile(ctx, remotePath, localPath)
}

func (r *remoteExecutor) FetchRemoteFile(ctx context.Context, remotePath string) (io.ReadCloser, error) {
	return r.conn.Fetch(ctx, remotePath)
}

func (r *remoteExecutor) GetRemoteFile(ctx context.Context, remotePath, localPath string) error {
	return r.conn.DownloadFile(ctx, remotePath, localPath)
}

func (r *remoteExecutor) PutRemoteFile(ctx context.Context, localPath, remotePath string, permissions os.FileMode) error {
	// Similar to CopyFileToRemote, using UploadFile then Chmod, or Scp if available and suitable.
	// Using Scp from connector as it takes mode.
	sourceFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local source file %s: %w", localPath, err)
	}
	defer sourceFile.Close()

	stat, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local source file %s: %w", localPath, err)
	}

	return r.conn.Scp(ctx, sourceFile, remotePath, stat.Size(), permissions)
}


func (r *remoteExecutor) CreateRemoteDirectory(ctx context.Context, path string, permissions os.FileMode) error {
	return r.conn.MkDirAll(ctx, path, permissions)
}

func (r *remoteExecutor) StatRemote(ctx context.Context, path string) (os.FileInfo, error) {
	return r.conn.StatRemote(ctx, path)
}

func (r *remoteExecutor) RemoteFileExists(ctx context.Context, path string) (bool, error) {
	return r.conn.RemoteFileExist(ctx, path)
}

func (r *remoteExecutor) RemoteDirExists(ctx context.Context, path string) (bool, error) {
	return r.conn.RemoteDirExist(ctx, path)
}

func (r *remoteExecutor) ChmodRemote(ctx context.Context, path string, permissions os.FileMode) error {
	return r.conn.Chmod(ctx, path, permissions)
}
