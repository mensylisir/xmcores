package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ClusterConfig is the top-level configuration structure.
type ClusterConfig struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   MetadataSpec `yaml:"metadata"`
	Spec       ClusterSpec  `yaml:"spec"`
}

// MetadataSpec defines metadata for the cluster configuration.
type MetadataSpec struct {
	Name string `yaml:"name"`
}

// ClusterSpec defines the main configuration details for the cluster.
type ClusterSpec struct {
	Hosts                []HostSpec               `yaml:"hosts"`
	RoleGroups           map[string][]string      `yaml:"roleGroups"` // e.g., "etcd": ["master1", "master2"], "worker": ["worker1"]
	ControlPlaneEndpoint ControlPlaneEndpointSpec `yaml:"controlPlaneEndpoint"`
	Kubernetes           KubernetesSpec           `yaml:"kubernetes"`
	Etcd                 EtcdSpec                 `yaml:"etcd"`
	Network              NetworkSpec              `yaml:"network"`
	Registry             RegistrySpec             `yaml:"registry"`
	InternalLoadbalancer string                   `yaml:"internalLoadbalancer,omitempty"`
}

// HostSpec defines the configuration for a single host.
type HostSpec struct {
	Name            string `yaml:"name"`
	Address         string `yaml:"address"`
	InternalAddress string `yaml:"internalAddress"`
	Port            int    `yaml:"port,omitempty"` // Default to 22 if not specified by user
	User            string `yaml:"user"`
	Password        string `yaml:"password,omitempty"`
	PrivateKeyPath  string `yaml:"privateKeyPath,omitempty"`
	// Arch string `yaml:"arch,omitempty"` // Can be added if needed per host
}

// ControlPlaneEndpointSpec defines the control plane endpoint.
type ControlPlaneEndpointSpec struct {
	Domain  string `yaml:"domain,omitempty"` // e.g., lb.kubesphere.local
	Address string `yaml:"address"`          // IP address if no domain
	Port    int    `yaml:"port"`             // e.g., 6443
}

// KubernetesSpec defines Kubernetes specific configurations.
type KubernetesSpec struct {
	Version          string `yaml:"version"`
	ClusterName      string `yaml:"clusterName,omitempty"` // Optional, metadata.name can be primary
	AutoRenewCerts   bool   `yaml:"autoRenewCerts,omitempty"`
	ContainerManager string `yaml:"containerManager"` // e.g., containerd, docker
}

// EtcdSpec defines etcd configuration.
type EtcdSpec struct {
	Type      string   `yaml:"type"` // "kubeadm", "xm", "external"
	Endpoints []string `yaml:"endpoints,omitempty"` // For type "external"
	CAFile    string   `yaml:"caFile,omitempty"`    // For type "external"
	CertFile  string   `yaml:"certFile,omitempty"`  // For type "external"
	KeyFile   string   `yaml:"keyFile,omitempty"`   // For type "external"
}

// NetworkSpec defines network configuration.
type NetworkSpec struct {
	Plugin          string        `yaml:"plugin"`    // e.g., calico, flannel, cilium
	KubePodsCIDR    string        `yaml:"kubePodsCIDR"`
	KubeServiceCIDR string        `yaml:"kubeServiceCIDR"`
	BlockSize       *int          `yaml:"blockSize,omitempty"` // Pointer to distinguish between 0 and not set
	MultusCNI       MultusCNISpec `yaml:"multusCNI,omitempty"`
}

// MultusCNISpec defines configuration for Multus CNI.
type MultusCNISpec struct {
	Enabled bool `yaml:"enabled"`
}

// RegistryAuthCredentials defines username and password for a registry.
type RegistryAuthCredentials struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// Auth string `yaml:"auth,omitempty"` // Base64 encoded user:pass, alternative
}

// RegistrySpec defines container registry configurations.
type RegistrySpec struct {
	Type              string                             `yaml:"type,omitempty"` // e.g., docker, harbor
	Auths             map[string]RegistryAuthCredentials `yaml:"auths,omitempty"`
	PrivateRegistry   string                             `yaml:"privateRegistry,omitempty"`   // e.g., dockerhub.kubekey.local
	NamespaceOverride string                             `yaml:"namespaceOverride,omitempty"` // e.g., kubekey
	RegistryMirrors   []string                           `yaml:"registryMirrors,omitempty"`   // e.g., ["https://docker.mirrors.ustc.edu.cn"]
	InsecureRegistries []string                           `yaml:"insecureRegistries,omitempty"`// e.g., ["my-private-registry.com"]
}

// LoadClusterConfig reads a YAML file from the given path and unmarshals it
// into a ClusterConfig struct.
func LoadClusterConfig(filePath string) (*ClusterConfig, error) {
	if filePath == "" {
		return nil, fmt.Errorf("filePath cannot be empty")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", filePath, err)
	}

	var cfg ClusterConfig
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config data from '%s': %w", filePath, err)
	}

	// Basic validation example (can be expanded)
	if cfg.APIVersion == "" {
		return nil, fmt.Errorf("apiVersion is a required field in the config file '%s'", filePath)
	}
	if cfg.Kind == "" {
		return nil, fmt.Errorf("kind is a required field in the config file '%s'", filePath)
	}
	if cfg.Metadata.Name == "" {
		return nil, fmt.Errorf("metadata.name is a required field in the config file '%s'", filePath)
	}

	return &cfg, nil
}
