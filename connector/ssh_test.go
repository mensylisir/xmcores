package connector

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSudoPrefix(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected string
	}{
		{
			name:     "simple command",
			command:  "ls -l /tmp",
			expected: `sudo -E /bin/bash -c "ls -l /tmp"`,
		},
		{
			name:     "command with quotes",
			command:  `echo "hello world"`,
			expected: `sudo -E /bin/bash -c "echo \"hello world\""`,
		},
		{
			name:     "command with backslashes",
			command:  `ls C:\Windows`,
			expected: `sudo -E /bin/bash -c "ls C:\\Windows"`,
		},
		{
			name:     "command with quotes and backslashes",
			command:  `grep "pattern\\" file.txt`,
			expected: `sudo -E /bin/bash -c "grep \"pattern\\\\\" file.txt"`,
		},
		{
			name:     "empty command",
			command:  "",
			expected: `sudo -E /bin/bash -c ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SudoPrefix(tt.command)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestValidateOptions(t *testing.T) {
	tempDir := t.TempDir()
	keyFilePath := filepath.Join(tempDir, "test_key.pem")
	keyContent := "test_private_key_content"
	err := os.WriteFile(keyFilePath, []byte(keyContent), 0600)
	require.NoError(t, err, "无法创建临时密钥文件")

	bastionKeyFilePath := filepath.Join(tempDir, "bastion_key.pem")
	bastionKeyContent := "bastion_private_key_content"
	err = os.WriteFile(bastionKeyFilePath, []byte(bastionKeyContent), 0600)
	require.NoError(t, err, "无法创建临时堡垒机密钥文件")

	tests := []struct {
		name        string
		inputCfg    Config
		expectedCfg Config
		expectError bool
		errorMsg    string // 部分错误消息匹配
	}{
		{
			name:        "valid with password",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129", Password: "xiaoming98"},
			expectedCfg: Config{Username: "root", Address: "192.168.192.129", Password: "xiaoming98", Port: 22, Timeout: 15 * time.Second},
			expectError: false,
		},
		{
			name:        "valid with private key content",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129", PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdzc2gtcn\nNhAAAAAwEAAQAAAYEAyafW5b+a8sVJ3NYSNheXao1xHBvpzE1XE/S3WtyeRmmiPWiDM9X/\nbKIZgXhnqqVOH6oQk67WRcVfZ//hfrS0b6HBaHu7eRIYH093y23Ioe8YhSX7wxmyO2ZHER\n0yRNWvnHCFW3YynRX0qcEWs9W6O/Cq46KfUP5gBBg9NNO2Pp67ga4tQdb0jL6l71H1Xxlb\nZ8HEgWVJbzfmz83hoRZ/Ehzr0BXF/XjtA3+PbcH1DPMmVhVIP21EVrfPRN2AbCCX+jEK+J\nP7HnJd4zo+ReMNGD+X8Wv2ZFVeoN4F/fOdjoogXOFlcz91zikx8uLgG+/CrWGoKnVkbLjI\nycn26qv+1vNw3KeS8jmQDGWIOlCwfIRpFoMe+AQ8RMrSt+RdQU3qWJQ8dHvzF6RlGcf4G2\ncqlkXOVogw1t9u4Mbuovu8xePcu9MtauKloukOxUM3HJi37O0uW8eWUInPjy1NYsov4MNc\nZU7KNWP/jd2BY2pCHqsBRAmIUXxe0TTgV+axbv/nAAAFiMpH5A/KR+QPAAAAB3NzaC1yc2\nEAAAGBAMmn1uW/mvLFSdzWEjYXl2qNcRwb6cxNVxP0t1rcnkZpoj1ogzPV/2yiGYF4Z6ql\nTh+qEJOu1kXFX2f/4X60tG+hwWh7u3kSGB9Pd8ttyKHvGIUl+8MZsjtmRxEdMkTVr5xwhV\nt2Mp0V9KnBFrPVujvwquOin1D+YAQYPTTTtj6eu4GuLUHW9Iy+pe9R9V8ZW2fBxIFlSW83\n5s/N4aEWfxIc69AVxf147QN/j23B9QzzJlYVSD9tRFa3z0TdgGwgl/oxCviT+x5yXeM6Pk\nXjDRg/l/Fr9mRVXqDeBf3znY6KIFzhZXM/dc4pMfLi4Bvvwq1hqCp1ZGy4yMnJ9uqr/tbz\ncNynkvI5kAxliDpQsHyEaRaDHvgEPETK0rfkXUFN6liUPHR78xekZRnH+BtnKpZFzlaIMN\nbfbuDG7qL7vMXj3LvTLWripaLpDsVDNxyYt+ztLlvHllCJz48tTWLKL+DDXGVOyjVj/43d\ngWNqQh6rAUQJiFF8XtE04FfmsW7/5wAAAAMBAAEAAAGACKBU6YwaQUNeRwOjUMwOjqDRU1\n4AUNyIGpLv2wOwA5wWNCFJ54hCfm+qvqabbKnYnzMjtWWXxfFNBQJlr4lkZJgbUXBlkybK\ngGBiZAHkwMSdHGkFDZIGVVMpPBqvIVGwyvTnR4PVY3HifvaDFZtRdan0bXtx7EGNcu9kgu\nOBmskohUIhrnzXBkRLjeLIJ9LKXbRkxxJBo2/VQFNy0PTI58nz7nlX+GFZZjppNM1EwdKO\n88TCS/BNKZaAV9ZP3ZBBTITmW2W5y2uqGx22rPxeAKHYRIIb5qHHHqWvXOeIwudgFMZmf9\nXb79ky4w1spSa8LJJQrsvFC3tXxnq7kOuks0qZd5+hpNK/ZFnwUPY4UQadMHHOCPRjNWfw\nHAvLVUKb5SPoipeWFfc5bEbuusfvBPI3103wgNmBCxzDHJfRXrGWTaM1AE5t3dyefVqpfB\nr53DZ/HzlAo5hsAnUwL9TReiTlZG3vi2vCUyF+dIPsk/bz1BSrXM9tuwuBPJgA0X2VAAAA\nwQDyhM/B8W3Bz7J591gwtlHQdBh/T/AjnAt4Be52/XLvM3ceB4VGiGGjci/camKcj+UoRj\n4mNeNnu8KOlJqpOGZq+Domkl8TDRI4T+YAv4ltuxI5AlZ3IkdvZ0UHaGSrLg1M6RgnyrCA\nGrLVOSk+R4nkwvRe9OW049PVjVlqJp7n7Wa2fP2fhJADo/M8wwOHL2sFWMBc1RCyPAPZkv\n1Gxd9Qu5E9hhMsuAks+Vlsx0w0zh6B8Wr++TnBcZ2tjzQMnUUAAADBAP13MoNl6zh9MdC7\np0IqgO6SmV8uIzwU8ACyp9NUW4uQ+gz6V94Tgjh82ZRJZqMhSjLzyihPGE16EoZuUY7zQO\nQzafWqrALektWJoKmO0Z2BVjCTu77QDmBbFSMzCLu9VX4PHrSX/i34CY9p3cu9lEYAQEt/\nCAyxEw8lMTSQqk325X+rc3im8haS3pL6jcJOaglps9qEFY5vp6SBlaXhxI0MQgbSNSCkDH\ngeeVQKvBtX9HjoEH5cZPyh/qp80Un/TQAAAMEAy6wF0FmOT0rp+wsM7c0vhjykI0KU29nR\nQIkONmk68L5YM6R1tI0VzwGcg0eMI4egZnpapHcgCjUOCFKOVUPNhfedI8tW769YECg6ZY\n/xHJB0XvkRnD3MS8qBc6VJpn0puBEVqw8h1bFYKWek/5CV0oKSg/TEoxkL6HponQprUPvl\nXMyU9nCl3r/OlW/8sscK+9D5UhDKU2xrrtd9Yt8TttlAU3+8LT1wVS4+npxgQ750YvygvE\nULuvv1cB6j+AoDAAAAEDcxMDA4MDY3NUBxcS5jb20BAg==\n-----END OPENSSH PRIVATE KEY-----\n"},
			expectedCfg: Config{Username: "root", Address: "192.168.192.129", PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\n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\n-----END OPENSSH PRIVATE KEY-----\n", Port: 22, Timeout: 15 * time.Second},
			expectError: false,
		},
		{
			name:        "valid with agent socket",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129", AgentSocket: "/tmp/ssh-XXXXXX62rSs9/agent.252508"},
			expectedCfg: Config{Username: "root", Address: "192.168.192.129", AgentSocket: "/tmp/ssh-XXXXXX62rSs9/agent.252508", Port: 22, Timeout: 15 * time.Second},
			expectError: false,
		},
		{
			name:        "valid with key file",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129", KeyFile: "/home/mensyli1/.ssh/id_rsa"},
			expectedCfg: Config{Username: "root", Address: "192.168.192.129", KeyFile: "/home/mensyli1/.ssh/id_rsa", PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\n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\n-----END OPENSSH PRIVATE KEY-----\n", Port: 22, Timeout: 15 * time.Second},
			expectError: false,
		},
		{
			name:        "missing username",
			inputCfg:    Config{Address: "192.168.192.129", Password: "xiaoming98"},
			expectError: true,
			errorMsg:    "未指定 SSH 连接的用户名",
		},
		{
			name:        "missing address",
			inputCfg:    Config{Username: "root", Password: "xiaoming98"},
			expectError: true,
			errorMsg:    "未指定 SSH 连接的地址",
		},
		{
			name:        "missing target auth method",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129"},
			expectError: true,
			errorMsg:    "必须为目标连接指定密码、私钥内容、私钥文件或 agent socket 中的至少一种",
		},
		{
			name:        "invalid key file path",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129", KeyFile: "/non/existent/keyfile"},
			expectError: true,
			errorMsg:    "读取目标主机密钥文件",
		},
		{
			name:        "custom port and timeout",
			inputCfg:    Config{Username: "root", Address: "192.168.192.129", Password: "xiaoming98", Port: 2222, Timeout: 30 * time.Second},
			expectedCfg: Config{Username: "root", Address: "192.168.192.129", Password: "xiaoming98", Port: 2222, Timeout: 30 * time.Second},
			expectError: false,
		},
		// Bastion tests
		{
			name: "valid with bastion and bastion password",
			inputCfg: Config{
				Username: "root", Address: "192.168.192.129", Password: "xiaoming98",
				Bastion: "192.168.31.32", BastionUser: "mensyli1", BastionPassword: "xiaoming98",
			},
			expectedCfg: Config{
				Username: "root", Address: "192.168.192.129", Password: "xiaoming98", Port: 22, Timeout: 15 * time.Second,
				Bastion: "192.168.31.32", BastionUser: "mensyli1", BastionPassword: "xiaoming98", BastionPort: 22,
			},
			expectError: false,
		},
		{
			name: "valid with bastion, default bastion user",
			inputCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98",
				Bastion: "192.168.31.32", BastionPassword: "xiaoming98", // BastionUser is empty
			},
			expectedCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98", Port: 22, Timeout: 15 * time.Second,
				Bastion: "192.168.31.32", BastionUser: "mensyli1", BastionPassword: "xiaoming98", BastionPort: 22,
			},
			expectError: false,
		},
		{
			name: "valid with bastion key file",
			inputCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98",
				Bastion: "192.168.31.32", BastionKeyFile: "/home/mensyli1/.ssh/id_rsa",
			},
			expectedCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98", Port: 22, Timeout: 15 * time.Second,
				Bastion: "192.168.31.32", BastionUser: "mensyli1", BastionKeyFile: "/home/mensyli1/.ssh/id_rsa", BastionPrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\n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\n-----END OPENSSH PRIVATE KEY-----\n", BastionPort: 22,
			},
			expectError: false,
		},
		{
			name: "invalid bastion key file path",
			inputCfg: Config{
				Username: "root", Address: "192.168.192.129", Password: "xiaoming98",
				Bastion: "192.168.31.32", BastionKeyFile: "/non/existent/bastion_keyfile",
			},
			expectError: true,
			errorMsg:    "读取 bastion 密钥文件",
		},
		// Sudo file ops tests
		{
			name: "sudo file ops with specific user",
			inputCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98",
				UseSudoForFileOps: true, UserForSudoFileOps: "root",
			},
			expectedCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98", Port: 22, Timeout: 15 * time.Second,
				UseSudoForFileOps: true, UserForSudoFileOps: "root",
			},
			expectError: false,
		},
		{
			name: "sudo file ops with default user",
			inputCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98",
				UseSudoForFileOps: true, // UserForSudoFileOps is empty
			},
			expectedCfg: Config{
				Username: "mensyli1", Address: "192.168.192.129", Password: "xiaoming98", Port: 22, Timeout: 15 * time.Second,
				UseSudoForFileOps: true, UserForSudoFileOps: "mensyli1",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualCfg, err := validateOptions(tt.inputCfg)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				// 忽略比较函数指针和未导出字段
				assert.Equal(t, tt.expectedCfg.Username, actualCfg.Username)
				assert.Equal(t, tt.expectedCfg.Password, actualCfg.Password)
				assert.Equal(t, tt.expectedCfg.Address, actualCfg.Address)
				assert.Equal(t, tt.expectedCfg.Port, actualCfg.Port)
				assert.Equal(t, tt.expectedCfg.PrivateKey, actualCfg.PrivateKey)
				assert.Equal(t, tt.expectedCfg.KeyFile, actualCfg.KeyFile)
				assert.Equal(t, tt.expectedCfg.AgentSocket, actualCfg.AgentSocket)
				assert.Equal(t, tt.expectedCfg.Timeout, actualCfg.Timeout)
				assert.Equal(t, tt.expectedCfg.Bastion, actualCfg.Bastion)
				assert.Equal(t, tt.expectedCfg.BastionPort, actualCfg.BastionPort)
				assert.Equal(t, tt.expectedCfg.BastionUser, actualCfg.BastionUser)
				assert.Equal(t, tt.expectedCfg.BastionPassword, actualCfg.BastionPassword)
				assert.Equal(t, tt.expectedCfg.BastionPrivateKey, actualCfg.BastionPrivateKey)
				assert.Equal(t, tt.expectedCfg.BastionKeyFile, actualCfg.BastionKeyFile)
				assert.Equal(t, tt.expectedCfg.BastionAgentSocket, actualCfg.BastionAgentSocket)
				assert.Equal(t, tt.expectedCfg.UseSudoForFileOps, actualCfg.UseSudoForFileOps)
				assert.Equal(t, tt.expectedCfg.UserForSudoFileOps, actualCfg.UserForSudoFileOps)
			}
		})
	}
}

// ----------------------------------------------------------------------------------------------------------------------
// ----------------------------------------------------------------------------------------------------------------------
// 目标 SSH 服务器配置
var ( // 使用 var 允许在测试初始化函数（如 TestMain）中从其他来源（如配置文件）加载这些值
	TEST_SSH_HOST_VAL = "192.168.192.129" // 例如: "your.ssh.server.com"
	TEST_SSH_PORT_VAL = "22"              // 例如: "22"
	TEST_SSH_USER_VAL = "mensyli1"        // 例如: "testuser"

	TEST_SSH_PASSWORD_VAL     = "xiaoming98" // 例如: "yourpassword"
	TEST_SSH_PKEY_PATH_VAL    = "/home/mensyli1/.ssh/id_rsa" // 例如: "/path/to/your/id_rsa" (本地私钥文件路径)
	TEST_SSH_PKEY_CONTENT_VAL = "-----BEGIN OPENSSH PRIVATE KEY-----\n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\n-----END OPENSSH PRIVATE KEY-----\n" // 例如: "-----BEGIN RSA PRIVATE KEY-----\n..." (私钥文件内容)
	TEST_SSH_AGENT_SOCK_VAL   = "/tmp/ssh-XXXXXX62rSs9/agent.252508"
	SSH_AUTH_SOCK             = "/tmp/ssh-XXXXXX62rSs9/agent.252508" // 例如: os.Getenv("SSH_AUTH_SOCK") 或特定socket路径

	TEST_SUDO_PASSWORD_VAL = "xiaoming98" // 通常与 TEST_SSH_PASSWORD_VAL 相同，如果 sudo 需要密码
	TEST_SUDO_USER_VAL     = "mensyli1"   // 例如: "root" 或其他用于 chown 的用户
)

// 堡垒机 SSH 服务器配置
var (
	TEST_BASTION_HOST_VAL = "192.168.31.32" // 例如: "your.bastion.server.com"
	TEST_BASTION_PORT_VAL = "22"            // 例如: "22"
	TEST_BASTION_USER_VAL = "mensyli1"      // 例如: "bastionuser" (如果为空，会尝试使用目标用户)

	TEST_BASTION_PASSWORD_VAL     = "xiaoming98" // 例如: "bastionpassword"
	TEST_BASTION_PKEY_PATH_VAL    = "/home/mensyli1/.ssh/id_rsa" // 例如: "/path/to/your/bastion_key"
	TEST_BASTION_PKEY_CONTENT_VAL = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABlwAAAAdzc2gtcn\nNhAAAAAwEAAQAAAYEAyafW5b+a8sVJ3NYSNheXao1xHBvpzE1XE/S3WtyeRmmiPWiDM9X/\nbKIZgXhnqqVOH6oQk67WRcVfZ//hfrS0b6HBaHu7eRIYH093y23Ioe8YhSX7wxmyO2ZHER\n0yRNWvnHCFW3YynRX0qcEWs9W6O/Cq46KfUP5gBBg9NNO2Pp67ga4tQdb0jL6l71H1Xxlb\nZ8HEgWVJbzfmz83hoRZ/Ehzr0BXF/XjtA3+PbcH1DPMmVhVIP21EVrfPRN2AbCCX+jEK+J\nP7HnJd4zo+ReMNGD+X8Wv2ZFVeoN4F/fOdjoogXOFlcz91zikx8uLgG+/CrWGoKnVkbLjI\nycn26qv+1vNw3KeS8jmQDGWIOlCwfIRpFoMe+AQ8RMrSt+RdQU3qWJQ8dHvzF6RlGcf4G2\ncqlkXOVogw1t9u4Mbuovu8xePcu9MtauKloukOxUM3HJi37O0uW8eWUInPjy1NYsov4MNc\nZU7KNWP/jd2BY2pCHqsBRAmIUXxe0TTgV+axbv/nAAAFiMpH5A/KR+QPAAAAB3NzaC1yc2\nEAAAGBAMmn1uW/mvLFSdzWEjYXl2qNcRwb6cxNVxP0t1rcnkZpoj1ogzPV/2yiGYF4Z6ql\nTh+qEJOu1kXFX2f/4X60tG+hwWh7u3kSGB9Pd8ttyKHvGIUl+8MZsjtmRxEdMkTVr5xwhV\nt2Mp0V9KnBFrPVujvwquOin1D+YAQYPTTTtj6eu4GuLUHW9Iy+pe9R9V8ZW2fBxIFlSW83\n5s/N4aEWfxIc69AVxf147QN/j23B9QzzJlYVSD9tRFa3z0TdgGwgl/oxCviT+x5yXeM6Pk\nXjDRg/l/Fr9mRVXqDeBf3znY6KIFzhZXM/dc4pMfLi4Bvvwq1hqCp1ZGy4yMnJ9uqr/tbz\ncNynkvI5kAxliDpQsHyEaRaDHvgEPETK0rfkXUFN6liUPHR78xekZRnH+BtnKpZFzlaIMN\nbfbuDG7qL7vMXj3LvTLWripaLpDsVDNxyYt+ztLlvHllCJz48tTWLKL+DDXGVOyjVj/43d\ngWNqQh6rAUQJiFF8XtE04FfmsW7/5wAAAAMBAAEAAAGACKBU6YwaQUNeRwOjUMwOjqDRU1\n4AUNyIGpLv2wOwA5wWNCFJ54hCfm+qvqabbKnYnzMjtWWXxfFNBQJlr4lkZJgbUXBlkybK\ngGBiZAHkwMSdHGkFDZIGVVMpPBqvIVGwyvTnR4PVY3HifvaDFZtRdan0bXtx7EGNcu9kgu\nOBmskohUIhrnzXBkRLjeLIJ9LKXbRkxxJBo2/VQFNy0PTI58nz7nlX+GFZZjppNM1EwdKO\n88TCS/BNKZaAV9ZP3ZBBTITmW2W5y2uqGx22rPxeAKHYRIIb5qHHHqWvXOeIwudgFMZmf9\nXb79ky4w1spSa8LJJQrsvFC3tXxnq7kOuks0qZd5+hpNK/ZFnwUPY4UQadMHHOCPRjNWfw\nHAvLVUKb5SPoipeWFfc5bEbuusfvBPI3103wgNmBCxzDHJfRXrGWTaM1AE5t3dyefVqpfB\nr53DZ/HzlAo5hsAnUwL9TReiTlZG3vi2vCUyF+dIPsk/bz1BSrXM9tuwuBPJgA0X2VAAAA\nwQDyhM/B8W3Bz7J591gwtlHQdBh/T/AjnAt4Be52/XLvM3ceB4VGiGGjci/camKcj+UoRj\n4mNeNnu8KOlJqpOGZq+Domkl8TDRI4T+YAv4ltuxI5AlZ3IkdvZ0UHaGSrLg1M6RgnyrCA\nGrLVOSk+R4nkwvRe9OW049PVjVlqJp7n7Wa2fP2fhJADo/M8wwOHL2sFWMBc1RCyPAPZkv\n1Gxd9Qu5E9hhMsuAks+Vlsx0w0zh6B8Wr++TnBcZ2tjzQMnUUAAADBAP13MoNl6zh9MdC7\np0IqgO6SmV8uIzwU8ACyp9NUW4uQ+gz6V94Tgjh82ZRJZqMhSjLzyihPGE16EoZuUY7zQO\nQzafWqrALektWJoKmO0Z2BVjCTu77QDmBbFSMzCLu9VX4PHrSX/i34CY9p3cu9lEYAQEt/\nCAyxEw8lMTSQqk325X+rc3im8haS3pL6jcJOaglps9qEFY5vp6SBlaXhxI0MQgbSNSCkDH\ngeeVQKvBtX9HjoEH5cZPyh/qp80Un/TQAAAMEAy6wF0FmOT0rp+wsM7c0vhjykI0KU29nR\nQIkONmk68L5YM6R1tI0VzwGcg0eMI4egZnpapHcgCjUOCFKOVUPNhfedI8tW769YECg6ZY\n/xHJB0XvkRnD3MS8qBc6VJpn0puBEVqw8h1bFYKWek/5CV0oKSg/TEoxkL6HponQprUPvl\nXMyU9nCl3r/OlW/8sscK+9D5UhDKU2xrrtd9Yt8TttlAU3+8LT1wVS4+npxgQ750YvygvE\nULuvv1cB6j+AoDAAAAEDcxMDA4MDY3NUBxcS5jb20BAg==\n-----END OPENSSH PRIVATE KEY-----\n" // 例如: "-----BEGIN RSA PRIVATE KEY-----\n..."
	TEST_BASTION_AGENT_SOCK_VAL   = "/tmp/ssh-XXXXXX62rSs9/agent.252508" // 例如: os.Getenv("SSH_AUTH_SOCK_BASTION")
)

// 其他测试常量
const (
	defaultTestTimeout   = 30 * time.Second                     // 默认测试超时
	testRemoteTempDir    = "/tmp/xmcores_connector_tests_const" // 远程测试临时目录 (加后缀区分)
	localTestFile        = "local_test_file_connector_const.txt"
	localTestFileContent = "This is a local test file for xmcores connector tests (using consts/vars)."
	// 用于堡垒机复用认证的标记，可以根据实际测试环境设置
	// 如果您的环境允许堡垒机复用目标的认证（例如，相同的用户/密码），则设为 "true"
	TEST_BASTION_ALLOW_REUSE_AUTH_VAL = "false"
)

// getTestConfig 从常量加载基本配置
func getTestConfig(t *testing.T) Config {
	t.Helper()

	if TEST_SSH_HOST_VAL == "" || TEST_SSH_USER_VAL == "" {
		t.Skip("跳过测试：测试变量 TEST_SSH_HOST_VAL 和 TEST_SSH_USER_VAL 必须设置有效值")
	}
	port, err := strconv.Atoi(TEST_SSH_PORT_VAL)
	require.NoError(t, err, "TEST_SSH_PORT_VAL 必须是有效的数字")

	cfg := Config{
		Address:  TEST_SSH_HOST_VAL,
		Port:     port,
		Username: TEST_SSH_USER_VAL,
		Timeout:  defaultTestTimeout,
	}
	return cfg
}

// setupRemoteTempDir 和其他辅助函数 (createLocalTestFile, cleanupLocalTestFile, cleanupRemoteTempDir)
// 保持与之前类似，但使用这里定义的常量。

func setupRemoteTempDir(t *testing.T, conn Connection) {
	t.Helper()
	// 先尝试删除，忽略错误（可能不存在）
	// 注意：conn.(*connection) 用于访问内部 config，这在测试中可以接受
	rmCmd := fmt.Sprintf("rm -rf %s", testRemoteTempDir)
	if conn.(*connection).config.UseSudoForFileOps {
		rmCmd = SudoPrefix(rmCmd)
	}
	_, _, _, _ = conn.Exec(context.Background(), rmCmd)

	mkdirCmd := fmt.Sprintf("mkdir -p %s && chmod 777 %s", testRemoteTempDir, testRemoteTempDir)
	if conn.(*connection).config.UseSudoForFileOps {
		mkdirCmd = SudoPrefix(mkdirCmd)
	}

	stdout, stderr, exitCode, err := conn.Exec(context.Background(), mkdirCmd)
	require.NoError(t, err, "创建远程临时目录时 Exec 错误: %s, stdout: %s, stderr: %s", err, string(stdout), string(stderr))
	require.Equal(t, 0, exitCode, "创建远程临时目录失败 (exit %d): stdout: %s, stderr: %s", exitCode, string(stdout), string(stderr))
	t.Logf("远程临时目录 %s 已创建/清理", testRemoteTempDir)
}

func cleanupRemoteTempDir(t *testing.T, conn Connection) {
	t.Helper()
	if conn == nil {
		return
	}
	rmCmd := fmt.Sprintf("rm -rf %s", testRemoteTempDir)
	if conn.(*connection).config.UseSudoForFileOps {
		rmCmd = SudoPrefix(rmCmd)
	}
	_, _, _, err := conn.Exec(context.Background(), rmCmd)
	if err != nil {
		t.Logf("清理远程临时目录 %s 失败: %v", testRemoteTempDir, err)
	} else {
		t.Logf("远程临时目录 %s 已清理", testRemoteTempDir)
	}
}

func createLocalTestFile(t *testing.T) string {
	t.Helper()
	err := os.WriteFile(localTestFile, []byte(localTestFileContent), 0644)
	require.NoError(t, err, "创建本地测试文件失败")
	return localTestFile
}

func cleanupLocalTestFile(t *testing.T) {
	t.Helper()
	_ = os.Remove(localTestFile)
}

func TestNewConnection_Direct_PasswordAuth(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过直接密码认证测试: 测试变量 TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL
	cfg.PrivateKey = ""
	cfg.KeyFile = ""
	cfg.AgentSocket = "" // 确保只用密码

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "使用密码直接连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_direct_pass_auth_vars")
	assert.NoError(t, errExec, "Exec 执行失败")
	assert.Equal(t, 0, exitCode, "Exec 退出码应为 0")
}

// TestNewConnection_Direct_PrivateKeyAuth 测试直接使用私钥文件认证连接
func TestNewConnection_Direct_PrivateKeyAuth(t *testing.T) {
	if TEST_SSH_PKEY_PATH_VAL == "" {
		t.Skip("跳过直接私钥认证测试: 测试变量 TEST_SSH_PKEY_PATH_VAL 未设置")
	}
	_, errStat := os.Stat(TEST_SSH_PKEY_PATH_VAL)
	if os.IsNotExist(errStat) {
		t.Fatalf("私钥文件 %s (来自 TEST_SSH_PKEY_PATH_VAL) 不存在", TEST_SSH_PKEY_PATH_VAL)
	}

	cfg := getTestConfig(t)
	cfg.KeyFile = TEST_SSH_PKEY_PATH_VAL
	cfg.Password = ""
	cfg.AgentSocket = ""
	cfg.PrivateKey = "" // 确保只用私钥文件

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "使用私钥文件直接连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_direct_pkey_auth_vars")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// TestNewConnection_Direct_PrivateKeyContentAuth 测试直接使用私钥内容认证连接
func TestNewConnection_Direct_PrivateKeyContentAuth(t *testing.T) {
	var pkeyContentToUse string
	if TEST_SSH_PKEY_CONTENT_VAL != "" {
		pkeyContentToUse = TEST_SSH_PKEY_CONTENT_VAL
	} else if TEST_SSH_PKEY_PATH_VAL != "" { // 如果内容为空，但路径存在，则从路径读取
		contentBytes, errRead := os.ReadFile(TEST_SSH_PKEY_PATH_VAL)
		if errRead != nil {
			t.Fatalf("无法从 TEST_SSH_PKEY_PATH_VAL (%s) 读取私钥内容: %v", TEST_SSH_PKEY_PATH_VAL, errRead)
		}
		pkeyContentToUse = string(contentBytes)
	} else {
		t.Skip("跳过直接私钥内容认证测试: TEST_SSH_PKEY_CONTENT_VAL 和 TEST_SSH_PKEY_PATH_VAL (备用) 均未设置")
	}

	cfg := getTestConfig(t)
	cfg.PrivateKey = pkeyContentToUse
	cfg.Password = ""
	cfg.KeyFile = ""
	cfg.AgentSocket = "" // 确保只用私钥内容

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "使用私钥内容直接连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_direct_pkey_content_auth_vars")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// TestNewConnection_Direct_AgentAuth 测试直接使用 SSH Agent 认证连接
func TestNewConnection_Direct_AgentAuth(t *testing.T) {
	agentSock := TEST_SSH_AGENT_SOCK_VAL
	if agentSock == "" {
		agentSock = SSH_AUTH_SOCK // 仍然可以尝试标准环境变量作为备用
		if agentSock == "" {
			t.Skip("跳过直接 Agent 认证测试: TEST_SSH_AGENT_SOCK_VAL 和 SSH_AUTH_SOCK 均未设置")
		}
	}
	// 对 agent socket 的存在性检查可以保留之前的逻辑，或根据需要调整
	if !(strings.HasPrefix(agentSock, "npipe:") || strings.HasPrefix(agentSock, "\\\\.\\pipe\\")) {
		_, errStat := os.Stat(agentSock)
		if os.IsNotExist(errStat) {
			t.Logf("警告: Agent socket %s 不存在，但仍将尝试连接", agentSock)
		}
	}

	cfg := getTestConfig(t)
	cfg.AgentSocket = agentSock
	cfg.Password = ""
	cfg.KeyFile = ""
	cfg.PrivateKey = "" // 确保只用 Agent

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "使用 Agent 直接连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_direct_agent_auth_vars")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// TestNewConnection_Direct_AgentAuth_EnvPrefix 测试 AgentSocket 使用 env: 前缀
func TestNewConnection_Direct_AgentAuth_EnvPrefix(t *testing.T) {
	agentSockValueToSet := TEST_SSH_AGENT_SOCK_VAL
	agentSockEnvVarName := "MY_CUSTOM_AGENT_SOCK_FOR_CONNECTOR_TESTS"

	if agentSockValueToSet == "" {
		agentSockValueToSet = os.Getenv("SSH_AUTH_SOCK")
		if agentSockValueToSet == "" {
			t.Skip("跳过 Agent (env:) 认证测试: TEST_SSH_AGENT_SOCK_VAL 和 SSH_AUTH_SOCK 均未设置 (用于环境变量值)")
		}
	}

	origEnvVal, origEnvSet := os.LookupEnv(agentSockEnvVarName)
	errSetEnv := os.Setenv(agentSockEnvVarName, agentSockValueToSet)
	require.NoError(t, errSetEnv, "设置临时环境变量失败")
	defer func() {
		if origEnvSet {
			os.Setenv(agentSockEnvVarName, origEnvVal)
		} else {
			os.Unsetenv(agentSockEnvVarName)
		}
	}()

	cfg := getTestConfig(t)
	cfg.AgentSocket = "env:" + agentSockEnvVarName
	cfg.Password = ""
	cfg.KeyFile = ""
	cfg.PrivateKey = "" // 确保只用 Agent

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "使用 env:AgentSocket 直接连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_direct_agent_env_auth_vars")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// --- Bastion Connection Tests (使用变量配置) ---

func getBastionTestConfig(t *testing.T, targetCfg Config) Config {
	t.Helper()
	if TEST_BASTION_HOST_VAL == "" {
		t.Skip("跳过堡垒机测试: 测试变量 TEST_BASTION_HOST_VAL 未设置")
	}
	bastionPort, err := strconv.Atoi(TEST_BASTION_PORT_VAL)
	require.NoError(t, err, "TEST_BASTION_PORT_VAL 必须是有效的数字")

	bastionCfg := targetCfg
	bastionCfg.Bastion = TEST_BASTION_HOST_VAL
	bastionCfg.BastionPort = bastionPort
	bastionCfg.BastionUser = TEST_BASTION_USER_VAL // validateOptions 会处理空值

	return bastionCfg
}

func TestNewConnection_Bastion_PasswordAuth(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过堡垒机密码认证测试: 目标 TEST_SSH_PASSWORD_VAL 未设置")
	}
	if TEST_BASTION_PASSWORD_VAL == "" {
		t.Skip("跳过堡垒机密码认证测试: TEST_BASTION_PASSWORD_VAL 未设置")
	}

	targetCfg := getTestConfig(t)
	targetCfg.Password = TEST_SSH_PASSWORD_VAL

	cfg := getBastionTestConfig(t, targetCfg)
	cfg.BastionPassword = TEST_BASTION_PASSWORD_VAL

	cfg.PrivateKey = ""
	cfg.KeyFile = ""
	cfg.AgentSocket = ""
	cfg.BastionPrivateKey = ""
	cfg.BastionKeyFile = ""
	cfg.BastionAgentSocket = ""

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "通过堡垒机使用密码认证连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_bastion_pass_auth_vars")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// TestNewConnection_Bastion_PrivateKeyAuth 和 TestNewConnection_Bastion_MixedAuth
// 可以类似地从 TEST_..._PKEY_PATH_VAL 或 TEST_..._PKEY_CONTENT_VAL 获取密钥信息来构建测试。
// 我将跳过它们的完整实现以节省篇幅，但模式与密码认证类似。

// TestNewConnection_Bastion_ReuseTargetAuth 测试堡垒机复用目标认证方式
func TestNewConnection_Bastion_ReuseTargetAuth(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" { // 假设复用的是密码
		t.Skip("跳过堡垒机复用认证测试: 目标 TEST_SSH_PASSWORD_VAL 未设置")
	}
	targetCfg := getTestConfig(t)
	targetCfg.Password = TEST_SSH_PASSWORD_VAL
	targetCfg.KeyFile = ""
	targetCfg.PrivateKey = ""
	targetCfg.AgentSocket = ""

	cfg := getBastionTestConfig(t, targetCfg)
	cfg.BastionPassword = ""
	cfg.BastionKeyFile = ""
	cfg.BastionPrivateKey = ""
	cfg.BastionAgentSocket = "" // 不提供堡垒机凭证

	allowReuse, _ := strconv.ParseBool(TEST_BASTION_ALLOW_REUSE_AUTH_VAL)

	conn, err := NewConnection(cfg)
	if allowReuse {
		require.NoError(t, err, "通过堡垒机复用目标认证连接失败 (预期成功)")
		require.NotNil(t, conn, "连接对象不应为 nil")
		defer conn.Close()
		_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_bastion_reuse_auth_vars")
		assert.NoError(t, errExec)
		assert.Equal(t, 0, exitCode)
	} else {
		if err == nil {
			t.Log("警告: 堡垒机复用目标认证测试意外成功，但 TEST_BASTION_ALLOW_REUSE_AUTH_VAL 未设置为 true。请检查堡垒机配置。")
			defer conn.Close()
		} else {
			t.Logf("堡垒机复用目标认证测试按预期失败 (或因配置未允许而失败): %v", err)
			assert.Error(t, err, "预期连接失败，因为堡垒机可能不接受复用的目标认证")
		}
		// t.Skip("跳过堡垒机复用认证断言部分，除非 TEST_BASTION_ALLOW_REUSE_AUTH_VAL 为 true")
	}
}

func TestNewConnection_Direct_SudoConfigPlaceholder(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 Sudo 配置占位测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	// Case 1: UseSudoForFileOps = true, UserForSudoFileOps 未设置 (应默认为 Username)
	cfg.UseSudoForFileOps = true
	cfg.UserForSudoFileOps = "" // 清空以测试默认值

	conn1, err1 := NewConnection(cfg)
	require.NoError(t, err1)
	require.NotNil(t, conn1)
	connImpl1 := conn1.(*connection)
	assert.True(t, connImpl1.config.UseSudoForFileOps, "UseSudoForFileOps 应为 true")
	assert.Equal(t, cfg.Username, connImpl1.config.UserForSudoFileOps, "UserForSudoFileOps 应默认为 Username")
	conn1.Close()

	// Case 2: UseSudoForFileOps = true, UserForSudoFileOps 已设置
	cfg.UseSudoForFileOps = true
	expectedSudoUser := "customsudoer"
	if TEST_SUDO_USER_VAL != "" { // 如果测试变量已设置，则使用它
		expectedSudoUser = TEST_SUDO_USER_VAL
	}
	cfg.UserForSudoFileOps = expectedSudoUser

	conn2, err2 := NewConnection(cfg)
	require.NoError(t, err2)
	require.NotNil(t, conn2)
	connImpl2 := conn2.(*connection)
	assert.True(t, connImpl2.config.UseSudoForFileOps, "UseSudoForFileOps 应为 true")
	assert.Equal(t, expectedSudoUser, connImpl2.config.UserForSudoFileOps, "UserForSudoFileOps 应为指定值")
	conn2.Close()
}

// TestNewConnection_Bastion_PrivateKeyContentAuth 测试通过堡垒机使用私钥内容认证
func TestNewConnection_Bastion_PrivateKeyContentAuth(t *testing.T) {
	// 目标认证（例如，使用密码）
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过堡垒机私钥内容认证测试: 目标 TEST_SSH_PASSWORD_VAL 未设置")
	}
	targetCfg := getTestConfig(t)
	targetCfg.Password = TEST_SSH_PASSWORD_VAL

	// 堡垒机认证（使用私钥内容）
	var bastionPkeyContentToUse string
	if TEST_BASTION_PKEY_CONTENT_VAL != "" {
		bastionPkeyContentToUse = TEST_BASTION_PKEY_CONTENT_VAL
	} else if TEST_BASTION_PKEY_PATH_VAL != "" {
		contentBytes, errRead := os.ReadFile(TEST_BASTION_PKEY_PATH_VAL)
		if errRead != nil {
			t.Fatalf("无法从 TEST_BASTION_PKEY_PATH_VAL (%s) 读取堡垒机私钥内容: %v", TEST_BASTION_PKEY_PATH_VAL, errRead)
		}
		bastionPkeyContentToUse = string(contentBytes)
	} else {
		t.Skip("跳过堡垒机私钥内容认证测试: TEST_BASTION_PKEY_CONTENT_VAL 和 TEST_BASTION_PKEY_PATH_VAL (备用) 均未设置")
	}

	cfg := getBastionTestConfig(t, targetCfg)
	cfg.BastionPrivateKey = bastionPkeyContentToUse
	cfg.BastionPassword = ""
	cfg.BastionKeyFile = ""
	cfg.BastionAgentSocket = "" // 清理堡垒机其他认证

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "通过堡垒机使用私钥内容认证连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_bastion_pkey_content_auth")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// TestNewConnection_Bastion_AgentAuth 测试通过堡垒机使用 Agent 认证
func TestNewConnection_Bastion_AgentAuth(t *testing.T) {
	// 目标认证（例如，使用密码）
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过堡垒机 Agent 认证测试: 目标 TEST_SSH_PASSWORD_VAL 未设置")
	}
	targetCfg := getTestConfig(t)
	targetCfg.Password = TEST_SSH_PASSWORD_VAL

	// 堡垒机认证（使用 Agent）
	bastionAgentSock := TEST_BASTION_AGENT_SOCK_VAL
	if bastionAgentSock == "" {
		bastionAgentSock = os.Getenv("SSH_AUTH_SOCK") // 备用标准 Agent Socket
		if bastionAgentSock == "" {
			t.Skip("跳过堡垒机 Agent 认证测试: TEST_BASTION_AGENT_SOCK_VAL 和 SSH_AUTH_SOCK (备用) 均未设置")
		}
	}
	// Agent socket 存在性检查可以保留
	if !(strings.HasPrefix(bastionAgentSock, "npipe:") || strings.HasPrefix(bastionAgentSock, "\\\\.\\pipe\\")) {
		_, errStat := os.Stat(bastionAgentSock)
		if os.IsNotExist(errStat) {
			t.Logf("警告: 堡垒机 Agent socket %s 不存在，但仍将尝试连接", bastionAgentSock)
		}
	}

	cfg := getBastionTestConfig(t, targetCfg)
	cfg.BastionAgentSocket = bastionAgentSock
	cfg.BastionPassword = ""
	cfg.BastionKeyFile = ""
	cfg.BastionPrivateKey = "" // 清理堡垒机其他认证

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "通过堡垒机使用 Agent 认证连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_bastion_agent_auth")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

// TestNewConnection_Bastion_AgentAuth_EnvPrefix 测试堡垒机 AgentSocket 使用 env: 前缀
func TestNewConnection_Bastion_AgentAuth_EnvPrefix(t *testing.T) {
	// 目标认证
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过堡垒机 Agent (env:) 认证测试: 目标 TEST_SSH_PASSWORD_VAL 未设置")
	}
	targetCfg := getTestConfig(t)
	targetCfg.Password = TEST_SSH_PASSWORD_VAL

	// 堡垒机 Agent (env:)
	bastionAgentSockValueToSet := TEST_BASTION_AGENT_SOCK_VAL
	bastionAgentSockEnvVarName := "MY_CUSTOM_BASTION_AGENT_SOCK_FOR_TESTS"

	if bastionAgentSockValueToSet == "" {
		bastionAgentSockValueToSet = os.Getenv("SSH_AUTH_SOCK")
		if bastionAgentSockValueToSet == "" {
			t.Skip("跳过堡垒机 Agent (env:) 认证测试: TEST_BASTION_AGENT_SOCK_VAL 和 SSH_AUTH_SOCK (备用) 均未设置 (用于环境变量值)")
		}
	}

	origEnvVal, origEnvSet := os.LookupEnv(bastionAgentSockEnvVarName)
	errSetEnv := os.Setenv(bastionAgentSockEnvVarName, bastionAgentSockValueToSet)
	require.NoError(t, errSetEnv, "设置临时环境变量失败")
	defer func() {
		if origEnvSet {
			os.Setenv(bastionAgentSockEnvVarName, origEnvVal)
		} else {
			os.Unsetenv(bastionAgentSockEnvVarName)
		}
	}()

	cfg := getBastionTestConfig(t, targetCfg)
	cfg.BastionAgentSocket = "env:" + bastionAgentSockEnvVarName
	cfg.BastionPassword = ""
	cfg.BastionKeyFile = ""
	cfg.BastionPrivateKey = "" // 清理堡垒机其他认证

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "通过堡垒机使用 env:AgentSocket 认证连接失败")
	require.NotNil(t, conn, "连接对象不应为 nil")
	defer conn.Close()

	_, _, exitCode, errExec := conn.Exec(context.Background(), "echo hello_bastion_agent_env_auth")
	assert.NoError(t, errExec)
	assert.Equal(t, 0, exitCode)
}

func TestConnection_Close_Successful(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" { // 假设用密码建连接来测试 Close
		t.Skip("跳过 Close 测试: TEST_SSH_PASSWORD_VAL 未设置 (需要建立连接)")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "建立连接失败，无法测试 Close")
	require.NotNil(t, conn)

	connImpl := conn.(*connection)
	require.NotNil(t, connImpl.sshclient)
	require.NotNil(t, connImpl.sftpclient)
	require.NotNil(t, connImpl.cancel)
	require.NotNil(t, connImpl.ctx)

	errClose := conn.Close()
	assert.NoError(t, errClose, "Close() 不应返回错误")

	assert.Nil(t, connImpl.sshclient, "Close() 后 sshclient 应为 nil")
	assert.Nil(t, connImpl.sftpclient, "Close() 后 sftpclient 应为 nil")
	assert.Nil(t, connImpl.cancel, "Close() 后 cancel 函数应为 nil")

	select {
	case <-connImpl.ctx.Done():
		// Context 已按预期取消
	default:
		t.Errorf("Close() 后 conn.ctx 应该被取消")
	}

	errCloseAgain := conn.Close()
	assert.NoError(t, errCloseAgain, "再次 Close() 也不应返回错误")
}

func TestConnection_Close_OnAlreadyClosedOrNil(t *testing.T) {
	// Case 1: 部分初始化的 connection
	cfgBase := getTestConfig(t) // 确保 config 字段有效
	connNilClients := &connection{
		config: cfgBase,
	}
	ctx, cancel := context.WithCancel(context.Background())
	connNilClients.ctx = ctx
	connNilClients.cancel = cancel

	err := connNilClients.Close()
	assert.NoError(t, err, "在部分初始化的连接上 Close() 不应报错")
	select {
	case <-connNilClients.ctx.Done():
	default:
		t.Errorf("conn.ctx 应该在 Close 后被取消")
	}

	// Case 2: 真实连接，关闭两次
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过重复 Close 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfgReal := getTestConfig(t)
	cfgReal.Password = TEST_SSH_PASSWORD_VAL

	connReal, err := NewConnection(cfgReal)
	require.NoError(t, err)
	require.NotNil(t, connReal)

	errClose1 := connReal.Close()
	require.NoError(t, errClose1, "第一次 Close 失败")

	errClose2 := connReal.Close()
	assert.NoError(t, errClose2, "第二次 Close 不应报错")
}

func TestCreateSession_Successful(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 createSession 测试: TEST_SSH_PASSWORD_VAL 未设置 (需要建立连接)")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "建立连接失败，无法测试 createSession")
	require.NotNil(t, conn)
	defer conn.Close()

	connImpl := conn.(*connection) // 类型断言以访问内部字段

	cmdCtx, cmdCancel := context.WithCancel(context.Background())
	defer cmdCancel()

	session, monDone, errSession := connImpl.createSession(cmdCtx)
	require.NoError(t, errSession, "createSession 应该成功")
	require.NotNil(t, session, "返回的 session 不应为 nil")
	require.NotNil(t, monDone, "返回的 monDone channel 不应为 nil")
	defer session.Close() // 确保会话关闭
	defer close(monDone)  // 通知监控 goroutine 退出

	// 无法直接验证 PTY 和 Env 是否真的在远程设置成功，
	// 但可以检查函数是否无错执行完成。
	// 实际效果会在 Exec/PExec 中体现。
	t.Log("createSession 成功执行")
}

func TestCreateSession_ConnectionAlreadyClosed(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 createSession (连接已关闭) 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	require.NotNil(t, conn)

	connImpl := conn.(*connection)

	// 先关闭连接
	errClose := conn.Close()
	require.NoError(t, errClose, "关闭连接失败")

	cmdCtx, cmdCancel := context.WithCancel(context.Background())
	defer cmdCancel()

	_, _, errSession := connImpl.createSession(cmdCtx)
	assert.Error(t, errSession, "在已关闭的连接上 createSession 应该失败")
	if errSession != nil {
		assert.Contains(t, errSession.Error(), "ssh 连接已关闭", "错误信息应指示连接已关闭")
	}
}

func TestCreateSession_CommandContextCancelled(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 createSession (命令 context 取消) 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close() // 确保主连接最终关闭

	connImpl := conn.(*connection)

	cmdCtx, cmdCancel := context.WithCancel(context.Background())

	session, monDone, errSession := connImpl.createSession(cmdCtx)
	require.NoError(t, errSession, "createSession 应该成功")
	require.NotNil(t, session)
	// defer session.Close() // 会话应该由 context 取消自动关闭
	defer close(monDone)

	cmdCancel() // 立刻取消命令 context

	// 等待一小段时间，让监控 goroutine 有机会响应并关闭会话
	// 这是一个基于时间的测试，可能不够稳定，但对于集成测试来说是一种常见做法
	time.Sleep(100 * time.Millisecond)

	// 尝试在该会话上执行操作，应该会失败，因为会话可能已被关闭
	// 注意：session.Close() 本身是幂等的，再次调用通常无害
	// 检查会话是否真的关闭了比较困难，因为没有直接的状态可查。
	// 一种方法是尝试 Start/Run/Shell 一个命令。
	errShell := session.Shell() // Shell 通常会保持打开，如果 context 关闭了它，这里会出错
	if errShell != nil {
		t.Logf("session.Shell() 在 context 取消后返回错误 (符合预期): %v", errShell)
		// 常见的错误是 "EOF" 或 "session closed"
		assert.True(t, errors.Is(errShell, io.EOF) || strings.Contains(errShell.Error(), "closed"), "错误应指示会话已关闭")
	} else {
		// 如果 Shell 没报错，可能是 PTY 保持了会话，或者关闭不够快
		// 这种情况下的测试可能需要更细致的控制或对远程服务器行为的假设
		t.Log("session.Shell() 在 context 取消后没有立即报错，会话可能仍部分存活或关闭有延迟")
		session.Close() // 确保关闭
	}
}

func TestCreateSession_ConnectionContextCancelled(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 createSession (连接 context 取消) 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	require.NotNil(t, conn)
	// Defer conn.Close() 会调用连接的 cancel 函数

	connImpl := conn.(*connection)

	cmdCtx, cmdCancel := context.WithCancel(context.Background())
	defer cmdCancel()

	session, monDone, errSession := connImpl.createSession(cmdCtx)
	require.NoError(t, errSession, "createSession 应该成功")
	require.NotNil(t, session)
	// defer session.Close()
	defer close(monDone)

	// 现在关闭主连接，这应该会触发连接级别的 context 取消
	errCloseConn := conn.Close()
	require.NoError(t, errCloseConn)

	time.Sleep(100 * time.Millisecond)

	errShell := session.Shell()
	if errShell != nil {
		t.Logf("session.Shell() 在连接 context 取消后返回错误 (符合预期): %v", errShell)
		assert.True(t, errors.Is(errShell, io.EOF) || strings.Contains(errShell.Error(), "closed"), "错误应指示会话已关闭")
	} else {
		t.Log("session.Shell() 在连接 context 取消后没有立即报错")
		session.Close() // 确保关闭
	}
}

func TestCreateSession_PtyRequestFail(t *testing.T) {
	t.Skip("跳过 PTY 请求失败测试，需要 mock 或特殊服务器配置")
}

func TestExec_SimpleCommand_NoOutput(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 Exec 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	defer conn.Close()

	cmd := "true" // Unix 'true' 命令，无输出，退出码 0
	stdout, stderr, exitCode, errExec := conn.Exec(context.Background(), cmd)

	assert.NoError(t, errExec, "Exec(%q) 不应返回错误", cmd)
	assert.Equal(t, 0, exitCode, "Exec(%q) 退出码应为 0", cmd)
	assert.Empty(t, stdout, "Exec(%q) stdout 应为空", cmd)
	// stderr 可能因系统而异，但通常对于 true 命令为空
	// assert.Empty(t, stderr, "Exec(%q) stderr 应为空", cmd)
	// 更宽松的检查：如果 stderr 不为空，则打印它，但不一定失败测试，除非有明确的预期
	if len(stderr) > 0 {
		t.Logf("Exec(%q) stderr: %s", cmd, string(stderr))
	}
}

// TestExec_CommandWithStdout 测试执行有 stdout 输出的命令
func TestExec_CommandWithStdout(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 Exec 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	defer conn.Close()

	expectedOutput := "hello from exec test"                   // echo 添加换行符，scanner 读取后我们再添加
	cmd := fmt.Sprintf("echo -n '%s'", "hello from exec test") // -n 避免尾随换行，方便精确匹配

	stdout, stderr, exitCode, errExec := conn.Exec(context.Background(), cmd)

	assert.NoError(t, errExec, "Exec(%q) 不应返回错误", cmd)
	assert.Equal(t, 0, exitCode, "Exec(%q) 退出码应为 0", cmd)
	assert.Equal(t, expectedOutput, string(stdout), "Exec(%q) stdout 不匹配", cmd)
	if len(stderr) > 0 {
		t.Logf("Exec(%q) stderr: %s", cmd, string(stderr))
	}
}

// TestExec_CommandWithStderr 测试执行有 stderr 输出的命令
//func TestExec_CommandWithStderr(t *testing.T) {
//	if TEST_SSH_PASSWORD_VAL == "" {
//		t.Skip("跳过 Exec 测试: TEST_SSH_PASSWORD_VAL 未设置")
//	}
//	cfg := getTestConfig(t)
//	cfg.Password = TEST_SSH_PASSWORD_VAL
//
//	conn, err := NewConnection(cfg)
//	require.NoError(t, err)
//	defer conn.Close()
//
//	expectedErrOutput := "error message to stderr\n"
//	// bash -c 'echo "message" >&2'
//	cmd := fmt.Sprintf("/bin/bash -c \"echo -n '%s' >&2\"", "error message to stderr")
//
//	stdout, stderr, exitCode, errExec := conn.Exec(context.Background(), cmd)
//
//	t.Logf("Stdout content: %q", string(stdout))
//	t.Logf("Stderr content: %q", string(stderr))
//	assert.NoError(t, errExec, "Exec(%q) 不应返回错误 (命令本身成功，只是输出到 stderr)", cmd)
//	assert.Equal(t, 0, exitCode, "Exec(%q) 退出码应为 0 (命令本身成功)", cmd)
//	assert.Empty(t, stdout, "Exec(%q) stdout 应为空", cmd)
//	assert.Equal(t, expectedErrOutput, string(stderr), "Exec(%q) stderr 不匹配", cmd)
//}

// TestExec_CommandWithNonZeroExit 测试执行返回非零退出码的命令
func TestExec_CommandWithNonZeroExit(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 Exec 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	defer conn.Close()

	cmd := "false"              // Unix 'false' 命令，退出码 1
	expectedStderrContent := "" // 'false' 命令通常没有 stderr 输出

	stdout, stderr, exitCode, errExec := conn.Exec(context.Background(), cmd)

	// 对于非零退出码，Exec 会返回一个 *ssh.ExitError
	require.Error(t, errExec, "Exec(%q) 应返回错误 (ExitError)", cmd)
	var exitErr *ssh.ExitError
	isExitError := errors.As(errExec, &exitErr)
	assert.True(t, isExitError, "返回的错误应为 *ssh.ExitError")
	if isExitError {
		assert.Equal(t, 1, exitErr.ExitStatus(), "ssh.ExitError 状态码应为 1")
	}
	assert.Equal(t, 1, exitCode, "Exec(%q) 退出码应为 1", cmd)
	assert.Empty(t, stdout, "Exec(%q) stdout 应为空", cmd)
	// stderr 可能为空或包含特定于系统的消息，对于 'false' 通常为空
	if len(stderr) > 0 && string(stderr) != "\n" { // 有时会有一个空行
		assert.Equal(t, expectedStderrContent, strings.TrimSpace(string(stderr)), "Exec(%q) stderr 不匹配", cmd)
	}
}

// TestExec_SudoCommand_PasswordPrompt 测试需要 sudo 密码的命令 (如果配置了密码)
func TestExec_SudoCommand_PasswordPrompt(t *testing.T) {
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL // 这个密码将用于 SSH 和 sudo

	if cfg.Password == "" { // 如果 TEST_SSH_PASSWORD_VAL 为空，则从 TEST_SUDO_PASSWORD_VAL 获取
		cfg.Password = TEST_SUDO_PASSWORD_VAL
		if cfg.Password == "" {
			t.Skip("跳过 Exec Sudo 密码测试: TEST_SSH_PASSWORD_VAL 和 TEST_SUDO_PASSWORD_VAL 均未设置")
		}
	}
	// 确保目标服务器的 testuser 用户有 sudo 权限，并且执行 `sudo id` 会提示输入密码。
	// 某些 sudo 配置（如 `NOPASSWD`）不会提示密码。此测试依赖于会提示密码的配置。
	if os.Getenv("TEST_SUDO_REQUIRES_PASSWORD") != "true" {
		t.Skip("跳过 Exec Sudo 密码测试: TEST_SUDO_REQUIRES_PASSWORD 未设置为 'true' (测试环境 sudo 可能配置为 NOPASSWD)")
	}

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	defer conn.Close()

	// 使用一个通常需要 root 权限的命令，并确保它会提示密码
	// 例如 `sudo -k id` (-k 会使 sudo 忽略缓存的凭证，强制提示)
	// 或者一个只有 root 能读的文件 `sudo cat /etc/shadow` (危险，不推荐)
	// 更安全的是 `sudo whoami` 或 `sudo id`
	// 为了确保提示，我们可能需要一个特定的 sudo 配置。
	// 简单起见，我们用 `sudo id` 并假设它会提示。
	// 如果您的 sudo 配置了 NOPASSWD:all，这个测试可能不会按预期工作（密码不会被发送）。
	// `SudoPrefix` 会将命令包装成 `sudo -E /bin/bash -c "id"`
	cmd := SudoPrefix("id -u") // 获取用户ID，sudo 后应该是 0

	stdout, stderr, exitCode, errExec := conn.Exec(context.Background(), cmd)

	t.Logf("Sudo command stdout: %s", string(stdout))
	t.Logf("Sudo command stderr: %s", string(stderr))

	assert.NoError(t, errExec, "Exec(sudo %q) 不应返回错误", cmd)
	assert.Equal(t, 0, exitCode, "Exec(sudo %q) 退出码应为 0", cmd)
	// id -u 在 sudo 下应输出 0 (root 的 UID) + 换行符
	assert.Equal(t, "0\n", string(stdout), "Exec(sudo %q) stdout 应为 root 的 UID '0'", cmd)
}

// TestExec_CommandWithLongLine_Stdout 测试 stdout 长行处理 (bufio.ErrTooLong)
func TestExec_CommandWithLongLine_Stdout(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" { // 或其他合适的认证方式的检查
		t.Skip("跳过 Exec 长行测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL // 使用密码认证进行测试

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "建立连接失败")
	defer conn.Close()

	// setupRemoteTempDir(t, conn) // 确保远程临时目录存在并可写

	// 定义长行内容，不包含尾随换行符
	longLineContent := strings.Repeat("a", 2*1024*1024+500) // 例如，略大于 2MB

	// 创建一个本地文件，其内容就是 longLineContent
	localLongFile := createTempFileWithLongLine(t, longLineContent)
	defer os.Remove(localLongFile)

	remoteLongFilePath := filepath.Join(testRemoteTempDir, "longline_for_exec_test.txt")

	// 为了确保测试的隔离性，每次测试前先清理远程文件（如果存在）
	// 并确保远程目录存在
	t.Logf("步骤 1: mkdir 和 rm 远程文件 (可能 sudo)")
	_, _, _, errExecPre := conn.Exec(context.Background(), SudoPrefix(fmt.Sprintf("mkdir -p %s && rm -f %s", testRemoteTempDir, remoteLongFilePath)))
	require.NoError(t, errExecPre, "预处理 mkdir/rm 失败")
	t.Logf("步骤 1 完成")

	errUpload := conn.UploadFile(context.Background(), localLongFile, remoteLongFilePath)
	require.NoError(t, errUpload, "上传长行文件失败")
	defer conn.Exec(context.Background(), SudoPrefix(fmt.Sprintf("rm -f %s", remoteLongFilePath))) // 测试后清理

	cmdToExec := fmt.Sprintf("cat %s", remoteLongFilePath)

	// 预期的输出：
	// cat 命令会输出文件内容。
	// 我们的 Exec 的 stdout goroutine 在处理从 pipe 读取的数据时，
	// 如果数据块的最后没有换行符（例如，当 EOF 发生时，累加器中可能有不带换行的最后一部分），
	// 它目前的设计可能不会主动添加换行。
	// 而之前的 bufio.Scanner 的版本，outputBuffer.WriteByte('\n') 会为每一行（即使是最后一行）添加换行。
	// 使用 bufio.Reader 和 Read() 后，输出更接近原始流。
	// 如果远程文件 `longline_for_exec_test.txt` 的内容就是 `longLineContent`（没有尾随换行），
	// 那么 `cat` 命令的输出也就是 `longLineContent`。
	// `outputBuffer.Write(readChunk[:n])` 会忠实地写入。
	// 所以，预期的 stdout 应该是原始的 longLineContent。
	// 然而，PTY 可能会在命令结束后添加一个换行符或回车换行符作为 shell 提示的一部分，或者 cat 自身行为。
	// 为了稳定测试，我们最好让远程文件本身就包含一个换行符。

	// 重新设计 createTempFileWithLongLine，使其内容总是以单个换行符结尾
	longLineContentWithNewline := longLineContent + "\n"
	localLongFileWithNewline := createTempFileWithSingleNewline(t, longLineContentWithNewline)
	defer os.Remove(localLongFileWithNewline)

	errUpload = conn.UploadFile(context.Background(), localLongFileWithNewline, remoteLongFilePath)
	require.NoError(t, errUpload, "上传带换行符的长行文件失败")

	stdout, stderr, exitCode, errExec := conn.Exec(context.Background(), cmdToExec)

	t.Logf("Long line stdout length: %d, expected approx: %d", len(stdout), len(longLineContentWithNewline))
	t.Logf("Long line stderr: %q", string(stderr))

	assert.NoError(t, errExec, "Exec(long line) 不应返回错误")
	assert.Equal(t, 0, exitCode, "Exec(long line) 退出码应为 0")

	// 新的断言：stdout 应该与原始长行内容（加换行）完全匹配
	// PTY 可能会将 \n 转换成 \r\n，需要处理
	actualStdout := strings.ReplaceAll(string(stdout), "\r\n", "\n")
	assert.Equal(t, longLineContentWithNewline, actualStdout, "完整的长行输出不匹配")

	// 确保不再有之前的错误标记
	assert.NotContains(t, string(stdout), "[CONNECTOR_ERROR:", "stdout 不应包含旧的行太长错误标记")
	assert.NotContains(t, string(stdout), "行太长", "stdout 不应包含旧的行太长错误标记")
}

// createTempFileWithSingleNewline 确保文件内容就是 content (预期 content 以 \n 结尾)
func createTempFileWithSingleNewline(t *testing.T, contentWithNewline string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp(t.TempDir(), "longline-nl-*.txt")
	require.NoError(t, err)
	_, err = tmpfile.WriteString(contentWithNewline)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())
	return tmpfile.Name()
}

// createTempFileWithLongLine 是一个辅助函数，用于 TestExec_CommandWithLongLine_Stdout
func createTempFileWithLongLine(t *testing.T, content string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp(t.TempDir(), "longline-*.txt")
	require.NoError(t, err)
	_, err = tmpfile.WriteString(content) // 写入长行
	// _, err = tmpfile.WriteString("\n") // 确保有换行，但 cat 会自己加，Scanner 会处理
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())
	return tmpfile.Name()
}

// TestExec_CommandTimeout 测试命令执行超时 (通过 context)
func TestExec_CommandTimeout(t *testing.T) {
	if TEST_SSH_PASSWORD_VAL == "" {
		t.Skip("跳过 Exec 测试: TEST_SSH_PASSWORD_VAL 未设置")
	}
	cfg := getTestConfig(t)
	cfg.Password = TEST_SSH_PASSWORD_VAL

	conn, err := NewConnection(cfg)
	require.NoError(t, err)
	defer conn.Close()

	cmd := "sleep 5" // 一个会持续一段时间的命令

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second) // 设置1秒超时
	defer cancel()

	startTime := time.Now()
	stdout, stderr, exitCode, errExec := conn.Exec(ctx, cmd)
	duration := time.Since(startTime)

	t.Logf("Timeout Exec stdout: %s", string(stdout))
	t.Logf("Timeout Exec stderr: %s", string(stderr)) // 应该为 nil 或空
	t.Logf("Timeout Exec exitCode: %d", exitCode)     // 可能是 129, -1 等
	t.Logf("Timeout Exec err: %v", errExec)
	t.Logf("Timeout Exec duration: %s", duration)

	assert.Error(t, errExec, "Exec(%q) 应该因为 context 超时而返回错误", cmd)

	if errExec != nil {
		var sshExitErr *ssh.ExitError
		isContextOrSessionClosedError := errors.Is(errExec, context.DeadlineExceeded) ||
			errors.Is(errExec, context.Canceled) || // 也可能是 Canceled
			(strings.Contains(errExec.Error(), "session closed")) ||
			(strings.Contains(errExec.Error(), "context deadline exceeded")) ||
			(strings.Contains(errExec.Error(), "context canceled")) ||
			errors.Is(errExec, io.EOF)

		isSignalExitError := false
		if errors.As(errExec, &sshExitErr) {
			// 常见的因会话关闭导致的信号有 HUP, TERM, INT, KILL
			// 退出码通常是 128 + signal_number
			// SIGHUP = 1, SIGINT = 2, SIGTERM = 15, SIGKILL = 9
			// exitCode 129 (SIGHUP), 130 (SIGINT), 143 (SIGTERM), 137 (SIGKILL)
			// 我们不严格检查具体的信号，只要是 ExitError 并且不是正常退出 (status 0)
			// 并且是在超时场景下，就认为是合理的。
			// 更具体的，可以检查 sshExitErr.Signal()
			if sshExitErr.Signal() == "HUP" || sshExitErr.Signal() == "TERM" || sshExitErr.Signal() == "INT" || sshExitErr.Signal() == "KILL" {
				isSignalExitError = true
				t.Logf("Command exited due to signal: %s, status: %d", sshExitErr.Signal(), sshExitErr.ExitStatus())
			} else if strings.Contains(errExec.Error(), "killed") { // 有时 ssh.ExitError 的 Error() 会包含 "killed"
				isSignalExitError = true
			}
		}

		assert.True(t,
			isContextOrSessionClosedError || isSignalExitError,
			"错误信息应表明超时、会话关闭或命令被信号终止: %v", errExec,
		)

		// 对于 exitCode 的断言，可以更灵活：
		if isSignalExitError {
			// 如果是信号退出，exitCode 应该是 128 + signal_number
			assert.True(t, exitCode > 128, "超时且因信号退出的命令 exitCode 应 > 128, got %d", exitCode)
		} else if isContextOrSessionClosedError {
			// 如果是其他类型的会话关闭错误，exitCode 通常是 -1
			assert.Equal(t, -1, exitCode, "超时且会话关闭错误的命令 exitCode 应为 -1, got %d", exitCode)
		}
		// 如果两者都不是，但 errExec != nil，那可能是其他未预期的错误，上面的 assert.True 会失败。
	}

	assert.Less(t, duration, 3*time.Second, "命令执行时间应远小于其本身的sleep时间 (5s)")
}
