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

The `config.yaml` file defines the cluster specification. Below is an example structure for `Kind: Cluster`:

```yaml
apiVersion: xms.xiaoming.io/v1 # Example API version
kind: Cluster
metadata:
  name: my-k8s-cluster
spec:
  hosts:
    - name: master01
      address: 192.168.1.10
      internalAddress: 192.168.1.10 # Or specific internal IP
      port: 22
      user: root
      # password: "your_password" # Alternatively, use privateKeyPath
      privateKeyPath: "/path/to/your/ssh/key"
    - name: worker01
      address: 192.168.1.20
      # ... other host details ...
    # Define all hosts (masters, workers, etcd, loadbalancers) here

  roleGroups:
    etcd:
      - master01 # Example: etcd colocated on master01
    control-plane:
      - master01
    worker:
      - worker01
    loadbalancer: # Optional: for HA control plane
      - master01 # Example: LB also on master01, or dedicated hosts

  controlPlaneEndpoint:
    # internalLoadbalancer: haproxy # If using the loadbalancer role
    domain: "k8s-api.your.domain" # DNS name for the API server
    address: "192.168.1.10" # VIP or IP of the primary control plane node if no LB
    port: 6443

  kubernetes:
    version: "v1.28.2" # Specify target Kubernetes version
    clusterName: "production-cluster"
    autoRenewCerts: true
    containerManager: "containerd" # Currently focused on containerd

  network:
    plugin: "calico" # Specify CNI plugin (e.g., calico, flannel)
    kubePodsCIDR: "10.244.0.0/16"
    kubeServiceCIDR: "10.96.0.0/12"
    blockSize: 26 # Optional: CNI-specific block size (e.g., for Calico)
    multusCNI:
      enabled: false

  registry: # Optional: Configuration for a private registry
    # type: "harbor"
    # privateRegistry: "your.private.registry:5000"
    # auths:
    #   "your.private.registry:5000":
    #     username: "user"
    #     password: "password"
    # insecureRegistries:
    #   - "your.private.registry:5000"

  # etcd: # Optional: Specific etcd settings if not using default embedded
    # type: "external" # or "stacked" (default)
    # ... etcd specific params ...
```

Key `spec` fields:
- `hosts`: A list of all machines involved in the cluster, with their SSH details.
- `roleGroups`: Assigns roles (like `etcd`, `control-plane`, `worker`, `loadbalancer`) to the hosts defined in `spec.hosts`.
- `controlPlaneEndpoint`: Defines how to access the Kubernetes API server.
- `kubernetes`: Kubernetes-specific settings like version and container manager.
- `network`: CNI plugin configuration, CIDRs, and optional `blockSize`.
- `registry`: Optional settings for using a private container registry.

## Development

(This section can be expanded later with details on how to contribute, build modules, etc.)

Key architectural components:
- **Pipelines (`pipeline/`)**: Orchestrate high-level operations (e.g., cluster installation).
- **Modules (`module/`)**: Implement specific stages within a pipeline (e.g., CNI setup, control plane initialization).
- **Steps (`step/`)**: Smallest units of execution, performing individual actions.
- **Runtime (`runtime/`)**: Manages execution context, including host connections and executors.
- **Configuration (`config/`)**: Handles parsing of the `ClusterConfig` YAML.
