package connector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/mensylisir/xmcores/file"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/mensylisir/xmcores/common"
	"github.com/mensylisir/xmcores/logger"
)

type Config struct {
	Username    string
	Password    string
	Address     string
	Port        int
	PrivateKey  string
	KeyFile     string
	AgentSocket string
	Timeout     time.Duration
	Bastion     string
	BastionPort int
	BastionUser string
}

const socketEnvPrefix = "env:"

var _ Connection = (*connection)(nil)

type connection struct {
	mu         sync.Mutex
	sftpclient *sftp.Client
	sshclient  *ssh.Client
	config     Config

	connCtx    context.Context
	connCancel context.CancelFunc

	agentSocketConn net.Conn
}

func NewConnection(cfg Config) (Connection, error) {
	var err error
	cfg, err = validateConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate ssh connection parameters")
	}

	authMethods := make([]ssh.AuthMethod, 0)
	conn := &connection{config: cfg}

	if len(cfg.Password) > 0 {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	if len(cfg.PrivateKey) > 0 {
		signer, parseErr := ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, "the given SSH key could not be parsed")
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if len(cfg.AgentSocket) > 0 {
		addr := cfg.AgentSocket
		if strings.HasPrefix(cfg.AgentSocket, socketEnvPrefix) {
			envName := strings.TrimPrefix(cfg.AgentSocket, socketEnvPrefix)
			if envAddr := os.Getenv(envName); len(envAddr) > 0 {
				addr = envAddr
			} else {
				logger.Log.Warnf("SSH Agent environment variable %s not found, using original socket string %s", envName, addr)
			}
		}

		var dialErr error
		conn.agentSocketConn, dialErr = net.Dial("unix", addr)
		if dialErr != nil {
			return nil, errors.Wrapf(dialErr, "could not open SSH agent socket %q", addr)
		}

		agentClient := agent.NewClient(conn.agentSocketConn)
		signers, signersErr := agentClient.Signers()
		if signersErr != nil {
			_ = conn.agentSocketConn.Close()
			conn.agentSocketConn = nil
			return nil, errors.Wrap(signersErr, "error when creating signer for SSH agent")
		}
		authMethods = append(authMethods, ssh.PublicKeys(signers...))
	}

	sshClientConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Timeout:         cfg.Timeout,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	targetHost := cfg.Address
	targetPort := cfg.Port
	effectiveUser := cfg.Username

	if cfg.Bastion != "" {
		targetHost = cfg.Bastion
		targetPort = cfg.BastionPort
		effectiveUser = cfg.BastionUser
	}

	endpoint := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
	sshClientConfig.User = effectiveUser

	var client *ssh.Client
	client, err = ssh.Dial("tcp", endpoint, sshClientConfig)
	if err != nil {
		conn.cleanupAgentSocket()
		return nil, errors.Wrapf(err, "could not establish connection to %s", endpoint)
	}

	if cfg.Bastion != "" {
		endpointBehindBastion := net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port))
		connToTarget, dialErr := client.Dial("tcp", endpointBehindBastion)
		if dialErr != nil {
			_ = client.Close()
			conn.cleanupAgentSocket()
			return nil, errors.Wrapf(dialErr, "could not establish connection to target %s via bastion", endpointBehindBastion)
		}

		targetSSHConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Timeout:         cfg.Timeout,
			Auth:            authMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		ncc, chans, reqs, clientConnErr := ssh.NewClientConn(connToTarget, endpointBehindBastion, targetSSHConfig)
		if clientConnErr != nil {
			_ = connToTarget.Close()
			_ = client.Close()
			conn.cleanupAgentSocket()
			return nil, errors.Wrapf(clientConnErr, "failed to create new SSH client connection to %s via bastion", endpointBehindBastion)
		}
		_ = client.Close()
		client = ssh.NewClient(ncc, chans, reqs)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		conn.cleanupAgentSocket()
		return nil, errors.Wrapf(err, "failed to create SFTP client: %v", err)
	}

	conn.sshclient = client
	conn.sftpclient = sftpClient
	conn.connCtx, conn.connCancel = context.WithCancel(context.Background())

	return conn, nil
}

func (c *connection) cleanupAgentSocket() {
	if c.agentSocketConn != nil {
		_ = c.agentSocketConn.Close()
		c.agentSocketConn = nil
	}
}

func validateConfig(cfg Config) (Config, error) {
	if len(cfg.Username) == 0 {
		return cfg, errors.New("no username specified for SSH connection")
	}
	if len(cfg.Address) == 0 {
		return cfg, errors.New("no address specified for SSH connection")
	}
	if len(cfg.Password) == 0 && len(cfg.PrivateKey) == 0 && len(cfg.KeyFile) == 0 && len(cfg.AgentSocket) == 0 {
		return cfg, errors.New("must specify at least one of password, private key, keyfile or agent socket")
	}

	if len(cfg.PrivateKey) == 0 && len(cfg.KeyFile) > 0 {
		content, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return cfg, errors.Wrapf(err, "failed to read keyfile %q", cfg.KeyFile)
		}
		cfg.PrivateKey = string(content)
	}

	if cfg.Port <= 0 {
		cfg.Port = 22
	}
	if cfg.Bastion != "" {
		if cfg.BastionPort <= 0 {
			cfg.BastionPort = 22
		}
		if cfg.BastionUser == "" {
			cfg.BastionUser = cfg.Username
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return cfg, nil
}

func (c *connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sshclient == nil && c.sftpclient == nil && c.agentSocketConn == nil {
		return nil
	}

	if c.connCancel != nil {
		c.connCancel()
	}

	var SFTPErr, SSHErr, AgentErr error
	if c.sftpclient != nil {
		SFTPErr = c.sftpclient.Close()
		c.sftpclient = nil
	}
	if c.sshclient != nil {
		SSHErr = c.sshclient.Close()
		c.sshclient = nil
	}
	if c.agentSocketConn != nil {
		AgentErr = c.agentSocketConn.Close()
		c.agentSocketConn = nil
	}

	var combinedErrors []string
	if SFTPErr != nil {
		combinedErrors = append(combinedErrors, fmt.Sprintf("sftp close error: %v", SFTPErr))
	}
	if SSHErr != nil {
		combinedErrors = append(combinedErrors, fmt.Sprintf("ssh close error: %v", SSHErr))
	}
	if AgentErr != nil {
		combinedErrors = append(combinedErrors, fmt.Sprintf("agent socket close error: %v", AgentErr))
	}
	if len(combinedErrors) > 0 {
		return errors.New(strings.Join(combinedErrors, "; "))
	}
	return nil
}

func (c *connection) newSession(ctx context.Context) (*ssh.Session, error) {
	c.mu.Lock()
	client := c.sshclient
	c.mu.Unlock()

	if client == nil {
		return nil, errors.New("ssh connection is closed or not initialized")
	}

	opCtx, opCancel := context.WithCancel(ctx)
	defer opCancel()
	go func() {
		select {
		case <-c.connCtx.Done():
			opCancel()
		case <-opCtx.Done():
		}
	}()

	var sess *ssh.Session
	var err error
	sessionDone := make(chan error, 1)

	go func() {
		s, e := client.NewSession()
		if e != nil {
			sessionDone <- e
			return
		}
		sess = s
		sessionDone <- nil
	}()

	select {
	case <-opCtx.Done():
		return nil, errors.Wrap(opCtx.Err(), "failed to create ssh session (context cancelled)")
	case err = <-sessionDone:
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ssh session")
		}
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if ptyErr := sess.RequestPty("xterm", 100, 50, modes); ptyErr != nil {
		_ = sess.Close()
		return nil, errors.Wrap(ptyErr, "failed to request PTY")
	}
	return sess, nil
}

func (c *connection) Exec(ctx context.Context, cmd string) (stdout []byte, stderr []byte, exitCode int, err error) {
	sess, err := c.newSession(ctx)
	if err != nil {
		return nil, nil, -1, errors.Wrap(err, "failed to create session for Exec")
	}
	defer sess.Close()

	var stderrBuf bytes.Buffer
	sess.Stderr = &stderrBuf

	stdoutPipe, pipeErr := sess.StdoutPipe()
	if pipeErr != nil {
		return nil, stderrBuf.Bytes(), -1, errors.Wrap(pipeErr, "failed to get stdout pipe for Exec")
	}
	stdinPipe, pipeErr := sess.StdinPipe()
	if pipeErr != nil {
		partialStdout, _ := io.ReadAll(stdoutPipe)
		return partialStdout, stderrBuf.Bytes(), -1, errors.Wrap(pipeErr, "failed to get stdin pipe for Exec")
	}

	err = sess.Start(strings.TrimSpace(cmd))
	if err != nil {
		partialStdout, _ := io.ReadAll(stdoutPipe)
		_ = stdinPipe.Close()
		return partialStdout, stderrBuf.Bytes(), -1, errors.Wrapf(err, "failed to start command: %s", cmd)
	}

	var stdoutResultBytes []byte
	var wg sync.WaitGroup
	var sudoHandlingErr error
	var promptHandled bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = stdinPipe.Close() }()

		var outputCollector bytes.Buffer
		reader := bufio.NewReader(stdoutPipe)
		line := ""
		sudoUserForPrompt := c.config.Username

		for {
			b, readErr := reader.ReadByte()
			if readErr != nil {
				if readErr != io.EOF {
					sudoHandlingErr = errors.Wrap(readErr, "error reading stdout for prompt detection")
					logger.Log.Debugf("Exec: Error reading stdout pipe: %v", readErr)
				}
				break
			}
			outputCollector.WriteByte(b)

			if !promptHandled && c.config.Password != "" {
				if b == '\n' {
					line = ""
					continue
				}
				line += string(b)

				sudoPromptPattern := fmt.Sprintf("[sudo] password for %s:", sudoUserForPrompt)
				if (strings.HasPrefix(line, sudoPromptPattern) || strings.HasPrefix(line, "Password:")) && strings.HasSuffix(line, ": ") {
					logger.Log.Debugf("Exec: Sudo/Password prompt detected for command '%s'. Sending password.", cmd)
					_, writeErr := stdinPipe.Write([]byte(c.config.Password + "\n"))
					if writeErr != nil {
						sudoHandlingErr = errors.Wrap(writeErr, "failed to write password for sudo/password prompt")
						logger.Log.Errorf("Exec: Failed to write password: %v", writeErr)
					}
					promptHandled = true
					line = ""
				}
			}
		}
		stdoutResultBytes = outputCollector.Bytes()
	}()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- sess.Wait()
	}()

	var finalErr error

	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGINT)
		select {
		case <-time.After(250 * time.Millisecond):
		case finalErr = <-waitDone:
		}
		_ = sess.Close()
		wg.Wait()

		if finalErr == nil && sudoHandlingErr != nil && !promptHandled && strings.Contains(cmd, "sudo") {
			logger.Log.Warnf("Exec: Command '%s' (cancelled) may have had sudo prompt issue: %v", cmd, sudoHandlingErr)
		}
		return stdoutResultBytes, stderrBuf.Bytes(), -1, errors.Wrap(ctx.Err(), "command execution cancelled")

	case finalErr = <-waitDone:
		wg.Wait()

		exitCode = -1

		if finalErr == nil {
			exitCode = 0
		} else {
			if exitErr, ok := finalErr.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
				finalErr = nil
			}
		}

		if finalErr == nil && sudoHandlingErr != nil && !promptHandled && strings.Contains(cmd, "sudo") {
			logger.Log.Warnf("Exec: Command '%s' may have failed due to sudo prompt issue: %v", cmd, sudoHandlingErr)
		}
		return stdoutResultBytes, stderrBuf.Bytes(), exitCode, finalErr
	}
}

func (c *connection) PExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (exitCode int, err error) {
	sess, err := c.newSession(ctx)
	if err != nil {
		return -1, errors.Wrap(err, "failed to create session for PExec")
	}
	defer sess.Close()

	sess.Stdin = stdin
	sessionStdoutPipe, pipeErr := sess.StdoutPipe()
	if pipeErr != nil {
		return -1, errors.Wrap(pipeErr, "failed to get stdout pipe for PExec")
	}
	sess.Stderr = stderr

	sessionStdinPipe, pipeErr := sess.StdinPipe()
	if pipeErr != nil {
		return -1, errors.Wrap(pipeErr, "failed to get stdin pipe for PExec")
	}

	err = sess.Start(strings.TrimSpace(cmd))
	if err != nil {
		_ = sessionStdinPipe.Close()
		return -1, errors.Wrapf(err, "failed to start command: %s", cmd)
	}

	var wg sync.WaitGroup
	var sudoHandlingErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = sessionStdinPipe.Close() }()

		reader := bufio.NewReader(sessionStdoutPipe)
		line := ""
		promptHandled := false
		sudoUserForPrompt := c.config.Username

		for {
			b, readErr := reader.ReadByte()
			if readErr != nil {
				if readErr != io.EOF {
					sudoHandlingErr = errors.Wrap(readErr, "error reading PExec stdout for prompt detection")
					logger.Log.Debugf("PExec: Error reading stdout pipe: %v", readErr)
				}
				break
			}

			if _, writeErr := stdout.Write([]byte{b}); writeErr != nil {
				sudoHandlingErr = errors.Wrap(writeErr, "error writing to user's stdout in PExec")
				break
			}

			if !promptHandled && c.config.Password != "" {
				if b == '\n' {
					line = ""
					continue
				}
				line += string(b)
				sudoPromptPattern := fmt.Sprintf("[sudo] password for %s:", sudoUserForPrompt)
				if (strings.HasPrefix(line, sudoPromptPattern) || strings.HasPrefix(line, "Password:")) && strings.HasSuffix(line, ": ") {
					logger.Log.Debugf("PExec: Sudo/Password prompt detected for command '%s'. Sending password.", cmd)
					_, writeErr := sessionStdinPipe.Write([]byte(c.config.Password + "\n"))
					if writeErr != nil {
						sudoHandlingErr = errors.Wrap(writeErr, "failed to write password for PExec sudo/password prompt")
					}
					promptHandled = true
					line = ""
				}
			}
		}
	}()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- sess.Wait()
	}()

	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGINT)
		select {
		case <-time.After(250 * time.Millisecond):
		case err = <-waitDone:
		}
		_ = sess.Close()
		wg.Wait()
		return -1, errors.Wrap(ctx.Err(), "PExec command execution cancelled")

	case err = <-waitDone:
		wg.Wait()

		finalErr := err
		exitCode = -1

		if finalErr == nil {
			exitCode = 0
		} else {
			if exitErr, ok := finalErr.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
				finalErr = nil
			}
		}
		if sudoHandlingErr != nil {
			logger.Log.Warnf("PExec: Error during sudo/stdout handling for command '%s': %v", cmd, sudoHandlingErr)
			if finalErr == nil {
			}
		}
		return exitCode, finalErr
	}
}

func (c *connection) DownloadFile(ctx context.Context, remotePath string, localPath string) error {
	escapedRemotePath := escapeShellArg(remotePath)
	cmd := SudoPrefix(fmt.Sprintf("cat %s | base64 --wrap=0", escapedRemotePath)) // Using --wrap=0 for GNU base64

	outputBytes, _, exitCode, err := c.Exec(ctx, cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to exec command for downloading remote file %s (exit code %d)", remotePath, exitCode)
	}
	if exitCode != 0 {
		return errors.Errorf("command for downloading remote file %s failed with exit code %d: %s", remotePath, exitCode, string(outputBytes))
	}

	decodedBytes, decodeErr := base64.StdEncoding.DecodeString(string(outputBytes))
	if decodeErr != nil {
		return errors.Wrapf(decodeErr, "failed to decode base64 content from remote file %s", remotePath)
	}

	if err := file.Mkdir(localPath); err != nil {
		return errors.Wrapf(err, "failed to create local directory for %s", localPath)
	}

	dstFile, createErr := os.Create(localPath)
	if createErr != nil {
		return errors.Wrapf(createErr, "failed to create local file %s", localPath)
	}
	defer dstFile.Close()

	_, writeErr := dstFile.Write(decodedBytes)
	if writeErr != nil {
		return errors.Wrapf(writeErr, "failed to write content to local file %s", localPath)
	}
	return nil
}

func (c *connection) UploadFile(ctx context.Context, localPath string, remotePath string) error {
	fInfo, statErr := os.Stat(localPath)
	if statErr != nil {
		return errors.Wrapf(statErr, "failed to stat local path %s", localPath)
	}

	if fInfo.IsDir() {
		return c.uploadPathRecursive(ctx, localPath, remotePath)
	} else {
		return c.copyFileToRemoteSFTP(ctx, localPath, remotePath)
	}
}

func (c *connection) uploadPathRecursive(ctx context.Context, localSrc string, remoteDst string) error {
	baseRemotePath := remoteDst
	finfo, _ := os.Stat(localSrc)
	if !finfo.IsDir() {
		baseRemotePath = path.Dir(remoteDst)
	}

	if err := c.MkDirAll(ctx, baseRemotePath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create base remote directory %s", baseRemotePath)
	}

	srcFi, err := os.Stat(localSrc)
	if err != nil {
		return errors.Wrapf(err, "failed to stat local source %s", localSrc)
	}

	if !srcFi.IsDir() {
		return c.copyFileToRemoteSFTP(ctx, localSrc, remoteDst)
	}

	if err := c.MkDirAll(ctx, remoteDst, srcFi.Mode().Perm()); err != nil {
		return errors.Wrapf(err, "failed to create remote directory %s", remoteDst)
	}

	entries, err := os.ReadDir(localSrc)
	if err != nil {
		return errors.Wrapf(err, "failed to read local directory %s", localSrc)
	}

	for _, entry := range entries {
		localEntryPath := filepath.Join(localSrc, entry.Name())
		remoteEntryPath := path.Join(remoteDst, entry.Name())

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.IsDir() {
			if err := c.uploadPathRecursive(ctx, localEntryPath, remoteEntryPath); err != nil {
				return err
			}
		} else {
			if err := c.copyFileToRemoteSFTP(ctx, localEntryPath, remoteEntryPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *connection) copyFileToRemoteSFTP(ctx context.Context, localPath string, remotePath string) error {
	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient == nil {
		return errors.New("sftp client not available for file copy")
	}

	performMD5Check := true
	var localMd5 string
	if performMD5Check {
		localMd5, err := file.LocalMd5Sum(localPath)
		if err != nil {
			return errors.Wrapf(err, "failed to calculate MD5 for local file %s", localPath)
		}

		exists, _ := c.RemoteFileExist(ctx, remotePath)
		if exists {
			remoteMd5, _, md5ExitCode, md5Err := c.Exec(ctx, fmt.Sprintf("md5sum %s | cut -d' ' -f1", escapeShellArg(remotePath)))
			if md5Err == nil && md5ExitCode == 0 {
				if strings.TrimSpace(string(remoteMd5)) == localMd5 {
					logger.Log.Debugf("Remote file %s has same MD5 as local, skipping upload.", remotePath)
					return nil
				}
			} else {
				logger.Log.Warnf("Failed to get remote MD5 for %s (code: %d): %v. Proceeding with upload.", remotePath, md5ExitCode, md5Err)
			}
		}
	}

	srcFile, err := os.Open(localPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open local file %s", localPath)
	}
	defer srcFile.Close()

	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return errors.Wrapf(err, "failed to create remote file %s via sftp", remotePath)
	}
	defer dstFile.Close()

	srcFi, err := srcFile.Stat()
	if err != nil {
		return errors.Wrapf(err, "failed to stat local file %s", localPath)
	}

	if err := dstFile.Chmod(srcFi.Mode().Perm()); err != nil {
		logger.Log.Warnf("Failed to chmod remote file %s: %v", remotePath, err)
	}

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return errors.Wrapf(err, "sftp copy from %s to %s failed", localPath, remotePath)
	}

	if performMD5Check {

		remoteMd5Post, _, md5ExitCodePost, md5ErrPost := c.Exec(ctx, fmt.Sprintf("md5sum %s | cut -d' ' -f1", escapeShellArg(remotePath)))
		if md5ErrPost != nil || md5ExitCodePost != 0 {
			logger.Log.Warnf("Failed to get remote MD5 after upload for %s (code: %d): %v. Validation skipped.", remotePath, md5ExitCodePost, md5ErrPost)
		} else if strings.TrimSpace(string(remoteMd5Post)) != localMd5 {
			return errors.Errorf("MD5 checksum mismatch for %s after upload: local %s != remote %s",
				remotePath, localMd5, strings.TrimSpace(string(remoteMd5Post)))
		}
		logger.Log.Debugf("MD5 checksum validated for %s after upload.", remotePath)
	}

	return nil
}

func (c *connection) Fetch(ctx context.Context, remotePath string) (io.ReadCloser, error) {
	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient == nil {
		return nil, errors.New("sftp client is not initialized or connection is closed")
	}

	file, err := sftpClient.Open(remotePath)
	if err != nil {
		return nil, errors.Wrapf(err, "sftp: failed to open remote file %s for fetching", remotePath)
	}

	if ctx.Err() != nil {
		_ = file.Close()
		return nil, ctx.Err()
	}

	return file, nil
}

func (c *connection) Scp(ctx context.Context, localReader io.Reader, remotePath string, sizeHint int64, mode os.FileMode) error {
	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient == nil {
		return errors.New("sftp client is not initialized or connection is closed")
	}

	remoteDir := path.Dir(remotePath)

	if err := c.MkDirAll(ctx, remoteDir, 0755); err != nil {
		logger.Log.Warnf("Failed to ensure remote directory %s exists (continuing with create): %v", remoteDir, err)
	}

	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return errors.Wrapf(err, "sftp: failed to create remote file %s for scp", remotePath)
	}
	defer dstFile.Close()

	if mode == 0 {
		mode = 0644
	}
	if err := dstFile.Chmod(mode.Perm()); err != nil {
		logger.Log.Warnf("sftp: failed to chmod remote file %s to %v: %v. Continuing copy.", remotePath, mode, err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	_, err = io.Copy(dstFile, localReader)
	if err != nil {
		return errors.Wrapf(err, "sftp: failed to stream content to remote %s", remotePath)
	}
	return nil
}

func (c *connection) StatRemote(ctx context.Context, remotePath string) (os.FileInfo, error) {
	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient == nil {
		return nil, errors.New("sftp client is not initialized or connection is closed")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return nil, os.ErrNotExist
		}
		return nil, errors.Wrapf(err, "sftp: failed to stat remote path %s", remotePath)
	}
	return info, nil
}

func (c *connection) RemoteFileExist(ctx context.Context, remotePath string) (bool, error) {
	cmd := SudoPrefix(fmt.Sprintf("test -f %s && echo 1 || echo 0", escapeShellArg(remotePath)))

	stdout, _, exitCode, err := c.Exec(ctx, cmd)
	if err != nil {
		return false, errors.Wrapf(err, "failed to execute command to check file existence for %s", remotePath)
	}

	if exitCode != 0 {
		logger.Log.Debugf("RemoteFileExist: command '%s' exited %d. Output: %s", cmd, exitCode, string(stdout))
		return false, nil
	}

	outputStr := strings.TrimSpace(string(stdout))
	return outputStr == "1", nil
}

func (c *connection) RemoteDirExist(ctx context.Context, remotePath string) (bool, error) {
	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient != nil {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		_, err := sftpClient.ReadDir(remotePath)
		if err == nil {
			return true, nil
		}
		if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return false, nil
		}
		info, statErr := c.StatRemote(ctx, remotePath)
		if statErr == nil {
			return info.IsDir(), nil
		}
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to check remote directory %s with ReadDir, and Stat also failed: %v", remotePath, statErr)
	}

	cmd := SudoPrefix(fmt.Sprintf("test -d %s && echo 1 || echo 0", escapeShellArg(remotePath)))
	stdout, _, exitCode, err := c.Exec(ctx, cmd)
	if err != nil {
		return false, errors.Wrapf(err, "failed to execute command to check dir existence for %s", remotePath)
	}
	if exitCode != 0 {
		return false, nil
	}
	return strings.TrimSpace(string(stdout)) == "1", nil
}

func (c *connection) MkDirAll(ctx context.Context, remotePath string, mode os.FileMode) error {
	var modeStr string
	if mode == 0 {
		modeStr = "0755"
	} else {
		modeStr = "0" + strconv.FormatInt(int64(mode.Perm()), 8)
	}

	var cmd string
	escapedPath := escapeShellArg(remotePath)
	if strings.Contains(remotePath, filepath.Join(common.TmpDirBase, common.AppName)) {
		cmd = SudoPrefix(fmt.Sprintf("mkdir -p -m %s %s", modeStr, escapedPath))
	} else {
		cmd = SudoPrefix(fmt.Sprintf("mkdir -p -m %s %s", modeStr, escapedPath))
	}

	_, _, exitCode, err := c.Exec(ctx, cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to execute mkdirall command for %s (exit code %d)", remotePath, exitCode)
	}
	if exitCode != 0 {
		return errors.Errorf("mkdirall command for %s failed with exit code %d", remotePath, exitCode)
	}
	return nil
}

func (c *connection) Chmod(ctx context.Context, remotePath string, mode os.FileMode) error {
	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()

	if sftpClient != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := sftpClient.Chmod(remotePath, mode)
		if err == nil {
			return nil
		}
		logger.Log.Warnf("SFTP chmod for %s failed: %v. Attempting SSH chmod.", remotePath, err)
	}

	modeStr := "0" + strconv.FormatInt(int64(mode.Perm()), 8)
	cmd := SudoPrefix(fmt.Sprintf("chmod %s %s", modeStr, escapeShellArg(remotePath)))

	_, _, exitCode, err := c.Exec(ctx, cmd)
	if err != nil {
		return errors.Wrapf(err, "ssh chmod command for %s failed (exit code %d)", remotePath, exitCode)
	}
	if exitCode != 0 {
		return errors.Errorf("ssh chmod command for %s failed with exit code %d", remotePath, exitCode)
	}
	return nil
}

func SudoPrefix(cmd string) string {
	return fmt.Sprintf("sudo -E /bin/bash -c %s", escapeShellArg(cmd))
}

func escapeShellArg(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}
