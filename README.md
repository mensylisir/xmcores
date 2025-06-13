# xmcores - Cluster Management Tool

`xmcores` is a command-line tool for declaratively managing and provisioning clusters, with an initial focus on Kubernetes.

## Overview

The tool uses a YAML configuration file, inspired by Kubernetes CRD patterns, to define the desired state of a cluster. Pipelines, composed of modules and steps, then act upon this configuration to achieve the desired state.

## Current Status

This project is under active development. The current focus is on building the foundational architecture for a Kubernetes installation pipeline.

## Getting Started (Conceptual)

### Prerequisites

- Go programming environment
- Target machines accessible via SSH (for remote execution)

### Installation / Building

```bash
# Clone the repository (assuming it will be hosted)
# git clone <repository_url>
# cd xmcores
go build -o xm .
```

### Running the Cluster Installation Pipeline

The primary command for creating a Kubernetes cluster is:

```bash
./xm create cluster -f config.yaml
```

Global operational flags can also be used:
- `--log-level <level>`: Set log level (e.g., "debug", "info", "warn").
- `-v, --verbose`: Enable verbose (debug) logging.
- `--work-dir <path>`: Specify a working directory for temporary files (defaults to `./.xm_work_data`).
- `--ignore-errors`: If set, the tool will attempt to continue execution even if some non-critical steps fail.

### Configuration File (`config.yaml`)

The `config.yaml` file defines the cluster specification. Below is an example structure for `kind: Cluster`:

```yaml
apiVersion: installer.xiaoming.io/v1alpha1 # Defines the API version for this configuration
kind: Cluster # Specifies the object type as Cluster
metadata:
  name: my-k8s-cluster # Name of your cluster
spec:
  hosts:
    - name: master01
      address: 192.168.1.10       # External IP used for SSH
      internalAddress: 192.168.1.10 # Internal IP, can be same as address
      port: 22
      user: root
      # password: "your_password" # Use instead of privateKeyPath if preferred
      privateKeyPath: "/path/to/your/ssh/id_rsa" # Path to SSH private key
    - name: worker01
      address: 192.168.1.20
      internalAddress: 192.168.1.20
      port: 22
      user: root
      privateKeyPath: "/path/to/your/ssh/id_rsa"
    # Define all hosts (masters, workers, etcd nodes if external, loadbalancers) here

  roleGroups:
    etcd: # Hosts for the etcd cluster. For kubeadm stacked type, these are control-plane nodes.
      - master01
    control-plane: # Hosts for Kubernetes control plane components
      - master01
    worker: # Hosts for Kubernetes worker nodes
      - worker01
    # loadbalancer: # Optional: for HA control plane if using internal LB solution
    #   - master01 # Can be co-located or dedicated hosts

  controlPlaneEndpoint:
    loadbalancer:
      enable: true # Set to true to enable load balancing for the control plane
      type: "haproxy-keepalived" # Options: "haproxy-keepalived" (if managed by xmcores), "external" (for user-provided LB)
    domain: "k8s-api.example.local"     # DNS name for the VIP or external LB
    address: "192.168.1.100"          # VIP for managed LB, or address of external LB. Interpreted by pipeline.
                                      # Can be empty if domain is resolvable and preferred.
    port: 6443

  kubernetes:
    version: "v1.28.2" # Specify target Kubernetes version
    clusterName: "production-cluster" # A user-friendly name for the cluster
    autoRenewCerts: true
    containerManager: "containerd" # Currently focused on containerd
    type: "kubeadm" # Installation type: "kubeadm" (uses kubeadm) or "kubexm" (binary deployment, future)

  network:
    plugin: "calico" # Specify CNI plugin (e.g., calico, flannel, cilium)
    kubePodsCIDR: "10.244.0.0/16"
    kubeServiceCIDR: "10.96.0.0/12"
    blockSize: 26 # Optional: CNI-specific block size (e.g., for Calico)
    multusCNI:
      enabled: false # Set to true to enable Multus CNI

  registry: # Optional: Configuration for a private or mirrored container registry
    # type: "docker" # Type of registry, e.g., docker, harbor (for future specific handling)
    privateRegistry: "your.private.registry:5000" # Domain of your private registry
    # namespaceOverride: "my-images" # Optional: Override the default namespace for images
    auths: # Credentials for private registries
      "your.private.registry:5000":
        username: "user"
        password: "password"
    registryMirrors: # List of registry mirrors to configure on container runtime
      - "https://mirror.example.com"
    insecureRegistries: # List of registries to allow insecure (HTTP) access or skip TLS verify
      - "your.private.registry:5000" # Often needed if private registry uses self-signed certs

  etcd:
    type: "kubeadm" # Default. Etcd is stacked on control-plane nodes, managed by kubeadm.
    # Example for xmcores-managed etcd (binary deployment):
    # type: "kubexm"
    # Example for external etcd cluster:
    # type: "external"
    # endpoints:
    #   - "https://etcd1.example.com:2379"
    #   - "https://etcd2.example.com:2379"
    #   - "https://etcd3.example.com:2379"
    # caFile: "/path/to/external/etcd-ca.crt"
    # certFile: "/path/to/external/etcd-client.crt"
    # keyFile: "/path/to/external/etcd-client.key"
```

Key `spec` fields:
- `hosts`: A list of all machines involved in the cluster, with their SSH details.
- `roleGroups`: Assigns roles (like `etcd`, `control-plane`, `worker`, `loadbalancer`) to the hosts defined in `spec.hosts`.
- `controlPlaneEndpoint`: Defines how to access the Kubernetes API server. The `loadbalancer` sub-field configures if and how a load balancer is used for the control plane.
- `kubernetes`: Kubernetes-specific settings like `version`, `clusterName`, `containerManager`, and installation `type` ("kubeadm" or "kubexm").
- `network`: CNI plugin configuration (e.g., Calico, Flannel), Pod and Service CIDRs, and optional CNI-specific settings like `blockSize`.
- `registry`: Optional settings for using private or mirrored container registries, including authentication.
- `etcd`: Configuration for the etcd cluster.
    - `type`: Defines the etcd deployment strategy:
        - `"kubeadm"`: (Default) Etcd is managed by kubeadm and typically stacked on control-plane nodes.
        - `"kubexm"`: (Future) Etcd cluster to be installed and managed by `xmcores` as separate binaries on hosts in the "etcd" role.
        - `"external"`: Use a pre-existing, external etcd cluster. Requires specifying `endpoints`, and TLS certificate paths (`caFile`, `certFile`, `keyFile`).

## Development

(This section can be expanded later with details on how to contribute, build modules, etc.)

Key architectural components:
- **Pipelines (`pipeline/`)**: Orchestrate high-level operations. Pipelines are organized by resource and action (e.g., `pipeline/kubernetes/install.go` for installing Kubernetes).
- **Modules (`module/`)**: Implement specific stages within a pipeline (e.g., CNI setup, control plane initialization).
- **Tasks (`task/`)**: Encapsulate a sequence of steps to perform a specific part of a module's work.
- **Steps (`step/`)**: Smallest units of execution, performing individual actions (e.g., running a command, copying a file).
- **Runtime (`runtime/`)**: Manages execution context, including host connections and executors.
- **Configuration (`config/`)**: Handles parsing of the `ClusterConfig` YAML.

Components like Pipelines, Modules, and Tasks now follow an Init/Execute pattern. The `Init()` phase is responsible for configuration validation, setup, and assembling child components (e.g., a pipeline initializes modules, a module initializes tasks, a task initializes steps). The `Execute()` phase then performs the actual work by invoking its children or executing its own logic.
