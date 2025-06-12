package executor

import (
	"context"
	"io" // Required for io.ReadAll
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalExecutor_Execute_SimpleCommands(t *testing.T) {
	le := NewLocalExecutor()
	ctx := context.Background()

	// Test echo command
	stdout, stderr, exitCode, err := le.Execute(ctx, "echo hello world")
	if err != nil {
		t.Fatalf("Execute(echo) failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Execute(echo) exitCode = %d; want 0. stderr: %s", exitCode, stderr)
	}
	if strings.TrimSpace(stdout) != "hello world" {
		t.Errorf("Execute(echo) stdout = %q; want %q", stdout, "hello world")
	}

	// Test a command that produces stderr and non-zero exit code
	// Using a command that is unlikely to exist to force an error handled by executeLocalCommand's os/exec part
	nonExistentCmd := "a_very_unlikely_command_to_exist_xyz123"
	stdout, stderr, exitCode, err = le.Execute(ctx, nonExistentCmd)

	// We expect an error because the command likely doesn't exist.
	// The 'err' from Execute should wrap the actual error from os/exec
	if err == nil {
		t.Errorf("Execute(%s) expected an error due to command not found, but got nil", nonExistentCmd)
	}
	// Exit code might be specific to OS or shell when command not found,
	// often 1, 127, or other. Here we check it's non-zero.
	// Our executeLocalCommand sets exitCode = 1 for "command not found" type errors if not ExitError.
	// If it IS an ExitError (e.g. script exiting with error), it takes that code.
	// For "command not found", it's often not an ExitError.
	if exitCode == 0 {
		t.Errorf("Execute(%s) exitCode = 0; want non-zero. stdout: %s, stderr: %s", nonExistentCmd, stdout, stderr)
	}
	if stderr == "" && err != nil && !strings.Contains(err.Error(), "executable file not found") && !strings.Contains(err.Error(), "cannot find the file") {
		// Sometimes stderr might be empty if the error is about finding the command itself.
		// But we expect some indication of error.
		t.Logf("Execute(%s) stderr was empty, err: %v", nonExistentCmd, err)
	}
	t.Logf("Tested non-existent command: stdout='%s', stderr='%s', exitCode=%d, err=%v", stdout, stderr, exitCode, err)
}

func TestLocalExecutor_SudoExecute_Conceptual(t *testing.T) {
	// Testing sudo execution properly requires a passwordless sudo setup for the test user
	// or a way to mock the sudo prompt, which is complex for unit tests.
	// This test will conceptually check if the command is prefixed with sudo.
	// We can't easily verify if it *actually* ran with sudo without specific environment setup.
	le := NewLocalExecutor()
	ctx := context.Background()

	// We'll use a command that prints the user ID.
	// If run with actual sudo, it would print root's ID (0).
	// Here, we are mostly checking the command formation and that it *tries* to run.
	// This is more of an integration test if we check UID.
	// For a unit test, we can only check if it attempts to run *something*.
	var cmdToRun string
	if runtime.GOOS == "windows" {
		// 'whoami' is available on Windows. Sudo is not applicable.
		// This test is primarily for Unix-like systems.
		t.Skip("SudoExecute test is not applicable on Windows")
		return
	}
	cmdToRun = "whoami" // This will be wrapped by sudo -E /bin/bash -c "whoami"

	t.Logf("Conceptually testing SudoExecute with '%s'. This test does not verify if UID is root.", cmdToRun)
	stdout, stderr, exitCode, err := le.SudoExecute(ctx, cmdToRun)

	// If sudo requires a password and it's not set up for passwordless, this command will likely fail
	// or hang if it were interactive (but os/exec is not).
	// We're checking that it ran and exited, not necessarily successfully as root.
	if err != nil {
		t.Logf("SudoExecute(whoami) returned error (as expected if sudo needs password): %v", err)
	}
	if exitCode != 0 {
		t.Logf("SudoExecute(whoami) exitCode = %d (expected if sudo needs password or 'whoami' not in secure_path). stderr: %s, stdout: %s", exitCode, stderr, stdout)
	}
	// If sudo is passwordless and 'whoami' runs, stdout might be 'root' or current user if sudo failed.
	t.Logf("SudoExecute(whoami) stdout: %s", stdout)
	// This test is weak for SudoExecute due to external dependencies.
}

func TestLocalExecutor_FileOperations(t *testing.T) {
	le := NewLocalExecutor()
	ctx := context.Background()

	// Create a temporary directory for testing file operations
	tmpDir, err := os.MkdirTemp("", "localexec_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcFilePath := filepath.Join(tmpDir, "source.txt")
	dstFilePath := filepath.Join(tmpDir, "destination.txt")
	dstDir := filepath.Join(tmpDir, "testdir")
	content := "hello from local executor test"

	// Create source file
	err = os.WriteFile(srcFilePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Test PutRemoteFile (local copy)
	perm := os.FileMode(0600)
	err = le.PutRemoteFile(ctx, srcFilePath, dstFilePath, perm)
	if err != nil {
		t.Fatalf("PutRemoteFile failed: %v", err)
	}

	// Verify destination file content and permissions
	dstContent, err := os.ReadFile(dstFilePath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if string(dstContent) != content {
		t.Errorf("PutRemoteFile content mismatch: got %q, want %q", string(dstContent), content)
	}
	dstStat, err := os.Stat(dstFilePath)
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}
	if runtime.GOOS != "windows" && dstStat.Mode().Perm() != perm { // Windows handles permissions differently
		t.Errorf("PutRemoteFile permission mismatch: got %s, want %s", dstStat.Mode().Perm(), perm)
	}

	// Test GetRemoteFile (local copy)
	copiedBackPath := filepath.Join(tmpDir, "source_copied_back.txt")
	err = le.GetRemoteFile(ctx, dstFilePath, copiedBackPath)
	if err != nil {
		t.Fatalf("GetRemoteFile failed: %v", err)
	}
	copiedBackContent, err := os.ReadFile(copiedBackPath)
	if err != nil {
		t.Fatalf("Failed to read copied back file: %v", err)
	}
	if string(copiedBackContent) != content {
		t.Errorf("GetRemoteFile content mismatch: got %q, want %q", string(copiedBackContent), content)
	}


	// Test CreateRemoteDirectory
	dirPerm := os.FileMode(0750)
	err = le.CreateRemoteDirectory(ctx, dstDir, dirPerm)
	if err != nil {
		t.Fatalf("CreateRemoteDirectory failed: %v", err)
	}
	dirStat, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("Failed to stat created directory: %v", err)
	}
	if !dirStat.IsDir() {
		t.Errorf("CreateRemoteDirectory did not create a directory")
	}
	if runtime.GOOS != "windows" && dirStat.Mode().Perm() != dirPerm {
		t.Errorf("CreateRemoteDirectory permission mismatch: got %s, want %s", dirStat.Mode().Perm(), dirPerm)
	}

	// Test RemoteFileExists
	exists, err := le.RemoteFileExists(ctx, dstFilePath)
	if err != nil {
		t.Fatalf("RemoteFileExists failed: %v", err)
	}
	if !exists {
		t.Errorf("RemoteFileExists: file %s should exist but reported as not existing", dstFilePath)
	}
	exists, _ = le.RemoteFileExists(ctx, dstFilePath+"_nonexistent")
	if exists {
		t.Errorf("RemoteFileExists: reported non-existent file as existing")
	}
    exists, _ = le.RemoteFileExists(ctx, dstDir) // Check on a directory
	if exists {
		t.Errorf("RemoteFileExists: reported directory %s as a file", dstDir)
	}


	// Test RemoteDirExists
	exists, err = le.RemoteDirExists(ctx, dstDir)
	if err != nil {
		t.Fatalf("RemoteDirExists failed: %v", err)
	}
	if !exists {
		t.Errorf("RemoteDirExists: directory %s should exist but reported as not existing", dstDir)
	}
    exists, _ = le.RemoteDirExists(ctx, dstDir+"_nonexistent")
	if exists {
		t.Errorf("RemoteDirExists: reported non-existent directory as existing")
	}
    exists, _ = le.RemoteDirExists(ctx, dstFilePath) // Check on a file
	if exists {
		t.Errorf("RemoteDirExists: reported file %s as a directory", dstFilePath)
	}


	// Test ChmodRemote
	newPerm := os.FileMode(0777)
	err = le.ChmodRemote(ctx, dstFilePath, newPerm)
	if err != nil {
		t.Fatalf("ChmodRemote failed: %v", err)
	}
	if runtime.GOOS != "windows" {
		stat, _ := os.Stat(dstFilePath)
		if stat.Mode().Perm() != newPerm {
			t.Errorf("ChmodRemote permission mismatch: got %s, want %s", stat.Mode().Perm(), newPerm)
		}
	}

    // Test FetchRemoteFile
    reader, err := le.FetchRemoteFile(ctx, srcFilePath)
    if err != nil {
        t.Fatalf("FetchRemoteFile failed: %v", err)
    }
    defer reader.Close()
    fetchedContentBytes, err := io.ReadAll(reader)
    if err != nil {
        t.Fatalf("Failed to read from fetched file stream: %v", err)
    }
    if string(fetchedContentBytes) != content {
        t.Errorf("FetchRemoteFile content mismatch: got %s, want %s", string(fetchedContentBytes), content)
    }
}
