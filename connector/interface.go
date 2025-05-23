package connector

import (
	"context"
	"github.com/mensylisir/xmcores/cache"
	"github.com/mensylisir/xmcores/common"
	"io"
	"os"
	"time"
)

type Executor interface {
	Exec(ctx context.Context, cmd string) (stdout []byte, stderr []byte, exitCode int, err error)
	PExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (exitCode int, err error)
}

type FileOperator interface {
	DownloadFile(ctx context.Context, remotePath string, localPath string) error
	UploadFile(ctx context.Context, localPath string, remotePath string) error
	Fetch(ctx context.Context, remotePath string) (io.ReadCloser, error)
	Scp(ctx context.Context, localReader io.Reader, remotePath string, sizeHint int64, mode os.FileMode) error
	StatRemote(ctx context.Context, remotePath string) (os.FileInfo, error)
	RemoteFileExist(ctx context.Context, remotePath string) (bool, error)
	RemoteDirExist(ctx context.Context, remotePath string) (bool, error)
	MkDirAll(ctx context.Context, remotePath string, mode os.FileMode) error
	Chmod(ctx context.Context, remotePath string, mode os.FileMode) error
}

type Connection interface {
	Executor
	FileOperator
	Close() error
}

type Connector interface {
	Connect(ctx context.Context, host Host) (Connection, error)
	Close() error
}

type Host interface {
	GetName() string
	SetName(name string)
	GetAddress() string
	SetAddress(addr string)
	GetInternalAddress() string
	GetInternalIPv4Address() string
	GetInternalIPv6Address() string
	SetInternalAddress(addr string)
	SetInternalAddresses(ipv4, ipv6 string)
	GetPort() int
	SetPort(port int)
	GetUser() string
	SetUser(user string)
	GetPassword() string
	SetPassword(password string)
	GetPrivateKey() string
	SetPrivateKey(privateKey string)
	GetPrivateKeyPath() string
	SetPrivateKeyPath(path string)
	GetArch() common.Arch
	SetArch(arch common.Arch)
	GetTimeout() time.Duration
	SetTimeout(timeout time.Duration)
	GetRoles() []string
	SetRoles(roles []string)
	AddRole(role string)
	RemoveRole(role string)
	IsRole(role string) bool
	GetCache() *cache.Cache[string, any]
	SetCache(c *cache.Cache[string, any])
	GetVars() map[string]interface{}
	SetVars(vars map[string]interface{})
	GetVar(key string) (value interface{}, exists bool)
	SetVar(key string, value interface{})
	Validate() error
	ID() string
}
