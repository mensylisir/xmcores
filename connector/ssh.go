package connector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/mensylisir/xmcores/util"
	"io" // 确保 io 包已导入以使用 io.ErrClosedPipe 和 io.EOF
	"net"
	"os"
	"path" // 使用 "path" 而不是 "path/filepath" 来处理远程路径，确保使用 '/'
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/pkg/sftp" // sftp 包
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/mensylisir/xmcores/common"
	"github.com/mensylisir/xmcores/file"
	"github.com/mensylisir/xmcores/logger"
)

// Config 存储 SSH 连接配置
type Config struct {
	Username    string
	Password    string // 目标主机的密码
	Address     string
	Port        int
	PrivateKey  string // 目标主机的私钥内容
	KeyFile     string // 目标主机私钥文件的路径
	AgentSocket string // 目标主机的 agent socket

	Timeout time.Duration

	Bastion            string
	BastionPort        int
	BastionUser        string
	BastionPassword    string // bastion 主机的密码
	BastionPrivateKey  string // bastion 主机私钥内容
	BastionKeyFile     string // bastion 主机私钥文件的路径
	BastionAgentSocket string // 可选: bastion 主机的 agent socket

	UseSudoForFileOps  bool   // 文件操作是否使用 sudo
	UserForSudoFileOps string // 使用 sudo 操作文件时的目标用户 (chown)
}

const socketEnvPrefix = "env:"

// SudoPrefix 使用 "bash -c" 将给定的命令字符串包装起来以便用 sudo 执行。
func SudoPrefix(command string) string {
	escapedCommand := strings.ReplaceAll(command, `\`, `\\`)
	escapedCommand = strings.ReplaceAll(escapedCommand, `"`, `\"`)
	finalSudoCommand := fmt.Sprintf("sudo -E /bin/bash -c \"%s\"", escapedCommand)
	return finalSudoCommand
}

// connection 实现 Connection 接口
type connection struct {
	mu                     sync.Mutex
	sftpclient             *sftp.Client
	sshclient              *ssh.Client
	config                 Config
	ctx                    context.Context    // 连接级别的 context
	cancel                 context.CancelFunc // 用于取消连接级别的 context
	agentSocketConn        net.Conn           // 用于目标主机的 Agent Socket 连接
	bastionSSHClient       *ssh.Client        // 到堡垒机主机的 SSH 客户端
	bastionAgentSocketConn net.Conn           // 用于堡垒机主机的 Agent Socket 连接
}

// Ensure connection struct implements the Connector interface.
var _ Connector = (*connection)(nil)

// NewConnection 创建一个新的 Connector 实例 (formerly Connection)
func NewConnection(cfg Config) (Connector, error) { // Changed return type to Connector
	var err error
	cfg, err = validateOptions(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "验证 SSH 连接参数失败")
	}

	connCtx, cancelFn := context.WithCancel(context.Background())

	// --- 目标认证方法 ---
	targetAuthMethods := make([]ssh.AuthMethod, 0)
	var targetAgentSocketConn net.Conn // 保存目标 Agent Socket 连接以便后续关闭

	if len(cfg.Password) > 0 {
		targetAuthMethods = append(targetAuthMethods, ssh.Password(cfg.Password))
	}
	if len(cfg.PrivateKey) > 0 {
		signer, parseErr := ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		if parseErr != nil {
			cancelFn()
			return nil, errors.Wrap(parseErr, "解析目标主机 SSH 私钥失败")
		}
		targetAuthMethods = append(targetAuthMethods, ssh.PublicKeys(signer))
	}
	if len(cfg.AgentSocket) > 0 {
		addr := cfg.AgentSocket
		if strings.HasPrefix(cfg.AgentSocket, socketEnvPrefix) {
			envVal := os.Getenv(strings.TrimPrefix(cfg.AgentSocket, socketEnvPrefix))
			if envVal != "" {
				addr = envVal
			} else {
				logger.Log.Warnf("环境变量 %s 未设置或为空, 目标 SSH Agent Socket 将尝试使用原始值 %s", strings.TrimPrefix(cfg.AgentSocket, socketEnvPrefix), addr)
			}
		}
		socket, dialErr := net.Dial("unix", addr)
		if dialErr != nil {
			cancelFn()
			return nil, errors.Wrapf(dialErr, "打开目标主机 SSH agent socket %q 失败", addr)
		}
		// socket 连接成功，保存它以便后续使用和关闭
		targetAgentSocketConn = socket
		agentClient := agent.NewClient(targetAgentSocketConn)
		signers, signersErr := agentClient.Signers()
		if signersErr != nil {
			_ = targetAgentSocketConn.Close() // 获取 Signers 失败，关闭 socket
			targetAgentSocketConn = nil       // 清理
			cancelFn()
			return nil, errors.Wrap(signersErr, "从目标主机 SSH agent 创建 signer 失败")
		}
		// Signers 获取成功，targetAgentSocketConn 保持打开状态
		targetAuthMethods = append(targetAuthMethods, ssh.PublicKeys(signers...))
	}
	if len(targetAuthMethods) == 0 {
		if targetAgentSocketConn != nil {
			_ = targetAgentSocketConn.Close()
		} // 如果因无认证方法而失败，关闭已打开的
		cancelFn()
		return nil, errors.New("目标主机没有可用的 SSH 认证方法")
	}

	var finalSSHClient *ssh.Client              // 到目标主机的最终 SSH 客户端
	var bastionClient *ssh.Client               // 到堡垒机主机的 SSH 客户端 (如果使用)
	var bastionAgentSocketConnForClose net.Conn // 保存堡垒机 Agent Socket 连接

	if cfg.Bastion != "" { // --- 如果配置了堡垒机 ---
		bastionAuthMethods := make([]ssh.AuthMethod, 0)
		hasExplicitBastionAuth := false

		if len(cfg.BastionPassword) > 0 {
			bastionAuthMethods = append(bastionAuthMethods, ssh.Password(cfg.BastionPassword))
			hasExplicitBastionAuth = true
		}
		if len(cfg.BastionPrivateKey) > 0 {
			signer, parseErr := ssh.ParsePrivateKey([]byte(cfg.BastionPrivateKey))
			if parseErr != nil {
				if targetAgentSocketConn != nil {
					_ = targetAgentSocketConn.Close()
				}
				cancelFn()
				return nil, errors.Wrap(parseErr, "解析 bastion 主机 SSH 私钥失败")
			}
			bastionAuthMethods = append(bastionAuthMethods, ssh.PublicKeys(signer))
			hasExplicitBastionAuth = true
		}
		if len(cfg.BastionAgentSocket) > 0 {
			addr := cfg.BastionAgentSocket
			if strings.HasPrefix(cfg.BastionAgentSocket, socketEnvPrefix) {
				envVal := os.Getenv(strings.TrimPrefix(cfg.BastionAgentSocket, socketEnvPrefix))
				if envVal != "" {
					addr = envVal
				} else {
					logger.Log.Warnf("环境变量 %s 未设置或为空, Bastion SSH Agent Socket 将尝试使用原始值 %s", strings.TrimPrefix(cfg.BastionAgentSocket, socketEnvPrefix), addr)
				}
			}
			bSocket, dialErr := net.Dial("unix", addr)
			if dialErr != nil {
				if targetAgentSocketConn != nil {
					_ = targetAgentSocketConn.Close()
				}
				cancelFn()
				return nil, errors.Wrapf(dialErr, "打开 bastion 主机 SSH agent socket %q 失败", addr)
			}
			bastionAgentSocketConnForClose = bSocket // 保存以便关闭
			agentClient := agent.NewClient(bastionAgentSocketConnForClose)
			signers, signersErr := agentClient.Signers()
			if signersErr != nil {
				_ = bastionAgentSocketConnForClose.Close()
				bastionAgentSocketConnForClose = nil
				if targetAgentSocketConn != nil {
					_ = targetAgentSocketConn.Close()
				}
				cancelFn()
				return nil, errors.Wrap(signersErr, "从 bastion 主机 SSH agent 创建 signer 失败")
			}
			bastionAuthMethods = append(bastionAuthMethods, ssh.PublicKeys(signers...))
			hasExplicitBastionAuth = true
		}

		if !hasExplicitBastionAuth {
			logger.Log.Warnf("没有为 %s@%s 提供特定的 bastion 认证方法。尝试使用目标主机的认证方法连接 bastion。", cfg.BastionUser, cfg.Bastion)
			bastionAuthMethods = targetAuthMethods // 复用目标机的认证方式
		}
		if len(bastionAuthMethods) == 0 {
			if targetAgentSocketConn != nil {
				_ = targetAgentSocketConn.Close()
			}
			if bastionAgentSocketConnForClose != nil {
				_ = bastionAgentSocketConnForClose.Close()
			}
			cancelFn()
			return nil, errors.New("没有可用于 bastion 连接的认证方法")
		}

		bastionSshConfig := &ssh.ClientConfig{
			User:            cfg.BastionUser,
			Timeout:         cfg.Timeout,
			Auth:            bastionAuthMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		bastionEndpoint := net.JoinHostPort(cfg.Bastion, strconv.Itoa(cfg.BastionPort))
		logger.Log.Debugf("通过 bastion %s (用户 %s) 连接目标 %s:%d", bastionEndpoint, cfg.BastionUser, cfg.Address, cfg.Port)

		var dialErr error
		bastionClient, dialErr = ssh.Dial("tcp", bastionEndpoint, bastionSshConfig)
		if dialErr != nil {
			if targetAgentSocketConn != nil {
				_ = targetAgentSocketConn.Close()
			}
			if bastionAgentSocketConnForClose != nil {
				_ = bastionAgentSocketConnForClose.Close()
			}
			cancelFn()
			return nil, errors.Wrapf(dialErr, "连接 bastion 主机 %s (用户 %s) 失败", bastionEndpoint, cfg.BastionUser)
		}
		// bastionClient 连接成功

		endpointBehindBastion := net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port))
		connToTargetViaBastion, dialErr := bastionClient.Dial("tcp", endpointBehindBastion)
		if dialErr != nil {
			_ = bastionClient.Close() // 关闭 bastionClient
			if targetAgentSocketConn != nil {
				_ = targetAgentSocketConn.Close()
			}
			if bastionAgentSocketConnForClose != nil {
				_ = bastionAgentSocketConnForClose.Close()
			}
			cancelFn()
			return nil, errors.Wrapf(dialErr, "通过 bastion %s 拨号目标 %s 失败", bastionEndpoint, endpointBehindBastion)
		}
		// connToTargetViaBastion (隧道) 成功建立

		targetSshClientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Timeout:         cfg.Timeout,
			Auth:            targetAuthMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		ncc, chans, reqs, clientConnErr := ssh.NewClientConn(connToTargetViaBastion, endpointBehindBastion, targetSshClientConfig)
		if clientConnErr != nil {
			_ = connToTargetViaBastion.Close()
			_ = bastionClient.Close()
			if targetAgentSocketConn != nil {
				_ = targetAgentSocketConn.Close()
			}
			if bastionAgentSocketConnForClose != nil {
				_ = bastionAgentSocketConnForClose.Close()
			}
			cancelFn()
			return nil, errors.Wrapf(clientConnErr, "通过 bastion 建立到 %s (用户 %s) 的 SSH 客户端连接失败", endpointBehindBastion, cfg.Username)
		}
		finalSSHClient = ssh.NewClient(ncc, chans, reqs)
		// finalSSHClient (到目标) 成功建立，bastionClient 保留，将在 Close 中关闭

	} else { // --- 直接连接，无堡垒机 ---
		directSshConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Timeout:         cfg.Timeout,
			Auth:            targetAuthMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		endpoint := net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port))
		logger.Log.Debugf("直接连接到 %s (用户 %s)", endpoint, cfg.Username)
		var dialErr error
		finalSSHClient, dialErr = ssh.Dial("tcp", endpoint, directSshConfig)
		if dialErr != nil {
			if targetAgentSocketConn != nil {
				_ = targetAgentSocketConn.Close()
			}
			// bastionClient 在此分支为 nil
			// bastionAgentSocketConnForClose 在此分支为 nil
			cancelFn()
			return nil, errors.Wrapf(dialErr, "直接连接到 %s (用户 %s) 失败", endpoint, cfg.Username)
		}
	}

	// --- 创建 SFTP 客户端 ---
	sftpClient, err := sftp.NewClient(finalSSHClient)
	if err != nil {
		_ = finalSSHClient.Close()
		if bastionClient != nil {
			_ = bastionClient.Close()
		}
		if targetAgentSocketConn != nil {
			_ = targetAgentSocketConn.Close()
		}
		if bastionAgentSocketConnForClose != nil {
			_ = bastionAgentSocketConnForClose.Close()
		}
		cancelFn()
		return nil, errors.Wrapf(err, "创建 SFTP 客户端失败")
	}

	sshConn := &connection{
		sshclient:              finalSSHClient,
		sftpclient:             sftpClient,
		config:                 cfg,
		ctx:                    connCtx,
		cancel:                 cancelFn,
		agentSocketConn:        targetAgentSocketConn,          // 存储目标 agent socket
		bastionSSHClient:       bastionClient,                  // 存储堡垒机 client
		bastionAgentSocketConn: bastionAgentSocketConnForClose, // 存储堡垒机 agent socket
	}
	return sshConn, nil
}

func validateOptions(cfg Config) (Config, error) {
	if len(cfg.Username) == 0 {
		return cfg, errors.New("未指定 SSH 连接的用户名")
	}
	if len(cfg.Address) == 0 {
		return cfg, errors.New("未指定 SSH 连接的地址")
	}

	hasTargetAuthMethod := false
	if len(cfg.Password) > 0 {
		hasTargetAuthMethod = true
	}
	if len(cfg.PrivateKey) > 0 {
		hasTargetAuthMethod = true
	}
	if len(cfg.AgentSocket) > 0 {
		hasTargetAuthMethod = true
	}

	if !hasTargetAuthMethod && len(cfg.KeyFile) > 0 {
		content, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return cfg, errors.Wrapf(err, "读取目标主机密钥文件 %q 失败", cfg.KeyFile)
		}
		cfg.PrivateKey = string(content)
		hasTargetAuthMethod = true
		logger.Log.Debugf("已从文件 %s 读取目标主机私钥", cfg.KeyFile)
	}
	if !hasTargetAuthMethod {
		return cfg, errors.New("必须为目标连接指定密码、私钥内容、私钥文件或 agent socket 中的至少一种")
	}

	if cfg.Port <= 0 {
		cfg.Port = 22
		logger.Log.Debugf("目标主机端口未设置或无效, 使用默认端口 %d", cfg.Port)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
		logger.Log.Debugf("连接超时未设置, 使用默认值 %s", cfg.Timeout)
	}

	if cfg.Bastion != "" {
		if cfg.BastionUser == "" {
			logger.Log.Debugf("Bastion 用户未设置, 将使用目标用户 %s 作为 Bastion 用户", cfg.Username)
			cfg.BastionUser = cfg.Username
		}
		if cfg.BastionPort <= 0 {
			cfg.BastionPort = 22
			logger.Log.Debugf("Bastion 端口未设置或无效, 使用默认端口 %d", cfg.BastionPort)
		}

		hasBastionAuthMethod := false
		if len(cfg.BastionPassword) > 0 {
			hasBastionAuthMethod = true
		}
		if len(cfg.BastionPrivateKey) > 0 {
			hasBastionAuthMethod = true
		}
		if len(cfg.BastionAgentSocket) > 0 {
			hasBastionAuthMethod = true
		}

		if !hasBastionAuthMethod && len(cfg.BastionKeyFile) > 0 {
			bastionKeyContent, err := os.ReadFile(cfg.BastionKeyFile)
			if err != nil {
				return cfg, errors.Wrapf(err, "读取 bastion 密钥文件 %q 失败", cfg.BastionKeyFile)
			}
			cfg.BastionPrivateKey = string(bastionKeyContent)
			hasBastionAuthMethod = true
			logger.Log.Debugf("已从文件 %s 读取 Bastion 私钥", cfg.BastionKeyFile)
		}
	}

	if cfg.UseSudoForFileOps && cfg.UserForSudoFileOps == "" {
		logger.Log.Debugf("UseSudoForFileOps 已启用, 但 UserForSudoFileOps 未设置。将使用目标用户 %s 进行 chown 操作。", cfg.Username)
		cfg.UserForSudoFileOps = cfg.Username
	}
	return cfg, nil
}

func (c *connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	hostInfo := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	if c.sshclient == nil && c.sftpclient == nil && c.bastionSSHClient == nil && c.agentSocketConn == nil && c.bastionAgentSocketConn == nil {
		logger.Log.Debugf("到 %s 的连接已完全关闭或未初始化", hostInfo)
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
		return nil
	}

	logger.Log.Debugf("正在关闭到 %s 的连接 (包括堡垒机和agent sockets)", hostInfo)
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}

	var errs []string

	if c.sftpclient != nil {
		if err := c.sftpclient.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("关闭 sftp 客户端失败: %v", err))
		}
		c.sftpclient = nil
		logger.Log.Debugf("SFTP 客户端已关闭 for %s", hostInfo)
	}

	if c.sshclient != nil { // 到目标的 SSH client
		if err := c.sshclient.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("关闭目标 ssh 客户端失败: %v", err))
		}
		c.sshclient = nil
		logger.Log.Debugf("目标 SSH 客户端已关闭 for %s", hostInfo)
	}

	if c.bastionSSHClient != nil { // 到堡垒机的 SSH client
		if err := c.bastionSSHClient.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("关闭 bastion ssh 客户端失败: %v", err))
		}
		c.bastionSSHClient = nil
		logger.Log.Debugf("Bastion SSH 客户端已关闭 (host: %s)", c.config.Bastion)
	}

	if c.agentSocketConn != nil { // 目标 Agent socket
		logger.Log.Debugf("正在关闭目标 Agent socket 连接 for %s", hostInfo)
		if err := c.agentSocketConn.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("关闭目标 agent socket 连接失败: %v", err))
		}
		c.agentSocketConn = nil
		logger.Log.Debugf("目标 Agent socket 连接已关闭 for %s", hostInfo)
	}

	if c.bastionAgentSocketConn != nil { // 堡垒机 Agent socket
		logger.Log.Debugf("正在关闭堡垒机 Agent socket 连接 (bastion host: %s)", c.config.Bastion)
		if err := c.bastionAgentSocketConn.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("关闭堡垒机 agent socket 连接失败: %v", err))
		}
		c.bastionAgentSocketConn = nil
		logger.Log.Debugf("Bastion Agent socket 连接已关闭 (bastion host: %s)", c.config.Bastion)
	}

	if len(errs) > 0 {
		errMsg := strings.Join(errs, "; ")
		logger.Log.Errorf("关闭到 %s 的连接时发生错误: %s", hostInfo, errMsg)
		return errors.New(errMsg)
	}
	logger.Log.Debugf("到 %s 的连接已成功关闭所有组件", hostInfo)
	return nil
}

func (c *connection) createSession(ctx context.Context) (*ssh.Session, chan struct{}, error) {
	c.mu.Lock()
	if c.sshclient == nil {
		c.mu.Unlock()
		return nil, nil, errors.New("ssh 连接已关闭, 无法创建会话")
	}
	client := c.sshclient
	c.mu.Unlock()

	sess, err := client.NewSession()
	if err != nil {
		return nil, nil, errors.Wrap(err, "创建 ssh 会话失败")
	}

	sessionLifecycleDone := make(chan struct{})

	go func(innerSess *ssh.Session, cmdCtx context.Context, connCtx context.Context, lifecycleChan <-chan struct{}) {
		select {
		case <-cmdCtx.Done():
			logger.Log.Debugf("会话 context (命令级别 %s:%d) 已取消, 尝试关闭会话: %v", c.config.Address, c.config.Port, cmdCtx.Err())
			_ = innerSess.Close()
		case <-connCtx.Done():
			logger.Log.Debugf("连接主 context (%s:%d) 已取消, 尝试关闭会话: %v", c.config.Address, c.config.Port, connCtx.Err())
			_ = innerSess.Close()
		case <-lifecycleChan:
			logger.Log.Debugf("会话生命周期 channel (%s:%d) 已关闭, 监控结束", c.config.Address, c.config.Port)
		}
		logger.Log.Debugf("会话监控 goroutine (%s:%d) 退出", c.config.Address, c.config.Port)
	}(sess, ctx, c.ctx, sessionLifecycleDone)

	modes := ssh.TerminalModes{ssh.ECHO: 0, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", 40, 80, modes); err != nil {
		_ = sess.Close()
		close(sessionLifecycleDone) // 如果Pty失败，我们也需要关闭 lifecycle channel，因为调用者不会得到它
		return nil, nil, errors.Wrap(err, "请求 PTY 失败")
	}

	if errEnv := sess.Setenv("LANG", "en_US.UTF-8"); errEnv != nil {
		logger.Log.Warnf("为 %s:%d 设置 LANG=en_US.UTF-8 失败 (将继续): %v", c.config.Address, c.config.Port, errEnv)
	}
	if errEnv := sess.Setenv("LC_ALL", "en_US.UTF-8"); errEnv != nil {
		logger.Log.Warnf("为 %s:%d 设置 LC_ALL=en_US.UTF-8 失败 (将继续): %v", c.config.Address, c.config.Port, errEnv)
	}

	return sess, sessionLifecycleDone, nil
}

func (c *connection) Exec(ctx context.Context, cmd string) (stdout []byte, stderr []byte, exitCode int, err error) {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[Exec %s] Cmd: %s. (PTY enabled, PTY merges stdout/stderr)", hostAddr, cmd)

	// cmdCtx governs the entire SSH command execution, including session setup and I/O.
	// It's derived from the input ctx.
	cmdCtx, cancelCmdCtx := context.WithCancel(ctx)
	defer cancelCmdCtx() // Ensure cmdCtx is cancelled on Exec return

	sess, sessionLifecycleDone, errSession := c.createSession(cmdCtx) // Pass cmdCtx for session lifecycle
	if errSession != nil {
		return nil, nil, -1, errors.Wrap(errSession, "Exec: 准备命令执行失败")
	}
	defer func() {
		if sessionLifecycleDone != nil {
			close(sessionLifecycleDone)
		}
		sess.Close()
		logger.Log.Debugf("[Exec %s] 会话已关闭 (cmd: %s)", hostAddr, cmd)
	}()

	ptyOutputPipe, errPipe := sess.StdoutPipe()
	if errPipe != nil {
		return nil, nil, -1, errors.Wrap(errPipe, "Exec: 获取 PTY output pipe 失败")
	}

	internalStdinPipe, errPipe := sess.StdinPipe()
	if errPipe != nil {
		return nil, nil, -1, errors.Wrap(errPipe, "Exec: 获取内部 stdin pipe 失败")
	}

	var ptyOutputBuf bytes.Buffer
	stderr = nil // PTY merges stderr, so this will remain nil or empty.
	var wg sync.WaitGroup
	var passwordSentLock sync.Mutex
	passwordSuccessfullySent := false
	sudoPrefixStr := fmt.Sprintf("[sudo] password for %s", c.config.Username)
	passwordSuffixStr := ": "

	// ioGoroutineCtx is specifically for the I/O goroutine's lifecycle.
	// It allows the goroutine to be stopped if cmdCtx (and thus input ctx) is cancelled.
	ioGoroutineCtx, cancelIOGoroutineCtx := context.WithCancel(cmdCtx)
	defer cancelIOGoroutineCtx()

	wg.Add(1)
	go func(goroutineCtx context.Context, ptyDataReader io.Reader, outputBuffer *bytes.Buffer, stdinPipeWriter io.WriteCloser) {
		defer func() {
			logger.Log.Debugf("[Exec-PtyOutput %s] Goroutine EXITING. outputBuffer.Len(): %d", hostAddr, outputBuffer.Len())
			wg.Done()
		}()
		// Use a larger buffer for bufio.Reader if dealing with large outputs,
		// though ReadByte reduces its impact. Default is 4096.
		r := bufio.NewReaderSize(ptyDataReader, 32*1024) // Example: 32KB buffer
		var currentLine string
		logger.Log.Debugf("[Exec-PtyOutput %s] Goroutine 已启动", hostAddr)
		for {
			select {
			case <-goroutineCtx.Done():
				logger.Log.Debugf("[Exec-PtyOutput %s] Goroutine context (goroutineCtx) 已取消, 正在退出: %v", hostAddr, goroutineCtx.Err())
				return
			default:
			}

			b, readErr := r.ReadByte()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					logger.Log.Debugf("[Exec-PtyOutput %s] EOF reached. outputBuffer.Len(): %d", hostAddr, outputBuffer.Len())
				} else {
					if goroutineCtx.Err() == nil { // Log error only if not due to context cancellation
						logger.Log.Warnf("[Exec-PtyOutput %s] 读取 PTY data 字节错误: %v. outputBuffer.Len(): %d", hostAddr, readErr, outputBuffer.Len())
					} else {
						logger.Log.Debugf("[Exec-PtyOutput %s] 读取 PTY data 字节错误 (likely due to context cancellation %v): %v. outputBuffer.Len(): %d", hostAddr, goroutineCtx.Err(), readErr, outputBuffer.Len())
					}
				}
				break // Exit loop on any error, including EOF
			}
			outputBuffer.WriteByte(b)

			if b == '\n' {
				currentLine = ""
			} else {
				currentLine += string(b)
				const maxPromptLineLength = 256
				if len(currentLine) > maxPromptLineLength {
					currentLine = currentLine[len(currentLine)-maxPromptLineLength:]
				}
			}
			passwordSentLock.Lock()
			if c.config.Password != "" && !passwordSuccessfullySent && stdinPipeWriter != nil {
				if (strings.HasPrefix(currentLine, sudoPrefixStr) || strings.HasPrefix(currentLine, "Password")) && strings.HasSuffix(currentLine, passwordSuffixStr) {
					logger.Log.Debugf("[Exec-PtyOutput %s] 检测到密码提示: '%s', 尝试写入密码...", hostAddr, currentLine)
					_, pwWriteErr := stdinPipeWriter.Write([]byte(c.config.Password + "\n"))
					if pwWriteErr != nil {
						if goroutineCtx.Err() == nil && !util.IsErrPipeClosed(pwWriteErr) {
							logger.Log.Errorf("[Exec-PtyOutput %s] 写入 sudo 密码失败: %v", hostAddr, pwWriteErr)
						}
					} else {
						logger.Log.Debugf("[Exec-PtyOutput %s] Sudo 密码已发送.", hostAddr)
					}

					if errCloseStdin := stdinPipeWriter.Close(); errCloseStdin != nil {
						if goroutineCtx.Err() == nil && !util.IsErrPipeClosed(errCloseStdin) {
							logger.Log.Warnf("[Exec-PtyOutput %s] 发送密码后关闭 stdin 出错: %v", hostAddr, errCloseStdin)
						}
					} else {
						logger.Log.Debugf("[Exec-PtyOutput %s] 发送密码后 stdin 已关闭", hostAddr)
					}
					passwordSuccessfullySent = true
					currentLine = ""
				}
			}
			passwordSentLock.Unlock()
		}
		logger.Log.Debugf("[Exec-PtyOutput %s] Goroutine loop finished. Final outputBuffer.Len(): %d", hostAddr, outputBuffer.Len())
	}(ioGoroutineCtx, ptyOutputPipe, &ptyOutputBuf, internalStdinPipe)

	logger.Log.Debugf("[Exec %s] 即将启动命令: %s", hostAddr, cmd)
	if err = sess.Start(cmd); err != nil {
		passwordSentLock.Lock()
		if !passwordSuccessfullySent && internalStdinPipe != nil {
			_ = internalStdinPipe.Close()
			passwordSuccessfullySent = true
		}
		passwordSentLock.Unlock()
		cancelIOGoroutineCtx() // Signal I/O goroutine to stop
		wg.Wait()
		exitCode = -1
		if sshExitErr, ok := errors.Cause(err).(*ssh.ExitError); ok {
			exitCode = sshExitErr.ExitStatus()
		}
		stdout = ptyOutputBuf.Bytes()
		return stdout, stderr, exitCode, errors.Wrapf(err, "启动命令 '%s' 失败", cmd)
	}
	logger.Log.Debugf("[Exec %s] 命令已启动.", hostAddr)

	passwordSentLock.Lock()
	if c.config.Password == "" && !passwordSuccessfullySent && internalStdinPipe != nil {
		logger.Log.Debugf("[Exec %s] 未配置密码, 关闭内部 stdin pipe.", hostAddr)
		if errClose := internalStdinPipe.Close(); errClose != nil && !util.IsErrPipeClosed(errClose) {
			logger.Log.Warnf("[Exec %s] 关闭 stdin pipe (无密码时) 出错: %v", hostAddr, errClose)
		}
		passwordSuccessfullySent = true
	}
	passwordSentLock.Unlock()

	logger.Log.Debugf("[Exec %s] 等待命令完成 (sess.Wait())...", hostAddr)
	waitErr := sess.Wait()
	logger.Log.Debugf("[Exec %s] sess.Wait() 已完成. Wait 错误: %v", hostAddr, waitErr)

	// After sess.Wait(), the remote command is done. ptyOutputPipe should be closed by ssh lib,
	// leading to EOF in the reading goroutine.
	// We don't explicitly cancelIOGoroutineCtx here to stop I/O; EOF should handle it.
	// The defer cancelIOGoroutineCtx() will eventually run. Or if the parent ctx was cancelled.

	passwordSentLock.Lock()
	if c.config.Password != "" && !passwordSuccessfullySent && internalStdinPipe != nil {
		logger.Log.Debugf("[Exec %s] 命令完成, 但密码未发送 (无提示?), 关闭 stdin pipe.", hostAddr)
		if errClose := internalStdinPipe.Close(); errClose != nil && !util.IsErrPipeClosed(errClose) {
			logger.Log.Warnf("[Exec %s] Wait 后关闭未用 stdin pipe 时出错: %v", hostAddr, errClose)
		}
	}
	passwordSentLock.Unlock()

	logger.Log.Debugf("[Exec %s] 等待 PTY 输出 goroutine (wg.Wait())... ptyOutputBuf.Len() before wait: %d", hostAddr, ptyOutputBuf.Len())
	wg.Wait()
	logger.Log.Debugf("[Exec %s] PTY 输出 goroutine 已完成. ptyOutputBuf.Len() after wait: %d", hostAddr, ptyOutputBuf.Len())

	// Create a new slice with a copy of the data to ensure no further modifications
	// to ptyOutputBuf affect the returned stdout, and to break any aliasing if ptyOutputBuf
	// were to be reused (though it's not in this function).
	// This is a defensive measure if length inconsistencies persist.
	finalOutputBytes := make([]byte, ptyOutputBuf.Len())
	copy(finalOutputBytes, ptyOutputBuf.Bytes())
	stdout = finalOutputBytes
	// stdout = ptyOutputBuf.Bytes() // Original way

	logger.Log.Debugf("[Exec %s] Final len(stdout) being returned: %d", hostAddr, len(stdout))

	if waitErr != nil {
		if sshExitErr, ok := errors.Cause(waitErr).(*ssh.ExitError); ok {
			exitCode = sshExitErr.ExitStatus()
			err = sshExitErr
		} else {
			// Could be context cancellation error from sess.Wait() if cmdCtx was cancelled
			exitCode = -1 // Indicate command did not complete with a status from itself
			err = errors.Wrapf(waitErr, "等待命令 '%s' 完成失败 (非ExitError)", cmd)
		}
		// logger.Log.Debugf("[Exec %s] 命令出错/非零退出. ExitCode: %d, Err: %v. OutputLen: %d", hostAddr, exitCode, err, len(stdout))
		return stdout, stderr, exitCode, err
	}

	exitCode = 0
	// logger.Log.Debugf("[Exec %s] 命令成功执行. ExitCode: 0. OutputLen: %d", hostAddr, len(stdout))
	return stdout, stderr, exitCode, nil
}

func (c *connection) PExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (exitCode int, err error) {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[PExec %s] Cmd: %s. (PTY enabled, passed stderr writer will likely receive no data due to PTY merge)", hostAddr, cmd)

	if stdout == nil {
		stdout = io.Discard
		logger.Log.Debugf("[PExec %s] stdout writer was nil, using io.Discard.", hostAddr)
	}
	if stderr == nil {
		stderr = io.Discard
		logger.Log.Debugf("[PExec %s] stderr writer was nil, using io.Discard.", hostAddr)
	} else {
		logger.Log.Warnf("[PExec %s] PTY is active; the provided stderr writer might not receive command's stderr as it's merged into stdout by PTY.", hostAddr)
	}

	cmdCtx, cancelCmdCtx := context.WithCancel(ctx)
	defer cancelCmdCtx()

	sess, sessionLifecycleDone, errSession := c.createSession(cmdCtx)
	if errSession != nil {
		return -1, errors.Wrap(errSession, "PExec: 准备命令执行失败")
	}
	defer func() {
		if sessionLifecycleDone != nil {
			close(sessionLifecycleDone)
		}
		sess.Close()
		logger.Log.Debugf("[PExec %s] 会话已关闭 (cmd: %s)", hostAddr, cmd)
	}()

	sess.Stderr = stderr // Set it, but with PTY, data likely won't go here from command's stderr.

	var internalStdinPipe io.WriteCloser
	var callerStdinToUse io.Reader = stdin // Use this for the copying goroutine if internalStdinPipe is active

	if c.config.Password != "" {
		pipe, pipeErr := sess.StdinPipe()
		if pipeErr != nil {
			return -1, errors.Wrap(pipeErr, "PExec: 获取内部 stdin pipe (for password) 失败")
		}
		internalStdinPipe = pipe
		sess.Stdin = nil // We are managing stdin via internalStdinPipe
		logger.Log.Debugf("[PExec %s] 使用内部 stdin 进行密码注入.", hostAddr)
	} else {
		sess.Stdin = stdin // Use caller's stdin directly
		logger.Log.Debugf("[PExec %s] 使用调用者提供的 stdin (if configured).", hostAddr)
	}

	internalPtyPipeReader, internalPtyPipeWriter := io.Pipe()
	sess.Stdout = internalPtyPipeWriter // PTY output goes here

	var wg sync.WaitGroup
	var passwordSentLock sync.Mutex
	passwordSuccessfullySent := false
	sudoPrefixPExec := fmt.Sprintf("[sudo] password for %s", c.config.Username)
	passwordSuffixPExec := ": "

	ioGoroutineCtxP, cancelIOGoroutineCtxP := context.WithCancel(cmdCtx)
	defer cancelIOGoroutineCtxP()

	wg.Add(1)
	go func(goroutineCtx context.Context, ptyPipeReader io.Reader, callerStdoutWriter io.Writer, stdinForPasswordInjection io.WriteCloser, originalStdinForCopying io.Reader) {
		defer wg.Done()
		reader := bufio.NewReaderSize(ptyPipeReader, 32*1024)
		logger.Log.Debugf("[PExec-PtyOutput %s] Goroutine 已启动 (reading merged PTY output to caller's stdout writer)", hostAddr)
		var currentLine string
		var stdinCopyWg sync.WaitGroup // To wait for stdin copy if it's started

		for {
			select {
			case <-goroutineCtx.Done():
				logger.Log.Debugf("[PExec-PtyOutput %s] Goroutine context 已取消, 正在退出: %v", hostAddr, goroutineCtx.Err())
				stdinCopyWg.Wait() // Ensure any stdin copying finishes before this goroutine fully exits
				return
			default:
			}

			b, readErr := reader.ReadByte()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					logger.Log.Debugf("[PExec-PtyOutput %s] EOF reached on PTY pipeReader.", hostAddr)
				} else if goroutineCtx.Err() == nil {
					logger.Log.Warnf("[PExec-PtyOutput %s] 读取 PTY pipeReader 错误: %v", hostAddr, readErr)
				} else {
					logger.Log.Debugf("[PExec-PtyOutput %s] 读取 PTY pipeReader 错误 (likely due to context %v): %v", hostAddr, goroutineCtx.Err(), readErr)
				}
				stdinCopyWg.Wait()
				break // Exit loop
			}

			if _, errWrite := callerStdoutWriter.Write([]byte{b}); errWrite != nil {
				if goroutineCtx.Err() == nil { // Avoid logging write error if context is already cancelled
					logger.Log.Warnf("[PExec-PtyOutput %s] 写入调用者 stdout 失败: %v. Aborting PTY read.", hostAddr, errWrite)
					cancelIOGoroutineCtxP() // Signal to stop everything if we can't write output
				}
				stdinCopyWg.Wait()
				return // Stop this goroutine
			}

			if b == '\n' {
				currentLine = ""
			} else {
				currentLine += string(b)
				const maxPromptLineLength = 256
				if len(currentLine) > maxPromptLineLength {
					currentLine = currentLine[len(currentLine)-maxPromptLineLength:]
				}
			}

			passwordSentLock.Lock()
			if c.config.Password != "" && !passwordSuccessfullySent && stdinForPasswordInjection != nil {
				if (strings.HasPrefix(currentLine, sudoPrefixPExec) || strings.HasPrefix(currentLine, "Password")) && strings.HasSuffix(currentLine, passwordSuffixPExec) {
					logger.Log.Debugf("[PExec-PtyOutput %s] 检测到密码提示: '%s', 尝试写入密码...", hostAddr, currentLine)
					_, pwWriteErr := stdinForPasswordInjection.Write([]byte(c.config.Password + "\n"))
					if pwWriteErr != nil {
						if goroutineCtx.Err() == nil && !util.IsErrPipeClosed(pwWriteErr) {
							logger.Log.Errorf("[PExec-PtyOutput %s] 写入 sudo 密码失败: %v", hostAddr, pwWriteErr)
						}
					} else {
						logger.Log.Debugf("[PExec-PtyOutput %s] Sudo 密码已发送.", hostAddr)
					}
					passwordSuccessfullySent = true
					currentLine = ""

					if originalStdinForCopying != nil { // If caller provided stdin, copy it after password
						logger.Log.Debugf("[PExec-PtyOutput %s] 密码已发送, 现在开始复制调用者的 stdin.", hostAddr)
						stdinCopyWg.Add(1)
						go func(dst io.WriteCloser, src io.Reader) {
							defer stdinCopyWg.Done()
							defer func() { // Ensure dst (stdinForPasswordInjection pipe) is closed after copy
								if errClose := dst.Close(); errClose != nil && !util.IsErrPipeClosed(errClose) {
									logger.Log.Warnf("[PExec-StdinCopier %s] 关闭注入用 stdin pipe (复制后) 出错: %v", hostAddr, errClose)
								} else {
									logger.Log.Debugf("[PExec-StdinCopier %s] 注入用 stdin pipe (复制后) 已关闭或早已关闭.", hostAddr)
								}
							}()
							logger.Log.Debugf("[PExec-StdinCopier %s] 开始复制...", hostAddr)
							copiedBytes, errCopy := io.Copy(dst, src)
							if errCopy != nil && !errors.Is(errCopy, io.EOF) && !util.IsErrPipeClosed(errCopy) {
								if goroutineCtx.Err() == nil { // Don't log error if parent context is already done
									logger.Log.Warnf("[PExec-StdinCopier %s] 复制调用者 stdin 时出错: %v (copied %d bytes)", hostAddr, errCopy, copiedBytes)
								}
							} else {
								logger.Log.Debugf("[PExec-StdinCopier %s] 调用者 stdin 复制完成 (copied %d bytes). Error: %v", hostAddr, copiedBytes, errCopy)
							}
						}(stdinForPasswordInjection, originalStdinForCopying)
					} else { // No original stdin to copy, just close the pipe we used for password
						if errCloseStdin := stdinForPasswordInjection.Close(); errCloseStdin != nil && !util.IsErrPipeClosed(errCloseStdin) {
							logger.Log.Warnf("[PExec-PtyOutput %s] 发送密码后关闭注入用 stdin (无后续复制) 出错: %v", hostAddr, errCloseStdin)
						} else {
							logger.Log.Debugf("[PExec-PtyOutput %s] 发送密码后注入用 stdin (无后续复制) 已关闭.", hostAddr)
						}
					}
				}
			}
			passwordSentLock.Unlock()
		}
		stdinCopyWg.Wait() // Wait for any pending stdin copy to complete
		logger.Log.Debugf("[PExec-PtyOutput %s] Goroutine loop finished.", hostAddr)
	}(ioGoroutineCtxP, internalPtyPipeReader, stdout, internalStdinPipe, callerStdinToUse)

	logger.Log.Debugf("[PExec %s] 即将启动命令: %s", hostAddr, cmd)
	if err = sess.Start(cmd); err != nil {
		if internalStdinPipe != nil {
			_ = internalStdinPipe.Close()
		}
		_ = internalPtyPipeWriter.Close() // Close writer if Start fails
		cancelIOGoroutineCtxP()           // Signal I/O goroutine to stop
		wg.Wait()                         // Wait for I/O goroutine
		exitCode = -1
		if sshExitErr, ok := errors.Cause(err).(*ssh.ExitError); ok {
			exitCode = sshExitErr.ExitStatus()
		}
		return exitCode, errors.Wrapf(err, "PExec: 启动命令 '%s' 失败", cmd)
	}
	logger.Log.Debugf("[PExec %s] 命令已启动.", hostAddr)

	passwordSentLock.Lock()
	if c.config.Password == "" && internalStdinPipe != nil && !passwordSuccessfullySent {
		// This case should not happen if PExec logic for internalStdinPipe is correct (only created if password exists)
		// but as a safeguard. Or if originalStdinForCopying was nil and password was also nil.
		logger.Log.Debugf("[PExec %s] 未配置密码, 但 internalStdinPipe 存在且未用于发送密码, 将其关闭.", hostAddr)
		if errClose := internalStdinPipe.Close(); errClose != nil && !util.IsErrPipeClosed(errClose) {
			logger.Log.Warnf("[PExec %s] 关闭 internalStdinPipe (无密码时) 出错: %v", hostAddr, errClose)
		}
		passwordSuccessfullySent = true
	} else if c.config.Password == "" && callerStdinToUse != nil && internalStdinPipe == nil {
		// If no password and caller provided stdin, it's directly connected via sess.Stdin.
		// If caller's stdin is an io.Closer (e.g. os.File), it's caller's responsibility to close it.
		// If it's something like bytes.Reader, closing is a no-op.
		// We generally don't close caller's stdin from here unless we piped it.
	}
	passwordSentLock.Unlock()

	logger.Log.Debugf("[PExec %s] 等待命令完成 (sess.Wait())...", hostAddr)
	waitErr := sess.Wait()
	logger.Log.Debugf("[PExec %s] sess.Wait() 已完成. Wait 错误: %v", hostAddr, waitErr)

	// Close the writer end of the PTY pipe. This signals EOF to the ptyPipeReader in the goroutine.
	if errClose := internalPtyPipeWriter.Close(); errClose != nil && !util.IsErrPipeClosed(errClose) {
		logger.Log.Warnf("[PExec %s] Wait 后关闭内部 PTY pipe writer 出错: %v", hostAddr, errClose)
	}

	passwordSentLock.Lock()
	// If password was configured, but not sent (no prompt), and no stdin was copied after it (because there was no original stdin to copy)
	// then internalStdinPipe might still be open.
	if c.config.Password != "" && !passwordSuccessfullySent && internalStdinPipe != nil && callerStdinToUse == nil {
		logger.Log.Debugf("[PExec %s] 命令完成, 密码未发送 (无提示?), 且无后续 stdin 复制, 关闭 internalStdinPipe.", hostAddr)
		if errClose := internalStdinPipe.Close(); errClose != nil && !util.IsErrPipeClosed(errClose) {
			logger.Log.Warnf("[PExec %s] Wait 后关闭未用 internalStdinPipe 时出错: %v", hostAddr, errClose)
		}
	}
	passwordSentLock.Unlock()

	// cancelIOGoroutineCtxP() // Can be called here to ensure goroutine stops if EOF wasn't enough,
	// but EOF from internalPtyPipeWriter.Close() should be the primary mechanism.
	// The defer cancelIOGoroutineCtxP() will handle cleanup.

	logger.Log.Debugf("[PExec %s] 等待 PTY 输出 goroutine (wg.Wait())...", hostAddr)
	wg.Wait() // Wait for the PTY output goroutine to finish.
	logger.Log.Debugf("[PExec %s] PTY 输出 goroutine 已完成.", hostAddr)

	if waitErr != nil {
		if sshExitErr, ok := errors.Cause(waitErr).(*ssh.ExitError); ok {
			exitCode = sshExitErr.ExitStatus()
			err = sshExitErr
		} else {
			exitCode = -1
			err = errors.Wrapf(waitErr, "PExec: 等待命令 '%s' 完成失败 (非ExitError)", cmd)
		}
		return exitCode, err
	}

	exitCode = 0
	return exitCode, nil
}

// --- FileOperator Methods ---

// isSftpPermissionDenied 检查错误是否为 SFTP 权限拒绝错误。
func isSftpPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	// sftp.ErrSSHFxPermissionDenied 是一个 *sftp.StatusError 类型的变量实例
	// 它的 Code 字段已设置为相应的 SFTP 协议状态码。
	// 我们可以使用 errors.Is 来直接比较这个实例。
	if errors.Is(err, sftp.ErrSSHFxPermissionDenied) {
		return true
	}
	// 作为备用方案，如果错误是 *sftp.StatusError 但不是 sftp.ErrSSHFxPermissionDenied 的确切实例
	// （例如，被包装过），我们可以检查其 Code 字段。
	var sftpStatusErr *sftp.StatusError
	if errors.As(err, &sftpStatusErr) {
		// sftp.ErrSSHFxPermissionDenied.Code 包含了正确的 SFTP 协议代码
		// uint32(1) is SSH_FX_OK, uint32(2) is SSH_FX_EOF, uint32(3) is SSH_FX_NO_SUCH_FILE, uint32(4) is SSH_FX_PERMISSION_DENIED
		// The sftp package itself defines constants like sftp.ErrSSHFxPermissionDenied which is a *StatusError
		// where Code is already set to the numeric SFTP code for permission denied.
		// So we can compare against its Code field.
		return sftpStatusErr.Code == uint32(sftp.ErrSSHFxPermissionDenied) // Code is uint32, use Status field name as in pkg/sftp
	}
	return false
}

// getTempRemotePath 生成一个远程临时文件路径。
func (c *connection) getTempRemotePath(baseNamePrefix string) string {
	tmpDir := common.GetTmpDir()
	if tmpDir == "" {
		tmpDir = "/tmp"
		logger.Log.Warnf("common.GetTmpDir() 返回空, 临时文件将使用 /tmp 目录")
	}
	fileName := fmt.Sprintf("%s-%s", baseNamePrefix, uuid.New().String())
	return path.Join(tmpDir, fileName)
}

func (c *connection) DownloadFile(ctx context.Context, remotePath string, localPath string) error {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[DownloadFile %s] Remote: %s, Local: %s, UseSudo: %t", hostAddr, remotePath, localPath, c.config.UseSudoForFileOps)

	if !c.config.UseSudoForFileOps {
		c.mu.Lock()
		sftpClient := c.sftpclient
		c.mu.Unlock()
		if sftpClient == nil {
			return errors.New("sftp 客户端未初始化")
		}

		logger.Log.Debugf("[DownloadFile %s] 使用 SFTP 下载", hostAddr)
		srcFile, err := sftpClient.Open(remotePath)
		if err != nil {
			return errors.Wrapf(err, "sftp: 打开远程文件 %s 失败", remotePath)
		}
		defer srcFile.Close()

		if err := os.MkdirAll(filepath.Dir(localPath), os.ModePerm); err != nil {
			return errors.Wrapf(err, "创建本地目录 %s 失败", filepath.Dir(localPath))
		}
		dstFile, err := os.Create(localPath)
		if err != nil {
			return errors.Wrapf(err, "创建本地文件 %s 失败", localPath)
		}
		defer dstFile.Close()

		bytesCopied, err := io.Copy(dstFile, srcFile)
		if err != nil {
			return errors.Wrapf(err, "从远程 %s 复制数据到本地 %s 失败 (已复制 %d 字节)", remotePath, localPath, bytesCopied)
		}
		logger.Log.Debugf("[DownloadFile %s] SFTP: 成功下载 %d 字节到 %s", hostAddr, bytesCopied, localPath)
		return nil
	}

	logger.Log.Infof("[DownloadFile %s] 使用 sudo 和 base64 下载", hostAddr)
	b64Cmd := fmt.Sprintf("cat %s | base64 --wrap=0", remotePath)
	sudoCmd := SudoPrefix(b64Cmd)

	stdoutBytes, stderrBytes, exitC, err := c.Exec(ctx, sudoCmd)
	if err != nil {
		errMsgContent := string(stderrBytes)
		if len(stdoutBytes) > 0 && exitC != 0 {
			errMsgContent += "\nStdout: " + string(stdoutBytes)
		}
		return errors.Wrapf(err, "sudo 下载: 执行 '%s' 失败 (退出码 %d, stderr/stdout: %s)", sudoCmd, exitC, errMsgContent)
	}
	if exitC != 0 {
		return errors.Errorf("sudo 下载: 执行命令 '%s' 失败，退出码 %d (stderr: %s, stdout: %s)", sudoCmd, exitC, string(stderrBytes), string(stdoutBytes))
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(stdoutBytes)))
	if err != nil {
		return errors.Wrapf(err, "sudo 下载: base64 解码来自 %s 的内容失败", remotePath)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), os.ModePerm); err != nil {
		return errors.Wrapf(err, "sudo 下载: 创建本地目录 %s 失败", filepath.Dir(localPath))
	}
	err = os.WriteFile(localPath, decodedBytes, 0644)
	if err != nil {
		return errors.Wrapf(err, "sudo 下载: 将解码后的内容写入本地文件 %s 失败", localPath)
	}
	logger.Log.Infof("[DownloadFile %s] Sudo: 成功下载 %s 并写入到 %s (大小: %d bytes)", hostAddr, remotePath, localPath, len(decodedBytes))
	return nil
}

func (c *connection) UploadFile(ctx context.Context, localPath string, remotePath string) error {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[UploadFile %s] Local: %s, Remote: %s, UseSudo: %t", hostAddr, localPath, remotePath, c.config.UseSudoForFileOps)

	if !c.config.UseSudoForFileOps {
		c.mu.Lock()
		sftpClient := c.sftpclient
		c.mu.Unlock()
		if sftpClient == nil {
			return errors.New("sftp 客户端未初始化")
		}
		logger.Log.Debugf("[UploadFile %s] 使用 SFTP 上传", hostAddr)

		srcFile, err := os.Open(localPath)
		if err != nil {
			return errors.Wrapf(err, "打开本地文件 %s 失败", localPath)
		}
		defer srcFile.Close()

		srcStat, err := srcFile.Stat()
		if err != nil {
			return errors.Wrapf(err, "获取本地文件 %s 状态失败", localPath)
		}

		remoteDir := path.Dir(remotePath)
		if err := sftpClient.MkdirAll(remoteDir); err != nil {
			statInfo, statErr := sftpClient.Stat(remoteDir)
			if statErr != nil || !statInfo.IsDir() {
				return errors.Wrapf(err, "sftp: 创建远程目录 %s 失败 (stat 也失败或不是目录: %v)", remoteDir, statErr)
			}
			logger.Log.Debugf("sftp MkdirAll 对 %s 报错 (%v), 但目录已存在; 继续执行。", remoteDir, err)
		}

		dstFile, err := sftpClient.Create(remotePath)
		if err != nil {
			return errors.Wrapf(err, "sftp: 创建远程文件 %s 失败", remotePath)
		}

		var closeErrDst error
		defer func() {
			if e := dstFile.Close(); e != nil && closeErrDst == nil {
				closeErrDst = errors.Wrapf(e, "sftp: 关闭远程文件 %s 失败", remotePath)
			}
		}()

		if err := dstFile.Chmod(srcStat.Mode().Perm()); err != nil {
			logger.Log.Warnf("sftp: 设置远程文件 %s 权限 (mode %s) 失败: %v", remotePath, srcStat.Mode().Perm(), err)
		}

		var localMd5 string
		if md5Str, md5Err := file.LocalMd5Sum(localPath); md5Err != nil {
			logger.Log.Warnf("计算本地文件 %s 的 MD5 失败: %v. 将跳过 MD5 校验。", localPath, md5Err)
		} else if md5Str != "" {
			localMd5 = md5Str
			logger.Log.Debugf("本地文件 %s 的 MD5: %s", localPath, localMd5)
		}

		bytesCopied, err := io.Copy(dstFile, srcFile)
		if err != nil {
			return errors.Wrapf(err, "sftp: 从本地 %s 复制数据到远程 %s 失败 (已复制 %d 字节)", localPath, remotePath, bytesCopied)
		}
		if closeErrDst != nil {
			return closeErrDst
		}

		if localMd5 != "" {
			md5Cmd := fmt.Sprintf("md5sum %s", remotePath)
			remoteMd5Bytes, remoteMd5Stderr, exitC, execE := c.Exec(ctx, md5Cmd)
			if execE == nil && exitC == 0 {
				outputParts := strings.Fields(string(remoteMd5Bytes))
				if len(outputParts) > 0 {
					remoteMd5 := outputParts[0]
					if localMd5 != remoteMd5 {
						return errors.Errorf("MD5 校验和不匹配 %s: 本地 (%s) != 远程 (%s)", remotePath, localMd5, remoteMd5)
					}
					logger.Log.Infof("文件 %s 的 MD5 校验和已验证", remotePath)
				} else {
					logger.Log.Warnf("无法解析远程文件 %s 的 MD5 输出: %s", remotePath, string(remoteMd5Bytes))
				}
			} else {
				logger.Log.Warnf("获取远程文件 %s 的 MD5 失败 (退出码: %d, 错误: %v, stderr: %s)", remotePath, exitC, execE, string(remoteMd5Stderr))
			}
		}
		logger.Log.Debugf("[UploadFile %s] SFTP: 成功上传 %d 字节到 %s", hostAddr, bytesCopied, remotePath)
		return nil
	}

	logger.Log.Infof("[UploadFile %s] 使用 sudo 上传 (先 SFTP 到临时位置, 然后 sudo mv)", hostAddr)
	srcFileToUpload, errOpen := os.Open(localPath)
	if errOpen != nil {
		return errors.Wrapf(errOpen, "sudo 上传: 打开本地文件 %s 失败", localPath)
	}
	defer srcFileToUpload.Close()

	srcStat, errStat := srcFileToUpload.Stat()
	if errStat != nil {
		return errors.Wrapf(errStat, "sudo 上传: 获取本地文件 %s 状态失败", localPath)
	}

	c.mu.Lock()
	sftpClientForTemp := c.sftpclient
	c.mu.Unlock()
	if sftpClientForTemp == nil {
		return errors.New("sftp 客户端未初始化 (用于 sudo 上传的临时阶段)")
	}

	tempRemotePath := c.getTempRemotePath("xm_upload_sudo")
	logger.Log.Debugf("[UploadFile %s] Sudo: 上传到临时路径 %s", hostAddr, tempRemotePath)

	dstTempFile, errCreateTemp := sftpClientForTemp.Create(tempRemotePath)
	if errCreateTemp != nil {
		return errors.Wrapf(errCreateTemp, "sudo 上传: sftp 创建临时远程文件 %s 失败", tempRemotePath)
	}

	bytesCopied, errCopyTemp := io.Copy(dstTempFile, srcFileToUpload)
	errCloseTemp := dstTempFile.Close()

	if errCopyTemp != nil {
		_ = sftpClientForTemp.Remove(tempRemotePath)
		return errors.Wrapf(errCopyTemp, "sudo 上传: sftp 复制到临时远程文件 %s 失败 (已复制 %d 字节)", tempRemotePath, bytesCopied)
	}
	if errCloseTemp != nil {
		_ = sftpClientForTemp.Remove(tempRemotePath)
		return errors.Wrapf(errCloseTemp, "sudo 上传: sftp 关闭临时远程文件 %s 失败", tempRemotePath)
	}
	logger.Log.Debugf("[UploadFile %s] Sudo: 成功通过 sftp 上传 %d 字节到临时文件 %s", hostAddr, bytesCopied, tempRemotePath)

	remoteDir := path.Dir(remotePath)
	mkDirCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
	sudoMkDirCmd := SudoPrefix(mkDirCmd)
	_, stderrBytesMkdir, exitCMkdir, errMkdir := c.Exec(ctx, sudoMkDirCmd)
	if errMkdir != nil {
		_ = sftpClientForTemp.Remove(tempRemotePath)
		return errors.Wrapf(errMkdir, "sudo 上传: 执行 '%s' (mkdir) 失败 (退出码 %d, stderr: %s)", sudoMkDirCmd, exitCMkdir, string(stderrBytesMkdir))
	}
	if exitCMkdir != 0 {
		_ = sftpClientForTemp.Remove(tempRemotePath)
		return errors.Errorf("sudo 上传: 执行命令 '%s' (mkdir) 失败，退出码 %d (stderr: %s)", sudoMkDirCmd, exitCMkdir, string(stderrBytesMkdir))
	}

	modeStr := fmt.Sprintf("%04o", srcStat.Mode().Perm())
	chownCmdPart := ""
	if c.config.UserForSudoFileOps != "" {
		chownCmdPart = fmt.Sprintf(" && chown %s %s", c.config.UserForSudoFileOps, remotePath)
	}
	mvCmd := fmt.Sprintf("mv -f %s %s && chmod %s %s%s", tempRemotePath, remotePath, modeStr, remotePath, chownCmdPart)
	sudoMvCmd := SudoPrefix(mvCmd)

	_, stderrBytesMv, exitCMv, errMv := c.Exec(ctx, sudoMvCmd)

	if exitCMv != 0 || errMv != nil {
		logger.Log.Debugf("[UploadFile %s] Sudo: mv/chmod/chown 命令失败，尝试清理临时文件 %s", hostAddr, tempRemotePath)
		if rmErr := sftpClientForTemp.Remove(tempRemotePath); rmErr != nil {
			logger.Log.Warnf("[UploadFile %s] Sudo: sftp 删除临时文件 %s 失败 (%v)，尝试 sudo rm", hostAddr, tempRemotePath, rmErr)
			_, _, _, rmExecErr := c.Exec(ctx, SudoPrefix(fmt.Sprintf("rm -f %s", tempRemotePath)))
			if rmExecErr != nil {
				logger.Log.Warnf("[UploadFile %s] Sudo: sudo rm 删除临时文件 %s 也失败: %v", hostAddr, tempRemotePath, rmExecErr)
			}
		}
	} else {
		logger.Log.Debugf("[UploadFile %s] Sudo: mv/chmod/chown 成功，临时文件 %s 已被移动/删除。", hostAddr, tempRemotePath)
	}

	if errMv != nil {
		return errors.Wrapf(errMv, "sudo 上传: 执行 '%s' (mv/chmod/chown) 失败 (退出码 %d, stderr: %s)", sudoMvCmd, exitCMv, string(stderrBytesMv))
	}
	if exitCMv != 0 {
		return errors.Errorf("sudo 上传: 执行命令 '%s' (mv/chmod/chown) 失败，退出码 %d (stderr: %s)", sudoMvCmd, exitCMv, string(stderrBytesMv))
	}

	var localMd5Sudo string
	if md5StrSudo, md5ErrSudo := file.LocalMd5Sum(localPath); md5ErrSudo != nil {
		logger.Log.Warnf("sudo 上传后计算本地 MD5 (%s) 失败: %v. 跳过 MD5 校验。", localPath, md5ErrSudo)
	} else if md5StrSudo != "" {
		localMd5Sudo = md5StrSudo
	}

	if localMd5Sudo != "" {
		md5CmdSudo := SudoPrefix(fmt.Sprintf("md5sum %s", remotePath))
		remoteMd5Bytes, remoteMd5Stderr, exitC, execE := c.Exec(ctx, md5CmdSudo)
		if execE == nil && exitC == 0 {
			outputParts := strings.Fields(string(remoteMd5Bytes))
			if len(outputParts) > 0 {
				remoteMd5 := outputParts[0]
				if localMd5Sudo != remoteMd5 {
					return errors.Errorf("sudo 上传后 MD5 校验和不匹配 %s: 本地 (%s) != 远程 (%s)", remotePath, localMd5Sudo, remoteMd5)
				}
				logger.Log.Infof("sudo 上传后文件 %s 的 MD5 校验和已验证", remotePath)
			} else {
				logger.Log.Warnf("sudo 上传后无法解析远程 MD5 输出 (%s): %s", remotePath, string(remoteMd5Bytes))
			}
		} else {
			logger.Log.Warnf("sudo 上传后获取远程 MD5 (%s) 失败 (退出码: %d, 错误: %v, stderr: %s)", remotePath, exitC, execE, string(remoteMd5Stderr))
		}
	}
	logger.Log.Infof("[UploadFile %s] Sudo: 成功上传本地 %s 到远程 %s", hostAddr, localPath, remotePath)
	return nil
}

func (c *connection) Fetch(ctx context.Context, remotePath string) (io.ReadCloser, error) {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[Fetch %s] Remote: %s, UseSudo: %t", hostAddr, remotePath, c.config.UseSudoForFileOps)

	if c.config.UseSudoForFileOps {
		logger.Log.Warnf("[Fetch %s] UseSudoForFileOps=true 时，Fetch 仍将尝试使用 SFTP 进行流式读取。如果需要 sudo 权限且 SFTP 失败，请考虑使用 DownloadFile（它会缓冲整个文件）或自定义 PExec 方案。", hostAddr)
	}

	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient == nil {
		return nil, errors.New("sftp 客户端未初始化")
	}

	logger.Log.Debugf("[Fetch %s] 使用 SFTP 获取文件流", hostAddr)
	file, err := sftpClient.Open(remotePath)
	if err != nil {
		return nil, errors.Wrapf(err, "sftp: 打开远程文件 %s 以进行读取失败", remotePath)
	}
	return file, nil
}

func (c *connection) Scp(ctx context.Context, localReader io.Reader, remotePath string, sizeHint int64, mode os.FileMode) error {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[Scp %s] Remote: %s, Mode: %s, SizeHint: %d, UseSudo: %t", hostAddr, remotePath, mode.String(), sizeHint, c.config.UseSudoForFileOps)

	if localReader == nil {
		return errors.New("Scp: localReader 不能为空")
	}

	if !c.config.UseSudoForFileOps {
		c.mu.Lock()
		sftpClient := c.sftpclient
		c.mu.Unlock()
		if sftpClient == nil {
			return errors.New("sftp 客户端未初始化")
		}
		logger.Log.Debugf("[Scp %s] 使用 SFTP (Create/Write) 实现 Scp", hostAddr)

		remoteDir := path.Dir(remotePath)
		if err := sftpClient.MkdirAll(remoteDir); err != nil {
			statInfo, statErr := sftpClient.Stat(remoteDir)
			if statErr != nil || !statInfo.IsDir() {
				return errors.Wrapf(err, "sftp (scp): 创建远程目录 %s 失败 (stat 也失败或不是目录: %v)", remoteDir, statErr)
			}
			logger.Log.Debugf("sftp MkdirAll 对 %s 报错 (%v), 但目录已存在; 继续执行。", remoteDir, err)
		}

		dstFile, err := sftpClient.Create(remotePath)
		if err != nil {
			return errors.Wrapf(err, "sftp (scp): 创建远程文件 %s 失败", remotePath)
		}

		var closeErrDst error
		defer func() {
			if e := dstFile.Close(); e != nil && closeErrDst == nil {
				closeErrDst = errors.Wrapf(e, "sftp (scp): 关闭远程文件 %s 失败", remotePath)
			}
		}()

		if errChmod := dstFile.Chmod(mode.Perm()); errChmod != nil {
			logger.Log.Warnf("sftp (scp): 设置远程文件 %s 权限 (mode %s) 失败: %v", remotePath, mode.Perm(), errChmod)
		}

		bytesCopied, errCopy := io.Copy(dstFile, localReader)
		if errCopy != nil {
			return errors.Wrapf(errCopy, "sftp (scp): 复制数据到远程 %s 失败 (已复制 %d 字节)", remotePath, bytesCopied)
		}
		if closeErrDst != nil {
			return closeErrDst
		}
		logger.Log.Debugf("[Scp %s] SFTP (scp): 成功传输 %d 字节到 %s", hostAddr, bytesCopied, remotePath)
		return nil
	}

	logger.Log.Infof("[Scp %s] 使用 sudo (PExec tee) 实现 Scp", hostAddr)

	remoteDir := path.Dir(remotePath)
	mkDirCmdSudo := SudoPrefix(fmt.Sprintf("mkdir -p %s", remoteDir))
	_, stderrMkdir, exitCMkdir, errMkdir := c.Exec(ctx, mkDirCmdSudo)
	if errMkdir != nil {
		return errors.Wrapf(errMkdir, "sudo scp: 创建父目录 %s 失败 (执行 '%s' 失败, 退出码 %d, stderr: %s)", remoteDir, mkDirCmdSudo, exitCMkdir, string(stderrMkdir))
	}
	if exitCMkdir != 0 {
		return errors.Errorf("sudo scp: 创建父目录 %s 失败 (执行 '%s' 返回退出码 %d, stderr: %s)", remoteDir, mkDirCmdSudo, exitCMkdir, string(stderrMkdir))
	}

	teeCmd := fmt.Sprintf("tee %s > /dev/null", remotePath)
	sudoTeeCmd := SudoPrefix(teeCmd)

	var pexecStdout, pexecStderr bytes.Buffer
	logger.Log.Debugf("[Scp %s] Sudo: 执行 PExec tee 命令: %s", hostAddr, sudoTeeCmd)
	exitCTee, errTee := c.PExec(ctx, sudoTeeCmd, localReader, &pexecStdout, &pexecStderr)

	if errTee != nil {
		return errors.Wrapf(errTee, "sudo scp: PExec 执行 '%s' (tee) 失败 (退出码 %d, pexec_stderr: %s, pexec_stdout: %s)", sudoTeeCmd, exitCTee, pexecStderr.String(), pexecStdout.String())
	}
	if exitCTee != 0 {
		return errors.Errorf("sudo scp: PExec 命令 '%s' (tee) 失败，退出码 %d (pexec_stderr: %s, pexec_stdout: %s)", sudoTeeCmd, exitCTee, pexecStderr.String(), pexecStdout.String())
	}
	logger.Log.Debugf("[Scp %s] Sudo: PExec tee 命令成功。Piped stdout: '%s', Piped stderr: '%s'", hostAddr, pexecStdout.String(), pexecStderr.String())

	modeStr := fmt.Sprintf("%04o", mode.Perm())
	chownCmdPart := ""
	if c.config.UserForSudoFileOps != "" {
		chownCmdPart = fmt.Sprintf(" && chown %s %s", c.config.UserForSudoFileOps, remotePath)
	}
	chmodCmd := fmt.Sprintf("chmod %s %s%s", modeStr, remotePath, chownCmdPart)
	sudoChmodCmd := SudoPrefix(chmodCmd)

	_, stderrChmod, exitCChmod, errChmod := c.Exec(ctx, sudoChmodCmd)
	if errChmod != nil {
		return errors.Wrapf(errChmod, "sudo scp: 执行 '%s' (chmod/chown) 失败 (退出码 %d, stderr: %s)", sudoChmodCmd, exitCChmod, string(stderrChmod))
	}
	if exitCChmod != 0 {
		return errors.Errorf("sudo scp: 执行命令 '%s' (chmod/chown) 失败，退出码 %d (stderr: %s)", sudoChmodCmd, exitCChmod, string(stderrChmod))
	}

	logger.Log.Infof("[Scp %s] Sudo: 成功通过 PExec tee 将流写入远程 %s 并设置权限/所有者", hostAddr, remotePath)
	return nil
}

func (c *connection) StatRemote(ctx context.Context, remotePath string) (os.FileInfo, error) {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[StatRemote %s] Path: %s, UseSudo: %t", hostAddr, remotePath, c.config.UseSudoForFileOps)

	c.mu.Lock()
	sftpClient := c.sftpclient
	c.mu.Unlock()
	if sftpClient == nil {
		return nil, errors.New("sftp 客户端未初始化")
	}

	stat, err := sftpClient.Stat(remotePath)
	if err != nil {
		if c.config.UseSudoForFileOps && isSftpPermissionDenied(err) {
			logger.Log.Warnf("[StatRemote %s] SFTP Stat 对 %s 操作失败 (权限问题: %v), 且 UseSudoForFileOps=true. 通过 Exec 执行 sudo stat 并解析其输出以获取完整的 os.FileInfo 很复杂且依赖平台，因此当前未实现。将返回原始 SFTP 错误。", hostAddr, remotePath, err)
			return nil, errors.Wrapf(err, "sftp stat 对 %s 权限被拒绝 (完整的 sudo stat 未实现)", remotePath)
		}
		return nil, errors.Wrapf(err, "sftp stat 对 %s 失败", remotePath)
	}
	return stat, nil
}

func (c *connection) RemoteFileExist(ctx context.Context, remotePath string) (bool, error) {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[RemoteFileExist %s] Path: %s, UseSudo: %t", hostAddr, remotePath, c.config.UseSudoForFileOps)

	info, err := c.StatRemote(ctx, remotePath)
	if err == nil {
		return info != nil && !info.IsDir(), nil
	}

	if errors.Is(err, sftp.ErrSSHFxNoSuchFile) {
		return false, nil
	}
	var sftpStatusErr *sftp.StatusError
	if errors.As(err, &sftpStatusErr) {
		if sftpStatusErr.Code == uint32(sftp.ErrSSHFxNoSuchFile) { // 比较 Code 字段
			return false, nil
		}
	}

	if c.config.UseSudoForFileOps && (isSftpPermissionDenied(err) || strings.Contains(err.Error(), "sftp stat 对") && strings.Contains(err.Error(), "权限被拒绝")) {
		logger.Log.Debugf("[RemoteFileExist %s] SFTP 检查文件 %s 失败 (权限问题: %v), 尝试使用 'sudo test -f'", hostAddr, remotePath, err)
		sudoCmd := SudoPrefix(fmt.Sprintf("test -f %s", remotePath))
		_, _, exitC, execErr := c.Exec(ctx, sudoCmd)

		if execErr != nil {
			if _, ok := errors.Cause(execErr).(*ssh.ExitError); ok {
				logger.Log.Debugf("[RemoteFileExist %s] 'sudo test -f %s' 执行完成，退出码: %d", hostAddr, remotePath, exitC)
				return exitC == 0, nil
			}
			return false, errors.Wrapf(execErr, "sudo test -f: 执行 '%s' 失败", sudoCmd)
		}
		return exitC == 0, nil
	}

	return false, err
}

func (c *connection) RemoteDirExist(ctx context.Context, remotePath string) (bool, error) {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[RemoteDirExist %s] Path: %s, UseSudo: %t", hostAddr, remotePath, c.config.UseSudoForFileOps)

	stat, err := c.StatRemote(ctx, remotePath)
	if err == nil {
		return stat.IsDir(), nil
	}

	if errors.Is(err, sftp.ErrSSHFxNoSuchFile) {
		return false, nil
	}
	var sftpStatusErr *sftp.StatusError
	if errors.As(err, &sftpStatusErr) {
		if sftpStatusErr.Code == uint32(sftp.ErrSSHFxNoSuchFile) { // 比较 Code 字段
			return false, nil
		}
	}

	if c.config.UseSudoForFileOps && (isSftpPermissionDenied(err) || strings.Contains(err.Error(), "sftp stat 对") && strings.Contains(err.Error(), "权限被拒绝")) {
		logger.Log.Debugf("[RemoteDirExist %s] SFTP 检查目录 %s 失败 (权限问题: %v), 尝试使用 'sudo test -d'", hostAddr, remotePath, err)
		sudoCmd := SudoPrefix(fmt.Sprintf("test -d %s", remotePath))
		_, _, exitC, execErr := c.Exec(ctx, sudoCmd)
		if execErr != nil {
			if _, ok := errors.Cause(execErr).(*ssh.ExitError); ok {
				logger.Log.Debugf("[RemoteDirExist %s] 'sudo test -d %s' 执行完成，退出码: %d", hostAddr, remotePath, exitC)
				return exitC == 0, nil
			}
			return false, errors.Wrapf(execErr, "sudo test -d: 执行 '%s' 失败", sudoCmd)
		}
		return exitC == 0, nil
	}
	return false, err
}

func (c *connection) MkDirAll(ctx context.Context, remotePath string, mode os.FileMode) error {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[MkDirAll %s] Path: %s, Mode: %s, UseSudo: %t", hostAddr, remotePath, mode.String(), c.config.UseSudoForFileOps)

	if !c.config.UseSudoForFileOps {
		c.mu.Lock()
		sftpClient := c.sftpclient
		c.mu.Unlock()
		if sftpClient == nil {
			return errors.New("sftp 客户端未初始化")
		}
		logger.Log.Debugf("[MkDirAll %s] 使用 SFTP MkdirAll", hostAddr)

		err := sftpClient.MkdirAll(remotePath)
		if err != nil {
			statInfo, statErr := sftpClient.Stat(remotePath)
			if statErr == nil && statInfo.IsDir() {
				logger.Log.Debugf("SFTP MkdirAll 对 %s 报错 (%v), 但目录已存在。继续设置权限。", remotePath, err)
			} else {
				return errors.Wrapf(err, "sftp: MkdirAll %s 失败 (stat 也失败或不是目录: %v)", remotePath, statErr)
			}
		}

		if errChmod := sftpClient.Chmod(remotePath, mode.Perm()); errChmod != nil {
			return errors.Wrapf(errChmod, "sftp: Chmod %s 到 %s 失败 (在 MkDirAll 之后)", remotePath, mode.Perm())
		}
		if common.GetTmpDir() != "" && strings.HasPrefix(path.Clean(remotePath), path.Clean(common.GetTmpDir())) {
			logger.Log.Debugf("路径 %s 位于 common.TmpDir (%s) 内。目录已通过 SFTP 创建/设置权限。", remotePath, common.GetTmpDir())
		}
		logger.Log.Debugf("[MkDirAll %s] SFTP: 远程目录 %s 已创建/权限已设置。", hostAddr, remotePath)
		return nil
	}

	logger.Log.Infof("[MkDirAll %s] 使用 sudo mkdir -p", hostAddr)
	modeStr := fmt.Sprintf("%04o", mode.Perm())
	chownCmdPart := ""
	if c.config.UserForSudoFileOps != "" {
		chownCmdPart = fmt.Sprintf(" && chown %s %s", c.config.UserForSudoFileOps, remotePath)
	}

	mkCmd := fmt.Sprintf("mkdir -p -m %s %s && chmod %s %s%s", modeStr, remotePath, modeStr, remotePath, chownCmdPart)
	sudoMkCmd := SudoPrefix(mkCmd)
	_, stderrBytes, exitC, err := c.Exec(ctx, sudoMkCmd)
	if err != nil {
		return errors.Wrapf(err, "sudo mkdir: 执行 '%s' 失败 (退出码 %d, stderr: %s)", sudoMkCmd, exitC, string(stderrBytes))
	}
	if exitC != 0 {
		return errors.Errorf("sudo mkdir: 执行命令 '%s' 失败，退出码 %d (stderr: %s)", sudoMkCmd, exitC, string(stderrBytes))
	}
	logger.Log.Infof("[MkDirAll %s] Sudo: 成功创建/设置目录 %s 的权限和所有者", hostAddr, remotePath)
	return nil
}

func (c *connection) Chmod(ctx context.Context, remotePath string, mode os.FileMode) error {
	hostAddr := fmt.Sprintf("%s:%d", c.config.Address, c.config.Port)
	logger.Log.Debugf("[Chmod %s] Path: %s, Mode: %s, UseSudo: %t", hostAddr, remotePath, mode.String(), c.config.UseSudoForFileOps)

	if !c.config.UseSudoForFileOps {
		c.mu.Lock()
		sftpClient := c.sftpclient
		c.mu.Unlock()
		if sftpClient == nil {
			return errors.New("sftp 客户端未初始化")
		}
		logger.Log.Debugf("[Chmod %s] 使用 SFTP Chmod", hostAddr)
		err := sftpClient.Chmod(remotePath, mode.Perm())
		if err != nil {
			return errors.Wrapf(err, "sftp: Chmod %s 到 %s 失败", remotePath, mode.Perm())
		}
		logger.Log.Debugf("[Chmod %s] SFTP: 成功更改 %s 的权限为 %s", hostAddr, remotePath, mode.Perm())
		return nil
	}

	logger.Log.Infof("[Chmod %s] 使用 sudo chmod", hostAddr)
	modeStr := fmt.Sprintf("%04o", mode.Perm())
	chmodCmd := fmt.Sprintf("chmod %s %s", modeStr, remotePath)
	sudoChmodCmd := SudoPrefix(chmodCmd)

	_, stderrBytes, exitC, err := c.Exec(ctx, sudoChmodCmd)
	if err != nil {
		return errors.Wrapf(err, "sudo chmod: 执行 '%s' 失败 (退出码 %d, stderr: %s)", sudoChmodCmd, exitC, string(stderrBytes))
	}
	if exitC != 0 {
		return errors.Errorf("sudo chmod: 执行命令 '%s' 失败，退出码 %d (stderr: %s)", sudoChmodCmd, exitC, string(stderrBytes))
	}
	logger.Log.Infof("[Chmod %s] Sudo: 成功更改 %s 的权限为 %s", hostAddr, remotePath, mode.String())
	return nil
}
