package connector

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	testSSHAddress    = "172.30.1.13"
	testSSHUser       = "root"
	testSSHPassword   = "Def@u1tpwd"
	testSSHPrivateKey = `
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdzc2gtcn
NhAAAAAwEAAQAAAYEAtdtTELKEn9rBYma7ZHr1ISiiCoCWVVdbKA0tz4P3OabpaxIXknix
vNxn6vVdDhzZnLokdCnG8KJ4rtIEi1yUglx2oZfR76Y154bbX6mFj0TJvL2R+6dm4W7vKY
qWUU8M7leJqw5rtc58IvF82HM22XRNeWB0SGtNHvswnokIZnP3O9fJiBNpfHGjjIdimx1m
mzB+O1QgQhuwpg6P2klGkiLc+PT6IyXfEZ0w8jWwD4t0QhJVNoZY/iastShorhxk+DtpA2
4H0Abw2NkSAAgSDpQyRxXqT/7bwL0Bzpjub7Q6+BNTXNpUblhF4swfZzIfxhyYWpP7N1R6
tS+QQ1UlGVv6ih1cnUP2qbqSMmX189JdvlpdTz53Q6hVbd1c+1iqqaZP0hNdXZIGxKG9SY
7vSEwlcVWPT4EQT+pda5UUPA5I1xWEzw/ELluW14b4ns9JUAxTxGGKmQ/CK+zqwlACHy4S
VMd497sIATqlUf3XHijHlJSqdYynADp0dZzytr51AAAFgFQwco9UMHKPAAAAB3NzaC1yc2
EAAAGBALXbUxCyhJ/awWJmu2R69SEoogqAllVXWygNLc+D9zmm6WsSF5J4sbzcZ+r1XQ4c
2Zy6JHQpxvCieK7SBItclIJcdqGX0e+mNeeG21+phY9Eyby9kfunZuFu7ymKllFPDO5Xia
sOa7XOfCLxfNhzNtl0TXlgdEhrTR77MJ6JCGZz9zvXyYgTaXxxo4yHYpsdZpswfjtUIEIb
sKYOj9pJRpIi3Pj0+iMl3xGdMPI1sA+LdEISVTaGWP4mrLUoaK4cZPg7aQNuB9AG8NjZEg
AIEg6UMkcV6k/+28C9Ac6Y7m+0OvgTU1zaVG5YReLMH2cyH8YcmFqT+zdUerUvkENVJRlb
+oodXJ1D9qm6kjJl9fPSXb5aXU8+d0OoVW3dXPtYqqmmT9ITXV2SBsShvUmO70hMJXFVj0
+BEE/qXWuVFDwOSNcVhM8PxC5blteG+J7PSVAMU8RhipkPwivs6sJQAh8uElTHePe7CAE6
pVH91x4ox5SUqnWMpwA6dHWc8ra+dQAAAAMBAAEAAAGACV/5m8JAMQdzcbGvFmJ6UY/JLr
ZrSZH7yohHZMu+SnQO02y211+udfh8yPGGLwyQsVItP+nJbi1KAGUmQ9LtevzuRq1PbsXI
QJvEol1YW8bliXvSU0FRfeycmq0gy6dCGOVdXPqc1d4Dqz98uqHR4Yrr1YaB6BvT+XVkj7
+rtbBjveuFYaTyiq5HCp8OF8X/vJ9W2pMfKJlJ1X2pr8yYPT9b2d+zJ22z3rIWTF41Kc/8
Gc3dI7bwToXK4HnpT5R6D8vyTIMt2iZXh25YAO2EfZnLEcWJYpsWz8G1fCo3ypppUacMpk
MAU6EHfqKal6HAKGfxO8Xm0HT/h3m3BiY4QpCsJw0nKQ6XBr0M5Nwq1GyDeHq69Y2c3/Qo
iMXN0CYYZGsTU44lVSgC5FZfeMVwnHy5LkwvCsNCZLpjnldx9PNgvUyjUn8u05qRMSgSWG
byVuapvMfYDlwdojYCfxB5/ctFThObPwY1IP9Fkbjhuu/etUUoduHgA+5F0ZcomNYhAAAA
wQCjNkd93ranY5rfLx653c25u/sjVfOOb1eyqwn+/WJRhqG+5vJnh8NrJs34k5QOkAmJ0M
HEBoUF0C6kK5n88on/yqxHgnDaO+R+IyIEZ0Sq0+A9+atCM12lJiyS7c2x+wuq6AwxD8sd
trAcKbjI+XdlL/Mr+0CHhnkpGL8S3pBY2T+vTS6jNyr5xAB43yjYI+MBPs6amtkc3R4Lyw
n4AatHn7KWWuesNBf8O6dQoFTahLO0fLxsQmDOez8JH0SLfaIAAADBANPwi0b1e3ppU29k
vPJ86B7+zcv+sMDzUsKapRtKHoxeXZq+JC3LHUhNY5pyxXm7CJjyxrIXmRCvBCoGgSvzvL
TixFtITiKfQyRp7sOtd7F1CPWUHIDNcCp4uhdtqP7SXfVMCspeYHfsuCYfV7tg1hLg4yq5
1r+prXFd/0XNdzvKSmh/E8EDFViDKTRumZe/qncDGQYY+DCjKBmiFejt5SQFKpnNl4cb2L
jyeKXewwl/1e8NcDPgV+NnPLi9sz7a5QAAAMEA26nD0yqp3HCEKKPY1wXsNab/NHZFQUQH
xeiuKo9CsrYj+v0wS/5PsHQMP6CssSlc86TTiUHN1eLg2wHFQrRPhGrCpk21nncUVYi9DP
fwXeuHGSeWrIcLmhLjvoXvn6IuWzUyrt1r4csJ3cxAkS4IFuUW7pHroxpR71k7gHAI30dj
ptIall2CLuMXAuP5Tc+mSP2p3nsgA0D+rHEx5KM8ccNS2PN9wBLXS0hSLjpSkcq6Uk0q3x
quOQdeZHv4gcxRAAAACnJvb3RAbm9kZTE=
-----END OPENSSH PRIVATE KEY-----
`
	testSSHAgentSocket = os.Getenv("SSH_AUTH_SOCK")

	testBastionAddress  = os.Getenv("TEST_BASTION_ADDRESS")
	testBastionUser     = os.Getenv("TEST_BASTION_USER")
	testBastionPassword = os.Getenv("TEST_BASTION_PASSWORD")
)

func skipIfNoEnvSet(t *testing.T) {
	if testSSHAddress == "" || testSSHUser == "" || (testSSHPassword == "" && testSSHPrivateKey == "") {
		t.Skip("Skipping integration test: TEST_SSH_ADDRESS, TEST_SSH_USER, and (TEST_SSH_PASSWORD or TEST_SSH_PRIVATE_KEY) environment variables not set.")
	}
}

func getTestConfig(t *testing.T) Config {
	port := 22
	address := testSSHAddress
	parts := strings.Split(testSSHAddress, ":")
	if len(parts) == 2 {
		address = parts[0]
		p, err := strconv.Atoi(parts[1])
		if err == nil {
			port = p
		} else {
			t.Logf("Warning: Could not parse port from TEST_SSH_ADDRESS, using default 22. Error: %v", err)
		}
	}

	return Config{
		Address:    address,
		Port:       port,
		Username:   testSSHUser,
		Password:   testSSHPassword,
		PrivateKey: testSSHPrivateKey,
		Timeout:    20 * time.Second,
	}
}

func uniqueRemotePath(baseName string) string {
	return fmt.Sprintf("/tmp/connector_test_%s_%d", baseName, time.Now().UnixNano())
}

func TestNewConnection_PasswordAuth(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	if cfg.Password == "" {
		t.Skip("Skipping password auth test: TEST_SSH_PASSWORD not set")
	}
	cfg.PrivateKey = ""

	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("NewConnection with password auth failed: %v", err)
	}
	if conn == nil {
		t.Fatal("NewConnection returned nil connection with password auth")
	}
	defer conn.Close()

	stdout, _, exitCode, err := conn.Exec(context.Background(), "echo hello")
	if err != nil {
		t.Errorf("Exec after password auth failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exec 'echo hello' exited with %d, expected 0", exitCode)
	}
	if strings.TrimSpace(string(stdout)) != "hello" {
		t.Errorf("Exec 'echo hello' stdout was '%s', expected 'hello'", string(stdout))
	}
}

func TestNewConnection_PrivateKeyAuth(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	if cfg.PrivateKey == "" {
		t.Skip("Skipping private key auth test: TEST_SSH_PRIVATE_KEY not set")
	}
	cfg.Password = ""

	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("NewConnection with private key auth failed: %v", err)
	}
	if conn == nil {
		t.Fatal("NewConnection returned nil connection with private key auth")
	}
	defer conn.Close()

	stdout, _, exitCode, err := conn.Exec(context.Background(), "id -u -n")
	if err != nil {
		t.Errorf("Exec after private key auth failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exec 'id -u -n' exited with %d, expected 0", exitCode)
	}
	if strings.TrimSpace(string(stdout)) != cfg.Username {
		t.Logf("Exec 'id -u -n' stdout was '%s', expected '%s' (this might be ok if user mapping differs)", string(stdout), cfg.Username)
	}
}

func TestConnection_Exec_Simple(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	cmd := "echo 'output to stdout' && echo 'output to stderr' >&2 && exit 0"
	stdout, stderr, exitCode, err := conn.Exec(ctx, cmd)

	if err != nil {
		t.Errorf("Exec failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
	expectedStdout := "output to stdout"
	if strings.TrimSpace(string(stdout)) != expectedStdout {
		t.Errorf("Expected stdout '%s', got '%s'", expectedStdout, string(stdout))
	}
	expectedStderr := "output to stderr"
	if strings.TrimSpace(string(stderr)) != expectedStderr {
		t.Errorf("Expected stderr '%s', got '%s'", expectedStderr, string(stderr))
	}
}

func TestConnection_Exec_Sudo(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	if cfg.Password == "" { // Sudo password test relies on the login password
		t.Skip("Skipping sudo test: TEST_SSH_PASSWORD not set (assumed for sudo)")
	}

	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	// This command assumes the user can sudo and 'whoami' as root will output 'root'
	// Also assumes password prompt will be "[sudo] password for [user]:" or "Password: "
	cmd := "sudo whoami"
	stdout, stderr, exitCode, err := conn.Exec(ctx, cmd)

	if err != nil {
		t.Errorf("Exec with sudo failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Expected sudo command to exit with 0, got %d. Stderr: %s", exitCode, string(stderr))
	}
	// Sudo prompt might be in stdout or stderr depending on TTY and sudoers.
	// Our Exec tries to handle it.
	if strings.TrimSpace(string(stdout)) != "root" {
		t.Errorf("Expected sudo whoami stdout 'root', got '%s'", string(stdout))
	}
	t.Logf("Sudo command stderr: %s", string(stderr)) // Often empty or contains password prompt if not handled
}

func TestConnection_Exec_Failure(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	cmd := "exit 123"
	_, stderr, exitCode, err := conn.Exec(ctx, cmd)

	if err != nil { // Exec itself should not error for non-zero exit codes
		t.Errorf("Exec failed unexpectedly for command that exits non-zero: %v", err)
	}
	if exitCode != 123 {
		t.Errorf("Expected exit code 123, got %d", exitCode)
	}
	t.Logf("Stderr for 'exit 123': %s", string(stderr))
}

func TestConnection_Exec_ContextCancellation(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond) // Short timeout
	defer cancel()

	cmd := "sleep 5" // Command that takes longer than timeout
	_, _, _, err = conn.Exec(ctx, cmd)

	if err == nil {
		t.Errorf("Expected context cancellation error, got nil")
	} else if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestConnection_PExec_Simple(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	var stdoutBuf, stderrBuf bytes.Buffer
	stdinStr := "hello from stdin\n"
	stdin := strings.NewReader(stdinStr)

	cmd := "tee /dev/stderr"
	exitCode, err := conn.PExec(ctx, cmd, stdin, &stdoutBuf, &stderrBuf)

	if err != nil {
		t.Errorf("PExec failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("PExec expected exit code 0, got %d", exitCode)
	}
	if stdoutBuf.String() != stdinStr {
		t.Errorf("PExec stdout mismatch: expected '%s', got '%s'", stdinStr, stdoutBuf.String())
	}
	if stderrBuf.String() != stdinStr { // tee /dev/stderr also copies to stderr
		t.Errorf("PExec stderr mismatch: expected '%s', got '%s'", stdinStr, stderrBuf.String())
	}
}

func TestConnection_UploadDownloadFile(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	content := "This is a test file for upload and download. " + time.Now().String()
	localTmpFile, err := os.CreateTemp("", "connector_test_local_*.txt")
	if err != nil {
		t.Fatalf("Failed to create local temp file: %v", err)
	}
	defer os.Remove(localTmpFile.Name())
	if _, err := localTmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to local temp file: %v", err)
	}
	localTmpFile.Close()

	remoteTestPath := uniqueRemotePath("upload_download.txt")
	defer func() {
		_, _, _, _ = conn.Exec(ctx, fmt.Sprintf("rm -f %s", escapeShellArg(remoteTestPath)))
	}()

	t.Logf("Uploading %s to %s", localTmpFile.Name(), remoteTestPath)
	err = conn.UploadFile(ctx, localTmpFile.Name(), remoteTestPath)
	if err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	stdout, _, exitCode, execErr := conn.Exec(ctx, fmt.Sprintf("cat %s", escapeShellArg(remoteTestPath)))
	if execErr != nil || exitCode != 0 {
		t.Fatalf("Failed to cat remote file %s (code %d): %v. Stdout: %s", remoteTestPath, exitCode, execErr, string(stdout))
	}
	if string(stdout) != content {
		t.Fatalf("Remote file content mismatch after UploadFile. Expected:\n%s\nGot:\n%s", content, string(stdout))
	}

	downloadedLocalPath := localTmpFile.Name() + "_downloaded"
	defer os.Remove(downloadedLocalPath)

	t.Logf("Downloading %s to %s", remoteTestPath, downloadedLocalPath)
	err = conn.DownloadFile(ctx, remoteTestPath, downloadedLocalPath)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	downloadedContent, err := os.ReadFile(downloadedLocalPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file %s: %v", downloadedLocalPath, err)
	}
	if string(downloadedContent) != content {
		t.Fatalf("Downloaded file content mismatch. Expected:\n%s\nGot:\n%s", content, string(downloadedContent))
	}
}

func TestConnection_ScpFetch_Streaming(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	content := "Streaming test content for Scp and Fetch. " + time.Now().String()
	contentReader := strings.NewReader(content)
	remoteTestPath := uniqueRemotePath("scp_fetch_stream.txt")
	fileMode := os.FileMode(0644)

	defer func() {
		_, _, _, _ = conn.Exec(ctx, fmt.Sprintf("rm -f %s", escapeShellArg(remoteTestPath)))
	}()

	t.Logf("Streaming content to %s", remoteTestPath)
	err = conn.Scp(ctx, contentReader, remoteTestPath, int64(len(content)), fileMode)
	if err != nil {
		t.Fatalf("Scp (stream upload) failed: %v", err)
	}

	t.Logf("Streaming content from %s", remoteTestPath)
	remoteReader, err := conn.Fetch(ctx, remoteTestPath)
	if err != nil {
		t.Fatalf("Fetch (stream download) failed: %v", err)
	}
	defer remoteReader.Close()

	downloadedContentBytes, err := io.ReadAll(remoteReader)
	if err != nil {
		t.Fatalf("Failed to read all from fetched stream: %v", err)
	}
	if string(downloadedContentBytes) != content {
		t.Fatalf("Fetched stream content mismatch. Expected:\n%s\nGot:\n%s", content, string(downloadedContentBytes))
	}
}

func TestConnection_FileDirExistence(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	testFile := uniqueRemotePath("existence_test.txt")
	testDir := uniqueRemotePath("existence_test_dir")

	defer func() {
		_, _, _, _ = conn.Exec(ctx, fmt.Sprintf("rm -f %s", escapeShellArg(testFile)))
		_, _, _, _ = conn.Exec(ctx, fmt.Sprintf("rm -rf %s", escapeShellArg(testDir)))
	}()

	exists, err := conn.RemoteFileExist(ctx, testFile)
	if err != nil {
		t.Errorf("RemoteFileExist for non-existent file errored: %v", err)
	}
	if exists {
		t.Errorf("RemoteFileExist: %s reported as existing, but should not", testFile)
	}
	exists, err = conn.RemoteDirExist(ctx, testDir)
	if err != nil {
		t.Errorf("RemoteDirExist for non-existent dir errored: %v", err)
	}
	if exists {
		t.Errorf("RemoteDirExist: %s reported as existing, but should not", testDir)
	}

	_, _, _, err = conn.Exec(ctx, fmt.Sprintf("touch %s", escapeShellArg(testFile)))
	if err != nil {
		t.Fatalf("Failed to touch test file %s: %v", testFile, err)
	}
	exists, err = conn.RemoteFileExist(ctx, testFile)
	if err != nil {
		t.Errorf("RemoteFileExist for existing file errored: %v", err)
	}
	if !exists {
		t.Errorf("RemoteFileExist: %s reported as not existing, but should", testFile)
	}
	exists, err = conn.RemoteDirExist(ctx, testFile)
	if err != nil {
		t.Errorf("RemoteDirExist for a file path errored: %v", err)
	}
	if exists {
		t.Errorf("RemoteDirExist: %s (a file) reported as a directory", testFile)
	}

	err = conn.MkDirAll(ctx, testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to MkDirAll for test dir %s: %v", testDir, err)
	}
	exists, err = conn.RemoteDirExist(ctx, testDir)
	if err != nil {
		t.Errorf("RemoteDirExist for existing dir errored: %v", err)
	}
	if !exists {
		t.Errorf("RemoteDirExist: %s reported as not existing, but should", testDir)
	}
	exists, err = conn.RemoteFileExist(ctx, testDir)
	if err != nil {
		t.Errorf("RemoteFileExist for a dir path errored: %v", err)
	}
	if exists {
		t.Errorf("RemoteFileExist: %s (a dir) reported as a file", testDir)
	}
}

func TestConnection_MkDirAllAndChmod(t *testing.T) {
	skipIfNoEnvSet(t)
	cfg := getTestConfig(t)
	conn, err := NewConnection(cfg)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	baseDir := uniqueRemotePath("perms_test_base")
	// targetDir := filepath.Join(baseDir, "subdir1", "subdir2") // REMOVED
	targetDirRemote := path.Join(baseDir, "subdir1", "subdir2") // Use path.Join for remote paths

	defer func() {
		// Cleanup uses baseDir which is fine
		_, _, _, _ = conn.Exec(ctx, fmt.Sprintf("rm -rf %s", escapeShellArg(baseDir)))
	}()

	mode := os.FileMode(0750)
	t.Logf("Creating remote directory %s with mode %s", targetDirRemote, mode.String())
	err = conn.MkDirAll(ctx, targetDirRemote, mode) // Using targetDirRemote
	if err != nil {
		t.Fatalf("MkDirAll failed for %s: %v", targetDirRemote, err)
	}

	// statCmd should also use targetDirRemote
	statCmd := fmt.Sprintf("stat -c %%a %s", escapeShellArg(targetDirRemote))
	stdout, _, exitCode, execErr := conn.Exec(ctx, statCmd)
	if execErr != nil || exitCode != 0 {
		t.Fatalf("Failed to stat remote dir %s (code %d): %v. Stdout: %s", targetDirRemote, exitCode, execErr, string(stdout))
	}
	perms := strings.TrimSpace(string(stdout))
	if !strings.HasSuffix(perms, "750") {
		t.Errorf("MkDirAll: Expected permissions for %s to be ~%s, got %s", targetDirRemote, mode.String(), perms)
	} else {
		t.Logf("MkDirAll: Permissions for %s are %s", targetDirRemote, perms)
	}

	newMode := os.FileMode(0777)
	t.Logf("Chmod-ing remote directory %s to %s", targetDirRemote, newMode.String())
	err = conn.Chmod(ctx, targetDirRemote, newMode) // Using targetDirRemote
	if err != nil {
		t.Fatalf("Chmod failed for %s: %v", targetDirRemote, err)
	}

	// Re-check permissions, statCmd is still valid
	stdout, _, exitCode, execErr = conn.Exec(ctx, statCmd)
	if execErr != nil || exitCode != 0 {
		t.Fatalf("Failed to stat remote dir %s after chmod (code %d): %v. Stdout: %s", targetDirRemote, exitCode, execErr, string(stdout))
	}
	newPerms := strings.TrimSpace(string(stdout))
	if !strings.HasSuffix(newPerms, "777") {
		t.Errorf("Chmod: Expected permissions for %s to be ~%s, got %s", targetDirRemote, newMode.String(), newPerms)
	} else {
		t.Logf("Chmod: Permissions for %s are now %s", targetDirRemote, newPerms)
	}
}
