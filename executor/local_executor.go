package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall" // For exit code
)

// localExecutor implements the Executor interface for local machine operations.
type localExecutor struct{}

// NewLocalExecutor creates a new Executor for local operations.
func NewLocalExecutor() Executor {
	return &localExecutor{}
}

func (l *localExecutor) executeLocalCommand(ctx context.Context, command string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = 1 // Default if status cannot be determined
			}
		} else {
			exitCode = 1 // Default for other errors (e.g., command not found)
			// Return the error as well for context like "command not found"
			return stdout.String(), stderr.String(), exitCode, fmt.Errorf("failed to run command '%s %s': %w", command, strings.Join(args, " "), err)
		}
	}
	// If cmd.Run() returned an error that was an ExitError, we still return nil for the error in Execute method signature
	// as the error is represented by the non-zero exit code.
	// However, for other errors (like command not found), we should return the error.
	if err != nil && exitCode != 0 { // An actual execution error occurred beyond just a non-zero exit
	    if _, ok := err.(*exec.ExitError); !ok {
             return stdout.String(), stderr.String(), exitCode, fmt.Errorf("command execution failed: %w", err)
        }
    }
	return stdout.String(), stderr.String(), exitCode, nil
}

func (l *localExecutor) Execute(ctx context.Context, command string) (string, string, int, error) {
	// Simple split for command, assumes no complex shell scripting needed directly in 'command' string.
	// For more complex cases, one might pass "bash", "-c", command.
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", "", 0, fmt.Errorf("empty command")
	}
	return l.executeLocalCommand(ctx, parts[0], parts[1:]...)
}

func (l *localExecutor) SudoExecute(ctx context.Context, command string) (string, string, int, error) {
	// Similar to remote, sudo -E /bin/bash -c "command"
	// However, for local execution, often just prepending "sudo" is enough if PATH is set.
	// For consistency and to handle complex commands, using bash -c is safer.
	bashCmd := fmt.Sprintf("sudo -E /bin/bash -c \"%s\"", strings.ReplaceAll(command, `"`, `\"`))
	return l.executeLocalCommand(ctx, "/bin/bash", "-c", bashCmd) // Using /bin/bash -c to interpret the sudo string
}

func (l *localExecutor) CopyFileToRemote(ctx context.Context, localPath, targetPath string, permissions os.FileMode) error {
	return l.copyLocalFile(localPath, targetPath, permissions)
}

func (l *localExecutor) CopyFileFromRemote(ctx context.Context, targetPath, localPath string) error {
	// For local executor, "remote" is also local. So, it's a local to local copy.
	// Get permissions from source to apply to dest, unless default is fine.
	sourceFileStat, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("failed to stat source file %s: %w", targetPath, err)
	}
	return l.copyLocalFile(targetPath, localPath, sourceFileStat.Mode().Perm())
}

func (l *localExecutor) copyLocalFile(src, dst string, perm os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", src, err)
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create directory for destination file %s: %w", dst, err)
	}

	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", dst, err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy data from %s to %s: %w", src, dst, err)
	}
	return destFile.Chmod(perm) // Ensure correct permissions are set
}


func (l *localExecutor) FetchRemoteFile(ctx context.Context, targetPath string) (io.ReadCloser, error) {
	return os.Open(targetPath)
}

func (l *localExecutor) GetRemoteFile(ctx context.Context, targetPath, localPath string) error {
	return l.CopyFileFromRemote(ctx, targetPath, localPath)
}

func (l *localExecutor) PutRemoteFile(ctx context.Context, localPath, targetPath string, permissions os.FileMode) error {
	return l.CopyFileToRemote(ctx, localPath, targetPath, permissions)
}

func (l *localExecutor) CreateRemoteDirectory(ctx context.Context, path string, permissions os.FileMode) error {
	return os.MkdirAll(path, permissions)
}

func (l *localExecutor) StatRemote(ctx context.Context, path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (l *localExecutor) RemoteFileExists(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

func (l *localExecutor) RemoteDirExists(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (l *localExecutor) ChmodRemote(ctx context.Context, path string, permissions os.FileMode) error {
	return os.Chmod(path, permissions)
}
