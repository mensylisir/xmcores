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

- **Configuration (`config/`)**: Handles parsing of the `ClusterConfig` YAML, which defines the desired state of the target cluster.

- **Runtime (`runtime/`)**:
    - **`KubeRuntime`**: This is the central runtime object created in `main.go` after parsing the `ClusterConfig` and command-line arguments (`CliArgs`). It holds the `ClusterConfig.Spec`, `CliArgs`, processed host/role information (as `connector.Host` objects), a global logger, and operational settings like `WorkDir` and `IgnoreError`. It also manages a cache for SSH connectors to hosts. The `KubeRuntime` is passed to the pipeline's factory/constructor.
    - The runtime facilitates interaction with target hosts by providing and managing `connector.Connector` instances.

- **Pipelines (`pipeline/`)**:
    - Orchestrate high-level operations (e.g., cluster installation, upgrade, deletion).
    - Pipelines are organized by resource and action (e.g., `pipeline/kubernetes/install.go` for installing Kubernetes).
    - They are instantiated by a factory that receives the `KubeRuntime`.
    - Key `pipeline.Pipeline` interface methods:
        - `Name() string`, `Description() string`
        - `Start(logger *logrus.Entry) error`: The main entry point called by `main.go`. It orchestrates the execution of modules.
        - `RunModule(mod module.Module) *ending.ModuleResult`: Manages the lifecycle of an individual module.

- **Modules (`module/`)**:
    - Implement specific stages or capabilities within a pipeline (e.g., etcd setup, CNI installation, control-plane configuration).
    - Modules are stateful and follow a rich lifecycle managed by the pipeline (typically via `RunModule`).
    - Key `module.Module` interface methods:
        - `Name() string`, `Description() string`
        - `IsSkip(runtime *krt.KubeRuntime) (bool, error)`: Determines if the module's execution should be skipped based on current state or configuration.
        - `Default(runtime *krt.KubeRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error`: Initializes the module with its runtime context, specific configuration slice (`moduleSpec` type-asserted from `ClusterConfig.Spec`), logger, and caches.
        - `AutoAssert() error`: Performs pre-run validation and prerequisite checks.
        - `Init() error`: For the module's internal setup, primarily to assemble its constituent tasks.
        - `Run(result *ending.ModuleResult)`: Executes the core logic of the module, often by running its tasks. Populates the `ModuleResult`.
        - `Until(runtime *krt.KubeRuntime) (done bool, err error)`: For modules with asynchronous or long-running operations, indicating completion.
        - `CallPostHook(res *ending.ModuleResult) error`: Executes any registered post-execution hooks.
        - `Is() module.Type`: Returns the module type (e.g., `TaskModuleType`, `GoroutineModuleType`), indicating its execution model.
        - `Slogan() string`: A brief message logged when the module starts.
        - `AppendPostHook(hookFn module.HookFn)`: Allows adding functions to be called after `Run`.

- **Tasks (`task/`)**:
    - Encapsulate a sequence of steps to perform a specific part of a module's work (e.g., generating a certificate, installing a package on a set of hosts).
    - Tasks also follow a similar lifecycle to modules (`IsSkip`, `Default`, `AutoAssert`, `Init`, `Run`, `Until`, `Slogan`).
    - `Init()` is where a task assembles its `step.Step` components using `AddStep()`.
    - `Run()` executes these steps sequentially, managing their outcome and contributing to the parent module's `ModuleResult`.

- **Steps (`step/`)**:
    - Smallest units of execution, performing individual atomic actions (e.g., running a single command, copying a file, rendering a template).
    - Key `step.Step` interface methods:
        - `Name() string`, `Description() string`
        - `Init(rt runtime.Runtime, logger *logrus.Entry) error`: Initializes the step.
        - `Execute(rt runtime.Runtime, logger *logrus.Entry) (output string, success bool, err error)`: Performs the action.
        - `Post(rt runtime.Runtime, logger *logrus.Entry, stepExecuteErr error) error`: For cleanup actions.
    - Steps are typically executed within a Task's `Run` method.

- **Execution Flow Summary:**
    1. `main.go` (via Cobra command) parses global flags and the cluster config file path.
    2. `CliArgs` are populated from flags.
    3. `config.LoadClusterConfig` parses the YAML into `ClusterConfig`.
    4. `runtime.NewKubeRuntime` creates the `KubeRuntime` using `ClusterConfig`, `CliArgs`, and global flags.
    5. The appropriate `pipeline.Pipeline` is retrieved from the registry, passing `KubeRuntime` to its factory. The factory initializes the pipeline and its modules (calling their `Default` and `Init` methods with necessary specs and the `KubeRuntime`).
    6. `main.go` calls `selectedPipeline.Start(logger)`.
    7. Pipeline's `Start()` method iterates through its initialized modules, calling `module.IsSkip()`, then `module.Run()`, `module.Until()`, and `module.CallPostHook()`.
    8. Module's `Run()` method iterates through its initialized tasks, calling their `IsSkip()`, then `task.Run()`, etc.
    9. Task's `Run()` method (typically from `BaseTask`) executes its sequence of steps (`step.Init()`, `step.Execute()`, `step.Post()`).
    10. Results and errors are propagated up using `ending.ModuleResult`.

- **Result Handling (`pipeline/ending/result.go`):**
    - The `ending.ModuleResult` struct (with `Status`, `Message`, `Errors`) is used by module and task `Run` methods to report their outcome. This allows for standardized success/failure/skip reporting and error aggregation.
