package executor

import (
	"context"
	"io"
	"os"
)

// Executor defines an interface for executing commands and managing files
// on a target system (which could be local or remote).
type Executor interface {
	// Execute runs a command on the target system.
	Execute(ctx context.Context, command string) (stdout string, stderr string, exitCode int, err error)

	// SudoExecute runs a command with superuser privileges on the target system.
	SudoExecute(ctx context.Context, command string) (stdout string, stderr string, exitCode int, err error)

	// CopyFileToRemote copies a file from a local path to a path on the target system.
	// For a local executor, 'remotePath' is also on the local system.
	CopyFileToRemote(ctx context.Context, localPath, targetPath string, permissions os.FileMode) error

	// CopyFileFromRemote copies a file from a path on the target system to a local path.
	// For a local executor, 'targetPath' is also on the local system.
	CopyFileFromRemote(ctx context.Context, targetPath, localPath string) error

	// FetchRemoteFile retrieves a file from the target system as an io.ReadCloser.
	// The caller is responsible for closing the reader.
	// For a local executor, this would open a local file.
	FetchRemoteFile(ctx context.Context, targetPath string) (io.ReadCloser, error)

	// GetRemoteFile downloads/copies a file from the target system to a local path.
	// Convenience wrapper around DownloadFile or local copy.
	GetRemoteFile(ctx context.Context, targetPath, localPath string) error

	// PutRemoteFile uploads/copies a local file to the target system.
	// Convenience wrapper around UploadFile/Scp or local copy.
	PutRemoteFile(ctx context.Context, localPath, targetPath string, permissions os.FileMode) error

	// CreateRemoteDirectory creates a directory on the target system.
	CreateRemoteDirectory(ctx context.Context, path string, permissions os.FileMode) error

	// StatRemote retrieves information about a file or directory on the target system.
	StatRemote(ctx context.Context, path string) (os.FileInfo, error)

	// RemoteFileExists checks if a file (not directory) exists on the target system.
	RemoteFileExists(ctx context.Context, path string) (bool, error)

	// RemoteDirExists checks if a directory exists on the target system.
	RemoteDirExists(ctx context.Context, path string) (bool, error)

	// ChmodRemote changes the permissions of a file or directory on the target system.
	ChmodRemote(ctx context.Context, path string, permissions os.FileMode) error
}
