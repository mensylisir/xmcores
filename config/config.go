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
	RoleGroups           map[string][]string      `yaml:"roleGroups"`
	ControlPlaneEndpoint ControlPlaneEndpointSpec `yaml:"controlPlaneEndpoint"`
	Kubernetes           KubernetesSpec           `yaml:"kubernetes"`
	Etcd                 EtcdSpec                 `yaml:"etcd"`
	Network              NetworkSpec              `yaml:"network"`
	Registry             RegistrySpec             `yaml:"registry"`
	// InternalLoadbalancer string field was here, removed as it's now part of ControlPlaneEndpoint.LoadBalancer
}

// HostSpec defines the configuration for a single host.
type HostSpec struct {
	Name            string `yaml:"name"`
	Address         string `yaml:"address"`
	InternalAddress string `yaml:"internalAddress"`
	Port            int    `yaml:"port,omitempty"`
	User            string `yaml:"user"`
	Password        string `yaml:"password,omitempty"`
	PrivateKeyPath  string `yaml:"privateKeyPath,omitempty"`
}

// LoadBalancerConfigSpec defines the configuration for a load balancer for the control plane.
type LoadBalancerConfigSpec struct {
	Enable bool   `yaml:"enable"`           // If true, indicates load balancing for the control plane is enabled.
	Type   string `yaml:"type,omitempty"` // e.g., "haproxy-keepalived" (if managed by this tool), "external" (if user-provided)
}

// ControlPlaneEndpointSpec defines the control plane endpoint.
type ControlPlaneEndpointSpec struct {
	Domain       string                 `yaml:"domain,omitempty"`
	Address      string                 `yaml:"address"` // VIP if internal LB, or IP of external LB, or IP of single CP node
	Port         int                    `yaml:"port"`
	LoadBalancer LoadBalancerConfigSpec `yaml:"loadbalancer,omitempty"`
}

// KubernetesSpec defines Kubernetes specific configurations.
type KubernetesSpec struct {
	Version          string `yaml:"version"`
	ClusterName      string `yaml:"clusterName,omitempty"`
	AutoRenewCerts   bool   `yaml:"autoRenewCerts,omitempty"`
	ContainerManager string `yaml:"containerManager"`
	Type             string `yaml:"type"` // Installation type: "kubeadm", "kubexm" (binary)
}

// EtcdSpec defines etcd configuration.
type EtcdSpec struct {
	Type      string   `yaml:"type"` // Expected values: "kubeadm" (stacked on control plane), "external" (user-provided), "kubexm" (managed by xmcores)
	Endpoints []string `yaml:"endpoints,omitempty"` // Required for type "external"
	CAFile    string   `yaml:"caFile,omitempty"`    // Required for type "external" (TLS)
	CertFile  string   `yaml:"certFile,omitempty"`  // Required for type "external" (TLS)
	KeyFile   string   `yaml:"keyFile,omitempty"`   // Required for type "external" (TLS)
}

// NetworkSpec defines network configuration.
type NetworkSpec struct {
	Plugin          string        `yaml:"plugin"`
	KubePodsCIDR    string        `yaml:"kubePodsCIDR"`
	KubeServiceCIDR string        `yaml:"kubeServiceCIDR"`
	BlockSize       *int          `yaml:"blockSize,omitempty"`
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
}

// RegistrySpec defines container registry configurations.
type RegistrySpec struct {
	Type              string                             `yaml:"type,omitempty"`
	Auths             map[string]RegistryAuthCredentials `yaml:"auths,omitempty"`
	PrivateRegistry   string                             `yaml:"privateRegistry,omitempty"`
	NamespaceOverride string                             `yaml:"namespaceOverride,omitempty"`
	RegistryMirrors   []string                           `yaml:"registryMirrors,omitempty"`
	InsecureRegistries []string                           `yaml:"insecureRegistries,omitempty"`
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
