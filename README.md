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
apiVersion: installer.xiaoming.io/v1alpha1 # Updated API version
kind: Cluster # Defines the object type
metadata:
  name: my-k8s-cluster # Name of your cluster
spec:
  hosts:
    - name: master01
      address: 192.168.1.10       # External IP
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
    # domain: "k8s-api.example.local" # DNS name for the API server (recommended)
    address: "192.168.1.10" # VIP or IP of the (first) control plane node if no LB/domain
    port: 6443

  kubernetes:
    version: "v1.28.2" # Specify target Kubernetes version
    clusterName: "production-cluster" # A user-friendly name for the cluster
    autoRenewCerts: true
    containerManager: "containerd" # Currently focused on containerd

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
    # namespaceOverride: "my-images" # Optional: Override the default namespace for images (e.g. k8s.gcr.io -> your.private.registry/my-images)
    auths: # Credentials for private registries
      "your.private.registry:5000":
        username: "user"
        password: "password"
    registryMirrors: # List of registry mirrors to configure on container runtime
      - "https://mirror.example.com"
    insecureRegistries: # List of registries to allow insecure (HTTP) access or skip TLS verify
      - "your.private.registry:5000" # Often needed if private registry uses self-signed certs

  etcd:
    type: "kubeadm" # Default and recommended for most cases. Etcd is stacked on control-plane nodes.
    # For an external etcd cluster, use type "external" and provide details:
    # type: "external"
    # endpoints:
    #   - "https://etcd1.example.com:2379"
    #   - "https://etcd2.example.com:2379"
    #   - "https://etcd3.example.com:2379"
    # caFile: "/path/to/external/etcd-ca.crt"
    # certFile: "/path/to/external/etcd-client.crt"
    # keyFile: "/path/to/external/etcd-client.key"
    # The "xm" type for etcd is reserved for future use where xmcores might manage a separate etcd cluster.

  # internalLoadbalancer: "haproxy" # Optional: if xmcores should deploy an internal LB (like haproxy+keepalived)
                                 # If empty, assumes an external LB or a single control-plane node setup.
```

Key `spec` fields:
- `hosts`: A list of all machines involved in the cluster, with their SSH details.
- `roleGroups`: Assigns roles (like `etcd`, `control-plane`, `worker`, `loadbalancer`) to the hosts defined in `spec.hosts`.
- `controlPlaneEndpoint`: Defines how to access the Kubernetes API server (via domain or direct IP/VIP).
- `kubernetes`: Kubernetes-specific settings like version, cluster name, and container manager.
- `network`: CNI plugin configuration (e.g., Calico, Flannel), Pod and Service CIDRs, and optional CNI-specific settings like `blockSize`.
- `registry`: Optional settings for using private or mirrored container registries, including authentication.
- `etcd`: Configuration for the etcd cluster.
    - `type`: Defines the etcd deployment strategy:
        - `"kubeadm"`: (Default) Etcd is managed by kubeadm and typically stacked on control-plane nodes.
        - `"xm"`: (Future) Etcd cluster managed by `xmcores` itself (deployed as separate binaries).
        - `"external"`: Use a pre-existing, external etcd cluster. Requires specifying `endpoints`, and TLS certificate paths (`caFile`, `certFile`, `keyFile`).

## Development

(This section can be expanded later with details on how to contribute, build modules, etc.)

Key architectural components:
- **Pipelines (`pipeline/`)**: Orchestrate high-level operations (e.g., cluster installation).
- **Modules (`module/`)**: Implement specific stages within a pipeline (e.g., CNI setup, control plane initialization).
- **Steps (`step/`)**: Smallest units of execution, performing individual actions.
- **Runtime (`runtime/`)**: Manages execution context, including host connections and executors.
- **Configuration (`config/`)**: Handles parsing of the `ClusterConfig` YAML.
