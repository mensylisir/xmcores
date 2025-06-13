package config

import (
	"fmt"
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
    loadbalancer: # New loadbalancer section
      enable: true
      type: "haproxy-keepalived"
    domain: "lb.example.com"
    address: "192.168.1.50"
    port: 6443
  kubernetes:
    version: "v1.25.4"
    clusterName: "my-test-k8s"
    autoRenewCerts: true
    containerManager: "containerd"
    type: "kubeadm" # New field for kubernetes type
  etcd:
    type: "external"
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

	assert.Equal(t, "installer.xiaoming.io/v1alpha1", cfg.APIVersion)
	assert.Equal(t, "ClusterConfig", cfg.Kind)
	assert.Equal(t, "test-cluster", cfg.Metadata.Name)

	spec := cfg.Spec
	require.NotNil(t, spec)

	require.Len(t, spec.Hosts, 2)
	assert.Equal(t, "master1", spec.Hosts[0].Name)
	assert.Equal(t, 22, spec.Hosts[0].Port)

	require.NotNil(t, spec.RoleGroups)
	assert.Contains(t, spec.RoleGroups["etcd"], "master1")

	assert.Equal(t, "lb.example.com", spec.ControlPlaneEndpoint.Domain)
	assert.Equal(t, "192.168.1.50", spec.ControlPlaneEndpoint.Address)
	assert.Equal(t, 6443, spec.ControlPlaneEndpoint.Port)
	assert.True(t, spec.ControlPlaneEndpoint.LoadBalancer.Enable)
	assert.Equal(t, "haproxy-keepalived", spec.ControlPlaneEndpoint.LoadBalancer.Type)

	assert.Equal(t, "v1.25.4", spec.Kubernetes.Version)
	assert.True(t, spec.Kubernetes.AutoRenewCerts)
	assert.Equal(t, "containerd", spec.Kubernetes.ContainerManager)
	assert.Equal(t, "kubeadm", spec.Kubernetes.Type) // Assertion for new type field

	assert.Equal(t, "external", spec.Etcd.Type)
	require.Len(t, spec.Etcd.Endpoints, 2)
	assert.Equal(t, "https://etcd1.example.com:2379", spec.Etcd.Endpoints[0])
	assert.Equal(t, "/path/to/etcd/external/ca.crt", spec.Etcd.CAFile)
	assert.Equal(t, "/path/to/etcd/external/client.crt", spec.Etcd.CertFile)
	assert.Equal(t, "/path/to/etcd/external/client.key", spec.Etcd.KeyFile)

	require.NotNil(t, spec.Network.BlockSize)
	assert.Equal(t, 26, *spec.Network.BlockSize)
	assert.True(t, spec.Network.MultusCNI.Enabled)

	require.NotNil(t, spec.Registry.Auths)
	assert.Equal(t, "user1", spec.Registry.Auths["myprivatereg.com"].Username)
}

func TestLoadClusterConfig_ValidationErrors(t *testing.T) {
	baseValidSpecForValidationTests := `
spec:
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  kubernetes: {version: v1.25.0, containerManager: containerd}
  etcd: {type: kubeadm}
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}`

	testCases := []struct {
		name          string
		yamlContent   string
		expectedError string
	}{
		{
			name:          "APIVersion missing",
			yamlContent:   fmt.Sprintf("kind: ClusterConfig\nmetadata: {name: test}\n%s", baseValidSpecForValidationTests),
			expectedError: "apiVersion is a required field",
		},
		{
			name:          "Kind missing",
			yamlContent:   fmt.Sprintf("apiVersion: installer.xiaoming.io/v1alpha1\nmetadata: {name: test}\n%s", baseValidSpecForValidationTests),
			expectedError: "kind is a required field",
		},
		{
			name:          "Metadata.name missing",
			yamlContent:   fmt.Sprintf("apiVersion: installer.xiaoming.io/v1alpha1\nkind: ClusterConfig\nmetadata: {}\n%s", baseValidSpecForValidationTests),
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

func TestLoadClusterConfig_KubernetesType(t *testing.T) {
	tests := []struct {
		name         string
		k8sSpecYAML  string // Only the kubernetes: section
		expectedType string
	}{
		{
			name: "k8s type kubeadm",
			k8sSpecYAML: `
kubernetes:
  version: "v1.25.0" # version and containerManager are needed to be valid for this sub-test
  containerManager: "containerd"
  type: "kubeadm"
`,
			expectedType: "kubeadm",
		},
		{
			name: "k8s type kubexm",
			k8sSpecYAML: `
kubernetes:
  version: "v1.25.0"
  containerManager: "containerd"
  type: "kubexm"
`,
			expectedType: "kubexm",
		},
		{
			name: "k8s type omitted", // Type is mandatory, but test parsing before validation
			k8sSpecYAML: `
kubernetes:
  version: "v1.25.0"
  containerManager: "containerd"
  # type is omitted
`,
			expectedType: "", // Should parse as empty string if omitted
		},
	}

	baseYAMLFormat := `
apiVersion: installer.xiaoming.io/v1alpha1
kind: ClusterConfig
metadata:
  name: k8s-type-test
spec:
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  controlPlaneEndpoint: {address: "1.1.1.1", port: 6443}
  %s # k8sSpecYAML will be injected here
  etcd: {type: kubeadm}
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}
`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalYAML := fmt.Sprintf(baseYAMLFormat, tt.k8sSpecYAML)
			tmpDir, err := os.MkdirTemp("", "config_test_k8s_type_")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
			err = os.WriteFile(configFilePath, []byte(finalYAML), 0644)
			require.NoError(t, err)

			cfg, err := LoadClusterConfig(configFilePath)
			require.NoError(t, err) // Expecting no error during parsing itself
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.Spec)

			assert.Equal(t, tt.expectedType, cfg.Spec.Kubernetes.Type)
		})
	}
}

func TestLoadClusterConfig_MalformedYAML(t *testing.T) {
	sampleYAML := `
apiVersion: installer.xiaoming.io/v1alpha1
kind: ClusterConfig
metadata:
  name: test-cluster
spec:
  hosts:
    - name: master1
      address: "192.168.1.10 # Unclosed quote
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
	_, err = os.Create(configFilePath)
	require.NoError(t, err)

	cfg, err := LoadClusterConfig(configFilePath)
	require.Error(t, err)
	assert.Nil(t, cfg)
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
apiVersion: installer.xiaoming.io/v1alpha1
kind: ClusterConfig
metadata:
  name: test-cluster-no-blocksize
spec:
  network:
    plugin: "calico"
    kubePodsCIDR: "10.244.0.0/16"
    kubeServiceCIDR: "10.96.0.0/12"
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
		expectExternalData bool
		expectedEndpoints  []string
		expectedCAFile     string
		expectedCertFile   string
		expectedKeyFile    string
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
			name: "etcd type kubexm",
			etcdSpecYAML: `
etcd:
  type: "kubexm"
`,
			expectedType:       "kubexm",
			expectExternalData: false,
		},
		{
			name: "etcd type external with data",
			etcdSpecYAML: `
etcd:
  type: "external"
  endpoints: ["https://etcd.example.com:2379", "https://etcd-another.example.com:2379"]
  caFile: "/test/etcd/ca.crt"
  certFile: "/test/etcd/client.crt"
  keyFile: "/test/etcd/client.key"
`,
			expectedType:       "external",
			expectExternalData: true,
			expectedEndpoints:  []string{"https://etcd.example.com:2379", "https://etcd-another.example.com:2379"},
			expectedCAFile:     "/test/etcd/ca.crt",
			expectedCertFile:   "/test/etcd/client.crt",
			expectedKeyFile:    "/test/etcd/client.key",
		},
		{
			name:               "etcd spec omitted",
			etcdSpecYAML:       ``,
			expectedType:       "",
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
  %s
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

			assert.Equal(t, tt.expectedType, cfg.Spec.Etcd.Type)

			if tt.expectExternalData {
				assert.Equal(t, tt.expectedEndpoints, cfg.Spec.Etcd.Endpoints)
				assert.Equal(t, tt.expectedCAFile, cfg.Spec.Etcd.CAFile)
				assert.Equal(t, tt.expectedCertFile, cfg.Spec.Etcd.CertFile)
				assert.Equal(t, tt.expectedKeyFile, cfg.Spec.Etcd.KeyFile)
			} else {
				assert.Empty(t, cfg.Spec.Etcd.Endpoints)
				assert.Empty(t, cfg.Spec.Etcd.CAFile)
				assert.Empty(t, cfg.Spec.Etcd.CertFile)
				assert.Empty(t, cfg.Spec.Etcd.KeyFile)
			}
		})
	}
}

func TestLoadClusterConfig_LoadBalancerConfig(t *testing.T) {
	tests := []struct {
		name                   string
		controlPlaneYAML       string
		expectedEnable         bool
		expectedType           string
		expectError            bool
		expectedErrorContains  string
	}{
		{
			name: "LB enabled with type",
			controlPlaneYAML: `
controlPlaneEndpoint:
  loadbalancer:
    enable: true
    type: "haproxy-keepalived"
  domain: "lb.example.com"
  address: "1.2.3.4"
  port: 6443
`,
			expectedEnable: true,
			expectedType:   "haproxy-keepalived",
		},
		{
			name: "LB enabled type omitted",
			controlPlaneYAML: `
controlPlaneEndpoint:
  loadbalancer:
    enable: true
  domain: "lb.example.com"
  address: "1.2.3.4"
  port: 6443
`,
			expectedEnable: true,
			expectedType:   "",
		},
		{
			name: "LB disabled",
			controlPlaneYAML: `
controlPlaneEndpoint:
  loadbalancer:
    enable: false
    type: "haproxy-keepalived"
  domain: "lb.example.com"
  address: "1.2.3.4"
  port: 6443
`,
			expectedEnable: false,
			expectedType:   "haproxy-keepalived",
		},
		{
			name: "LB section omitted",
			controlPlaneYAML: `
controlPlaneEndpoint:
  domain: "lb.example.com"
  address: "1.2.3.4"
  port: 6443
`,
			expectedEnable: false,
			expectedType:   "",
		},
	}

	baseYAMLFormat := `
apiVersion: installer.xiaoming.io/v1alpha1
kind: ClusterConfig
metadata:
  name: lb-config-test
spec:
  hosts: [{name: m1, address: "1.1.1.1", internalAddress: "1.1.1.1", user: "u"}]
  roleGroups: {etcd: [m1], control-plane: [m1], worker: [m1]}
  %s
  kubernetes: {version: v1.25.0, containerManager: containerd}
  etcd: {type: kubeadm}
  network: {plugin: calico, kubePodsCIDR: "10.0.0.0/16", kubeServiceCIDR: "10.1.0.0/16"}
  registry: {}
`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalYAML := fmt.Sprintf(baseYAMLFormat, tt.controlPlaneYAML)
			tmpDir, err := os.MkdirTemp("", "config_test_lb_")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			configFilePath := filepath.Join(tmpDir, "cluster_config.yaml")
			err = os.WriteFile(configFilePath, []byte(finalYAML), 0644)
			require.NoError(t, err)

			cfg, err := LoadClusterConfig(configFilePath)
			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)
			require.NotNil(t, cfg.Spec)

			assert.Equal(t, tt.expectedEnable, cfg.Spec.ControlPlaneEndpoint.LoadBalancer.Enable)
			assert.Equal(t, tt.expectedType, cfg.Spec.ControlPlaneEndpoint.LoadBalancer.Type)
		})
	}
}
