package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleClusterConfigYAML = `
apiVersion: installer.xiaoming.io/v1alpha1 # Updated APIVersion
kind: ClusterConfig
metadata:
  name: test-cluster
spec:
  hosts:
    - name: master1
      address: "192.168.1.10"
      internalAddress: "192.168.1.10"
      port: 22
      user: "testuser"
      privateKeyPath: "/tmp/id_rsa_master1"
    - name: worker1
      address: "192.168.1.20"
      internalAddress: "192.168.1.20"
      port: 2222
      user: "testuser2"
      password: "password123"
  roleGroups:
    etcd:
      - master1
    control-plane:
      - master1
    worker:
      - worker1
  controlPlaneEndpoint:
    domain: "lb.example.com"
    address: "192.168.1.50" # Could be derived if domain is resolvable, or explicitly set
    port: 6443
  kubernetes:
    version: "v1.25.4"
    clusterName: "my-test-k8s"
    autoRenewCerts: true
    containerManager: "containerd"
  etcd:
    type: "external" # Testing external etcd type
    endpoints:
      - "https://etcd1.example.com:2379"
      - "https://etcd2.example.com:2379"
    caFile: "/path/to/etcd/external/ca.crt"
    certFile: "/path/to/etcd/external/client.crt"
    keyFile: "/path/to/etcd/external/client.key"
  network:
    plugin: "calico"
    kubePodsCIDR: "10.244.0.0/16"
    kubeServiceCIDR: "10.96.0.0/12"
    blockSize: 26
    multusCNI:
      enabled: true
  registry:
    type: "docker"
    privateRegistry: "myprivatereg.com"
    namespaceOverride: "myimages"
    auths:
      "myprivatereg.com":
        username: "user1"
        password: "password123"
      "anotherreg.com":
        username: "user2"
        password: "password456"
    registryMirrors:
      - "https://mirror1.example.com"
    insecureRegistries:
      - "insecure.example.com"
  internalLoadbalancer: "haproxy"
`

func TestLoadClusterConfig_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_success_")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
	err = os.WriteFile(configFilePath, []byte(sampleClusterConfigYAML), 0644)
	require.NoError(t, err)

	cfg, err := LoadClusterConfig(configFilePath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Assert top-level fields
	assert.Equal(t, "installer.kubesphere.io/v1alpha1", cfg.APIVersion)
	assert.Equal(t, "ClusterConfig", cfg.Kind)
	assert.Equal(t, "test-cluster", cfg.Metadata.Name)

	// Assert Spec fields
	spec := cfg.Spec
	require.NotNil(t, spec)

	// Hosts
	require.Len(t, spec.Hosts, 2)
	assert.Equal(t, "master1", spec.Hosts[0].Name)
	assert.Equal(t, "192.168.1.10", spec.Hosts[0].Address)
	assert.Equal(t, 22, spec.Hosts[0].Port)
	assert.Equal(t, "testuser", spec.Hosts[0].User)
	assert.Equal(t, "/tmp/id_rsa_master1", spec.Hosts[0].PrivateKeyPath)
	assert.Equal(t, "worker1", spec.Hosts[1].Name)
	assert.Equal(t, "192.168.1.20", spec.Hosts[1].InternalAddress)
	assert.Equal(t, 2222, spec.Hosts[1].Port)
	assert.Equal(t, "password123", spec.Hosts[1].Password)


	// RoleGroups
	require.NotNil(t, spec.RoleGroups)
	assert.Contains(t, spec.RoleGroups["etcd"], "master1")
	assert.Contains(t, spec.RoleGroups["worker"], "worker1")

	// ControlPlaneEndpoint
	assert.Equal(t, "lb.example.com", spec.ControlPlaneEndpoint.Domain)
	assert.Equal(t, "192.168.1.50", spec.ControlPlaneEndpoint.Address)
	assert.Equal(t, 6443, spec.ControlPlaneEndpoint.Port)

	// Kubernetes
	assert.Equal(t, "v1.25.4", spec.Kubernetes.Version)
	assert.Equal(t, "my-test-k8s", spec.Kubernetes.ClusterName)
	assert.True(t, spec.Kubernetes.AutoRenewCerts)
	assert.Equal(t, "containerd", spec.Kubernetes.ContainerManager)

	// Etcd
	assert.Equal(t, "external", spec.Etcd.Type)
	require.Len(t, spec.Etcd.Endpoints, 2)
	assert.Equal(t, "https://etcd1.example.com:2379", spec.Etcd.Endpoints[0])
	assert.Equal(t, "https://etcd2.example.com:2379", spec.Etcd.Endpoints[1])
	assert.Equal(t, "/path/to/etcd/external/ca.crt", spec.Etcd.CAFile)
	assert.Equal(t, "/path/to/etcd/external/client.crt", spec.Etcd.CertFile)
	assert.Equal(t, "/path/to/etcd/external/client.key", spec.Etcd.KeyFile)

	// Network
	assert.Equal(t, "calico", spec.Network.Plugin)
	assert.Equal(t, "10.244.0.0/16", spec.Network.KubePodsCIDR)
	assert.Equal(t, "10.96.0.0/12", spec.Network.KubeServiceCIDR)
	require.NotNil(t, spec.Network.BlockSize, "BlockSize should not be nil when present in YAML")
	assert.Equal(t, 26, *spec.Network.BlockSize)
	assert.True(t, spec.Network.MultusCNI.Enabled)

	// Registry
	assert.Equal(t, "docker", spec.Registry.Type)
	assert.Equal(t, "myprivatereg.com", spec.Registry.PrivateRegistry)
	assert.Equal(t, "myimages", spec.Registry.NamespaceOverride)
	require.NotNil(t, spec.Registry.Auths)
	require.Contains(t, spec.Registry.Auths, "myprivatereg.com")
	assert.Equal(t, "user1", spec.Registry.Auths["myprivatereg.com"].Username)
	assert.Equal(t, "password123", spec.Registry.Auths["myprivatereg.com"].Password)
	require.Contains(t, spec.Registry.Auths, "anotherreg.com")
	assert.Equal(t, "user2", spec.Registry.Auths["anotherreg.com"].Username)
	assert.Equal(t, "password456", spec.Registry.Auths["anotherreg.com"].Password)
	assert.Contains(t, spec.Registry.RegistryMirrors, "https://mirror1.example.com")
	assert.Contains(t, spec.Registry.InsecureRegistries, "insecure.example.com")

	// InternalLoadbalancer
	assert.Equal(t, "haproxy", spec.InternalLoadbalancer)
}

func TestLoadClusterConfig_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlContent   string
		expectedError string
	}{
		{
			name: "APIVersion missing",
			yamlContent: `kind: ClusterConfig
metadata: {name: test}
spec: # Add minimal spec to pass other validations if LoadClusterConfig becomes stricter
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  kubernetes: {version: v1.25.0, containerManager: containerd}
  etcd: {type: kubeadm}
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}`,
			expectedError: "apiVersion is a required field",
		},
		{
			name: "Kind missing",
			yamlContent: `apiVersion: installer.xiaoming.io/v1alpha1 # Updated APIVersion
metadata: {name: test}
spec:
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  kubernetes: {version: v1.25.0, containerManager: containerd}
  etcd: {type: kubeadm}
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}`,
			expectedError: "kind is a required field",
		},
		{
			name: "Metadata.name missing",
			yamlContent: `apiVersion: installer.xiaoming.io/v1alpha1 # Updated APIVersion
kind: ClusterConfig
metadata: {}
spec:
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  kubernetes: {version: v1.25.0, containerManager: containerd}
  etcd: {type: kubeadm}
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}`,
			expectedError: "metadata.name is a required field",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "config_test_validation_")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
			err = os.WriteFile(configFilePath, []byte(tc.yamlContent), 0644)
			require.NoError(t, err)

			cfg, err := LoadClusterConfig(configFilePath)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}
}


func TestLoadClusterConfig_MalformedYAML(t *testing.T) {
	sampleYAML := `
apiVersion: installer.xiaoming.io/v1alpha1 # Updated APIVersion
kind: ClusterConfig
metadata:
  name: test-cluster
spec:
  hosts:
    - name: master1
      address: "192.168.1.10 # Unclosed quote, malformed
`
	tmpDir, err := os.MkdirTemp("", "config_test_malformed_")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
	err = os.WriteFile(configFilePath, []byte(sampleYAML), 0644)
	require.NoError(t, err)

	cfg, err := LoadClusterConfig(configFilePath)
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to unmarshal config data")
}

func TestLoadClusterConfig_EmptyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_test_empty_file_")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
	_, err = os.Create(configFilePath) // Create an empty file
	require.NoError(t, err)

	cfg, err := LoadClusterConfig(configFilePath)
	require.Error(t, err)
	assert.Nil(t, cfg)
	// The exact error might vary (e.g. unmarshal error, or custom validation error)
	// For an empty file, our custom validation (e.g. apiVersion required) should catch it.
	assert.Contains(t, err.Error(), "apiVersion is a required field")
}

func TestLoadClusterConfig_FileNotFound(t *testing.T) {
	cfg, err := LoadClusterConfig("non_existent_cluster_config.yaml")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadClusterConfig_EmptyFilePath(t *testing.T) {
	cfg, err := LoadClusterConfig("")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "filePath cannot be empty")
}

func TestLoadClusterConfig_BlockSizeOmitted(t *testing.T) {
	sampleYAMLNoBlockSize := `
apiVersion: installer.kubesphere.io/v1alpha1
kind: ClusterConfig
metadata:
  name: test-cluster-no-blocksize
spec:
  network:
    plugin: "calico"
    kubePodsCIDR: "10.244.0.0/16"
    kubeServiceCIDR: "10.96.0.0/12"
    # blockSize is omitted
  # Other required fields for basic parsing to pass LoadClusterConfig validation
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  kubernetes: {version: v1.25.0, containerManager: containerd}
  etcd: {type: kubeadm}
  registry: {}
`
	tmpDir, err := os.MkdirTemp("", "config_test_no_blocksize_")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
	err = os.WriteFile(configFilePath, []byte(sampleYAMLNoBlockSize), 0644)
	require.NoError(t, err)

	cfg, err := LoadClusterConfig(configFilePath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Spec)
	require.NotNil(t, cfg.Spec.Network)
	assert.Nil(t, cfg.Spec.Network.BlockSize, "BlockSize should be nil when omitted from YAML")
}

func TestLoadClusterConfig_EtcdTypes(t *testing.T) {
	tests := []struct {
		name               string
		etcdSpecYAML       string
		expectedType       string
		expectExternalData bool // True if Endpoints, CAFile etc. should be non-empty
	}{
		{
			name: "etcd type kubeadm",
			etcdSpecYAML: `
etcd:
  type: "kubeadm"
`,
			expectedType:       "kubeadm",
			expectExternalData: false,
		},
		{
			name: "etcd type xm",
			etcdSpecYAML: `
etcd:
  type: "xm"
`,
			expectedType:       "xm",
			expectExternalData: false,
		},
		{
			name: "etcd type external with data",
			etcdSpecYAML: `
etcd:
  type: "external"
  endpoints: ["https://etcd.example.com:2379"]
  caFile: "/etc/kubernetes/pki/etcd/ca.crt"
  certFile: "/etc/kubernetes/pki/etcd/client.crt"
  keyFile: "/etc/kubernetes/pki/etcd/client.key"
`,
			expectedType:       "external",
			expectExternalData: true,
		},
		{
			name: "etcd spec omitted", // Test when the whole etcd block is missing
			etcdSpecYAML: ``, // No etcd: block
			expectedType: "", // Default Go string value
			expectExternalData: false,
		},
	}

	baseYAMLFormat := `
apiVersion: installer.xiaoming.io/v1alpha1
kind: ClusterConfig
metadata:
  name: etcd-type-test
spec:
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  kubernetes: {version: v1.25.0, containerManager: containerd}
  %s # etcdSpecYAML will be injected here
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}
`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalYAML := fmt.Sprintf(baseYAMLFormat, tt.etcdSpecYAML)

			tmpDir, err := os.MkdirTemp("", "config_test_etcd_types_")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
			err = os.WriteFile(configFilePath, []byte(finalYAML), 0644)
			require.NoError(t, err)

			cfg, err := LoadClusterConfig(configFilePath)
			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.Spec)
			// Note: If etcdSpecYAML is empty, cfg.Spec.Etcd will be a zero-value EtcdSpec struct.

			assert.Equal(t, tt.expectedType, cfg.Spec.Etcd.Type)

			if tt.expectExternalData {
				assert.NotEmpty(t, cfg.Spec.Etcd.Endpoints, "Endpoints should not be empty for external etcd with data")
				assert.Equal(t, "https://etcd.example.com:2379", cfg.Spec.Etcd.Endpoints[0])
				assert.Equal(t, "/etc/kubernetes/pki/etcd/ca.crt", cfg.Spec.Etcd.CAFile)
				assert.Equal(t, "/etc/kubernetes/pki/etcd/client.crt", cfg.Spec.Etcd.CertFile)
				assert.Equal(t, "/etc/kubernetes/pki/etcd/client.key", cfg.Spec.Etcd.KeyFile)
			} else {
				assert.Empty(t, cfg.Spec.Etcd.Endpoints, "Endpoints should be empty for etcd type '%s'", tt.expectedType)
				assert.Empty(t, cfg.Spec.Etcd.CAFile, "CAFile should be empty for etcd type '%s'", tt.expectedType)
				assert.Empty(t, cfg.Spec.Etcd.CertFile, "CertFile should be empty for etcd type '%s'", tt.expectedType)
				assert.Empty(t, cfg.Spec.Etcd.KeyFile, "KeyFile should be empty for etcd type '%s'", tt.expectedType)
			}
		})
	}
}
