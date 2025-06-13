package connector

const (
	RoleControlPlane = "control-plane" // Standardized role name
	RoleMaster       = "master"        // Often used interchangeably with control-plane
	RoleWorker       = "worker"
	RoleEtcd         = "etcd"
	RoleLoadBalancer = "loadbalancer"
	RoleK8s          = "k8s" // General Kubernetes node role
	// Add other common roles as needed
)

import (
	"fmt"
	"strings"
	"time"

	"github.com/mensylisir/xmcores/cache"
	"github.com/mensylisir/xmcores/common"
)

var _ Host = (*BaseHost)(nil)

type BaseHost struct {
	Name              string        `yaml:"name,omitempty" json:"name,omitempty"`
	Address           string        `yaml:"address,omitempty" json:"address,omitempty"`
	InternalAddress   string        `yaml:"internalAddress,omitempty" json:"internalAddress,omitempty"`
	Port              int           `yaml:"port,omitempty" json:"port,omitempty"`
	User              string        `yaml:"user,omitempty" json:"user,omitempty"`
	Password          string        `yaml:"password,omitempty" json:"password,omitempty"`
	PrivateKey        string        `yaml:"privateKey,omitempty" json:"privateKey,omitempty"`
	PrivateKeyPath    string        `yaml:"privateKeyPath,omitempty" json:"privateKeyPath,omitempty"`
	HostArch          common.Arch   `yaml:"arch,omitempty" json:"arch,omitempty"`
	ConnectionTimeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	roles     []string
	roleTable map[string]bool
	hostCache *cache.Cache[string, any]
	vars      map[string]interface{}
}

func NewHost() *BaseHost {
	bh := &BaseHost{
		roles:     make([]string, 0),
		roleTable: make(map[string]bool),
		vars:      make(map[string]interface{}),
		hostCache: cache.NewCache[string, any](cache.WithDefaultTTL[string, any](5 * time.Minute)),
	}

	if bh.Port == 0 {
		bh.Port = 22
	}
	if bh.ConnectionTimeout == 0 {
		bh.ConnectionTimeout = 30 * time.Second
	}
	return bh
}

func (b *BaseHost) GetName() string {
	return b.Name
}

func (b *BaseHost) SetName(name string) {
	b.Name = name
}

func (b *BaseHost) GetAddress() string {
	return b.Address
}

func (b *BaseHost) SetAddress(addr string) {
	b.Address = addr
}

func (b *BaseHost) GetInternalAddress() string {
	return b.InternalAddress
}

func (b *BaseHost) SetInternalAddress(addr string) {
	b.InternalAddress = addr
}

func (b *BaseHost) GetInternalIPv4Address() string {
	if b.InternalAddress == "" {
		return ""
	}
	parts := strings.Split(b.InternalAddress, ",")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func (b *BaseHost) GetInternalIPv6Address() string {
	if b.InternalAddress == "" {
		return ""
	}
	parts := strings.Split(b.InternalAddress, ",")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func (b *BaseHost) SetInternalAddresses(ipv4, ipv6 string) {
	var addresses []string
	cleanIPv4 := strings.TrimSpace(ipv4)
	cleanIPv6 := strings.TrimSpace(ipv6)

	if cleanIPv4 != "" {
		addresses = append(addresses, cleanIPv4)
	}
	if cleanIPv6 != "" {
		addresses = append(addresses, cleanIPv6)
	}
	b.InternalAddress = strings.Join(addresses, ",")
}

func (b *BaseHost) GetPort() int {
	return b.Port
}

func (b *BaseHost) SetPort(port int) {
	b.Port = port
}

func (b *BaseHost) GetUser() string {
	return b.User
}

func (b *BaseHost) SetUser(u string) {
	b.User = u
}

func (b *BaseHost) GetPassword() string {
	return b.Password
}

func (b *BaseHost) SetPassword(password string) {
	b.Password = password
}

func (b *BaseHost) GetPrivateKey() string {
	return b.PrivateKey
}

func (b *BaseHost) SetPrivateKey(privateKey string) {
	b.PrivateKey = privateKey
}

func (b *BaseHost) GetPrivateKeyPath() string {
	return b.PrivateKeyPath
}

func (b *BaseHost) SetPrivateKeyPath(path string) {
	b.PrivateKeyPath = path
}

func (b *BaseHost) GetArch() common.Arch {
	return b.HostArch
}

func (b *BaseHost) SetArch(arch common.Arch) {
	b.HostArch = arch
}

func (b *BaseHost) GetTimeout() time.Duration {
	return b.ConnectionTimeout
}

func (b *BaseHost) SetTimeout(timeout time.Duration) {
	b.ConnectionTimeout = timeout
}

func (b *BaseHost) GetRoles() []string {
	rolesCopy := make([]string, len(b.roles))
	copy(rolesCopy, b.roles)
	return rolesCopy
}

func (b *BaseHost) SetRoles(roles []string) {
	b.roles = make([]string, 0, len(roles))
	b.roleTable = make(map[string]bool, len(roles))
	for _, role := range roles {
		b.AddRole(role)
	}
}

func (b *BaseHost) AddRole(role string) {
	trimmedRole := strings.TrimSpace(role)
	if trimmedRole == "" {
		return
	}
	if _, exists := b.roleTable[trimmedRole]; !exists {
		b.roleTable[trimmedRole] = true
		b.roles = append(b.roles, trimmedRole)
	}
}

func (b *BaseHost) RemoveRole(role string) {
	trimmedRole := strings.TrimSpace(role)
	if _, exists := b.roleTable[trimmedRole]; exists {
		delete(b.roleTable, trimmedRole)
		newRoles := make([]string, 0, len(b.roles)-1)
		for _, r := range b.roles {
			if r != trimmedRole {
				newRoles = append(newRoles, r)
			}
		}
		b.roles = newRoles
	}
}

func (b *BaseHost) IsRole(role string) bool {
	_, exists := b.roleTable[strings.TrimSpace(role)]
	return exists
}

func (b *BaseHost) GetCache() *cache.Cache[string, any] {
	if b.hostCache == nil {
		b.hostCache = cache.NewCache[string, any](cache.WithDefaultTTL[string, any](5 * time.Minute))
	}
	return b.hostCache
}

func (b *BaseHost) SetCache(c *cache.Cache[string, any]) {
	b.hostCache = c
}

func (b *BaseHost) GetVars() map[string]interface{} {
	varsCopy := make(map[string]interface{}, len(b.vars))
	for k, v := range b.vars {
		varsCopy[k] = v
	}
	return varsCopy
}

func (b *BaseHost) SetVars(vars map[string]interface{}) {
	if vars == nil {
		b.vars = make(map[string]interface{}) // Ensure vars is not nil
		return
	}
	// Create a new map to avoid external modifications to the passed map affecting BaseHost
	newVars := make(map[string]interface{}, len(vars))
	for k, v := range vars {
		newVars[k] = v
	}
	b.vars = newVars
}

func (b *BaseHost) GetVar(key string) (value interface{}, exists bool) {
	if b.vars == nil {
		return nil, false
	}
	val, ok := b.vars[key]
	return val, ok
}

func (b *BaseHost) SetVar(key string, value interface{}) {
	if b.vars == nil {
		b.vars = make(map[string]interface{})
	}
	b.vars[key] = value
}

func (b *BaseHost) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("host name cannot be empty")
	}
	if strings.TrimSpace(b.Address) == "" {
		return fmt.Errorf("host address cannot be empty for host '%s'", b.Name)
	}
	if b.Port <= 0 || b.Port > 65535 {
		return fmt.Errorf("invalid port number %d for host '%s'", b.Port, b.Name)
	}
	if strings.TrimSpace(b.User) == "" {
		return fmt.Errorf("user cannot be empty for host '%s'", b.Name)
	}
	hasPassword := strings.TrimSpace(b.Password) != ""
	hasPrivateKey := strings.TrimSpace(b.PrivateKey) != ""
	hasPrivateKeyPath := strings.TrimSpace(b.PrivateKeyPath) != ""
	if !hasPassword && !hasPrivateKey && !hasPrivateKeyPath {
		return fmt.Errorf("authentication method (password, privateKey, or privateKeyPath) must be provided for host '%s'", b.Name)
	}

	if b.HostArch != "" && !b.isValidArch(b.HostArch) {
		return fmt.Errorf("invalid architecture '%s' for host '%s'", b.HostArch, b.Name)
	}
	return nil
}

func (b *BaseHost) isValidArch(arch common.Arch) bool {
	switch arch {
	case common.ArchAmd64, common.ArchX86_64, common.ArchArm64, common.ArchArm:
		return true
	case common.ArchUnknown:
		return true
	case "":
		return true
	default:
		fmt.Printf("Warning: Unrecognized architecture '%s' for host '%s'. Validation might need adjustment.\n", arch, b.Name)
		return false
	}
}

func (b *BaseHost) ID() string {
	if trimmedName := strings.TrimSpace(b.Name); trimmedName != "" {
		return trimmedName
	}
	if trimmedAddress := strings.TrimSpace(b.Address); trimmedAddress != "" && b.Port > 0 {
		return fmt.Sprintf("%s:%d", trimmedAddress, b.Port)
	}
	return fmt.Sprintf("unidentified-host-%p", b)
}
