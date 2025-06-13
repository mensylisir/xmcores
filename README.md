# xmcores - Cluster Management Tool

`xmcores` is a command-line tool for declaratively managing and provisioning clusters, with an initial focus on Kubernetes.

## Overview

The tool uses a YAML configuration file, inspired by Kubernetes CRD patterns, to define the desired state of a cluster. Pipelines, composed of modules and tasks (which in turn are composed of steps), then act upon this configuration to achieve the desired state.

## Current Status

This project is under active development. The current focus is on building the foundational architecture for a Kubernetes installation pipeline and refining core component lifecycles.

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
- `--ignore-errors`: If set, the tool will attempt to continue execution even if some non-critical steps or module/task failures occur.
- `--artifact <path>`: Path to an offline artifact (e.g., tarball).
- `--skip-push-images`: Skip pushing images to a private registry.
- `--deploy-local-storage`: Deploy a local storage provisioner.
- `--install-packages`: Allow installation of OS packages (default true).
- `--skip-pull-images`: Skip pulling images (assume pre-loaded).
- `--security-enhancement`: Apply additional security enhancements.
- `--skip-install-addons`: Skip installing default addons.


### Configuration File (`config.yaml`)

The `config.yaml` file defines the cluster specification. Below is an example structure:

```yaml
apiVersion: installer.xiaoming.io/v1alpha1
kind: ClusterConfig # Changed from 'Cluster' to 'ClusterConfig' to match struct
metadata:
  name: my-k8s-cluster
spec:
  hosts:
    - name: master01
      address: 192.168.1.10
      internalAddress: 192.168.1.10
      port: 22
      user: root
      privateKeyPath: "/path/to/your/ssh/id_rsa"
    - name: worker01
      address: 192.168.1.20
      internalAddress: 192.168.1.20
      port: 22
      user: root
      privateKeyPath: "/path/to/your/ssh/id_rsa"

  roleGroups:
    etcd:
      - master01
    control-plane:
      - master01
    worker:
      - worker01
    # loadbalancer: # Optional
    #   - master01

  controlPlaneEndpoint:
    loadbalancer:
      enable: true
      type: "haproxy-keepalived" # Options: "haproxy-keepalived", "external"
    domain: "k8s-api.example.local"
    address: "192.168.1.100" # VIP or External LB IP
    port: 6443

  kubernetes:
    version: "v1.28.2"
    clusterName: "production-cluster"
    autoRenewCerts: true
    containerManager: "containerd"
    type: "kubeadm" # Options: "kubeadm", "kubexm" (future)

  network:
    plugin: "calico"
    kubePodsCIDR: "10.244.0.0/16"
    kubeServiceCIDR: "10.96.0.0/12"
    blockSize: 26 # Optional for Calico
    multusCNI:
      enabled: false

  registry:
    privateRegistry: "your.private.registry:5000"
    auths:
      "your.private.registry:5000":
        username: "user"
        password: "password"
    # Other registry fields like type, namespaceOverride, registryMirrors, insecureRegistries

  etcd:
    type: "kubeadm" # Options: "kubeadm", "kubexm", "external"
    # For "external" type, specify:
    # endpoints: ["https://etcd1:2379"]
    # caFile: "/path/to/etcd-ca.crt"
    # certFile: "/path/to/etcd-client.crt"
    # keyFile: "/path/to/etcd-client.key"
```

Key `spec` fields:
- `hosts`: A list of all machines involved in the cluster.
- `roleGroups`: Assigns roles (e.g., `etcd`, `control-plane`, `worker`, `loadbalancer`) to hosts.
- `controlPlaneEndpoint`: Defines access to the Kubernetes API, including optional load balancer configuration via `loadbalancer.enable` and `loadbalancer.type`.
- `kubernetes`: Settings like `version`, `clusterName`, `containerManager`, and installation `type` ("kubeadm" or "kubexm").
- `network`: CNI configuration, CIDRs, and `blockSize`.
- `registry`: Optional private/mirrored registry settings.
- `etcd`: Defines etcd deployment: `"kubeadm"` (stacked), `"kubexm"` (xmcores-managed), or `"external"`.

## Development - Architectural Overview

The `xmcores` tool follows a layered architecture to manage cluster operations.

- **Configuration (`config/`)**:
    - The primary configuration is `config.ClusterConfig`, parsed from the user-provided YAML file (e.g., `config.yaml`).
    - **Loading:** In `main.go`, `config.NewLoader(filePath).Load()` reads and unmarshals the YAML into a raw `*config.ClusterConfig`.
    - **Defaulting:** `config.SetDefaultClusterSpec()` is then called. This function processes the raw `ClusterConfig.Spec`, applies default values for various fields (e.g., Kubernetes version, network plugin, CIDRs), and resolves host definitions from `Spec.Hosts` into `connector.Host` objects, assigning roles based on `Spec.RoleGroups`.

- **Runtime (`runtime/`)**:
    - **`CliArgs` (`runtime/cli_args.go`):** A struct holding processed command-line arguments that can influence behavior globally (e.g., `--skip-push-images`, `--artifact`).
    - **`ClusterRuntime` (`runtime/cluster_runtime.go`):** This is the central runtime object for a given cluster operation.
        - **Contents:** It holds the defaulted `*config.ClusterSpec` (as `ClusterRuntime.Cluster`), the populated `*CliArgs` (`ClusterRuntime.Arg`), operational parameters like `WorkDir`, `IgnoreErr` (from `CliArgs.IgnoreErr`), `Debug` (from `CliArgs.Debug`), a scoped `logrus.Entry` for logging, and collections of processed `connector.Host` objects (`AllHosts`, `RoleHosts`). It also manages an SSH `ConnectorCache` and a `PipelineCache`.
        - **Initialization:** An instance of `ClusterRuntime` is created in `main.go` by `runtime.NewClusterRuntime()`, which takes the raw `ClusterConfig`, `CliArgs`, global operational flags, and a base logger. `NewClusterRuntime` itself calls `config.SetDefaultClusterSpec`.
    - **`BaseRuntime` (`connector/base_runtime.go`):** Embedded by `ClusterRuntime` to provide common functionalities like host list management, role map storage, and connector caching via a `Dialer`.

- **Pipelines (`pipeline/`)**:
    - Orchestrate high-level, end-to-end operations (e.g., "cluster-install").
    - Reside in packages like `pipeline/kubernetes/`. Example: `install.go` defines the `InstallPipeline`.
    - **Factory & Registry:** Pipelines are instantiated via factories (`pipeline.PipelineFactory`) of type `func(cr *runtime.ClusterRuntime) (pipeline.Pipeline, error)`. These factories are registered in `pipeline.DefaultRegistry`. `main.go` calls `pipeline.GetPipeline(name, clusterRuntime)` to get an initialized pipeline instance. The factory is responsible for creating the pipeline struct and initializing its modules.
    - **`pipeline.ConcretePipeline`**: A base struct that can be embedded by concrete pipelines to provide common fields like `NameField`, `DescriptionField`, `Runtime (*krt.ClusterRuntime)`, `Modules ([]module.Module)`, and `ModulePostHooks`. It also provides helper methods for cache management (delegating to `ClusterRuntime`).
    - **`Pipeline` Interface (`pipeline/interface.go`):**
        - `Name() string`, `Description() string`
        - `Start(logger *logrus.Entry) error`: The main entry point called by `main.go`. It orchestrates the execution of its configured modules.
        - `RunModule(mod module.Module) *ending.ModuleResult`: Called by `Start()`, this method manages the full lifecycle of an individual module.

- **Modules (`module/`)**:
    - Implement specific stages or capabilities within a pipeline (e.g., etcd setup, CNI installation).
    - **`Module` Interface (`module/interface.go`):** Defines a rich lifecycle:
        - `Name() string`, `Description() string`, `Slogan() string`
        - `Is() Type`: Returns module type (e.g., `TaskModuleType`, `GoroutineModuleType`).
        - `IsSkip(runtime *runtime.ClusterRuntime) (bool, error)`: Conditional execution.
        - `Default(runtime *runtime.ClusterRuntime, moduleSpec interface{}, pipelineCache interface{}, moduleCache interface{}) error`: Receives `ClusterRuntime`, its specific configuration slice (type-asserted from `ClusterConfig.Spec`), and caches. Sets up logger and stores context.
        - `AutoAssert(runtime *runtime.ClusterRuntime) error`: Prerequisite checks.
        - `Init() error`: Internal setup, primarily for assembling constituent tasks.
        - `Run(result *ending.ModuleResult)`: Core logic execution, populates `ModuleResult`.
        - `Until(runtime *runtime.ClusterRuntime) (bool, error)`: For asynchronous/long-running operations.
        - `CallPostHook(res *ending.ModuleResult) error`: Executes post-execution hooks.
        - `AppendPostHook(hookFn module.HookFn)`: Adds a post-execution hook.

- **Tasks (`task/`)**:
    - Represent a sequence of steps to achieve a specific part of a module's work.
    - **`Task` Interface (`task/interface.go`):** Mirrors the module lifecycle where applicable (`Name`, `Description`, `Slogan`, `IsSkip`, `Default`, `AutoAssert`, `Init`, `Run`, `Until`).
    - **`BaseTask` (`task/base_task.go`):** Provides a common implementation for `Task`.
        - `Default()` receives context and `taskSpec` from the parent Module.
        - `Init()` assembles and initializes `step.Step` instances using `AddStep()`.
        - `Run()` executes steps sequentially, calling their `Init` (if not already done), `Execute`, and `Post`, and translates outcomes into the `ending.ModuleResult` passed from the module.
    - `AddStep(s step.Step)` and `Steps() []step.Step` are part of the interface for step management.

- **Steps (`step/`)**:
    - The smallest, most granular units of execution, performing atomic actions.
    - **`Step` Interface (`step/interface.go`):**
        - `Name() string`, `Description() string`
        - `Init(rt *runtime.ClusterRuntime, logger *logrus.Entry) error`: Prepares the step.
        - `Execute(rt *runtime.ClusterRuntime, logger *logrus.Entry) (output string, success bool, err error)`: Performs the action.
        - `Post(rt *runtime.ClusterRuntime, logger *logrus.Entry, stepExecuteErr error) error`: For cleanup.
    - Example: `runcmd.RunCommandStep` executes shell commands, potentially on `TargetHosts`.

- **Result Handling (`pipeline/ending/result.go`):**
    - `ending.ModuleResult` (with `Status`, `Message`, `Errors`) is used by Module and Task `Run` methods for standardized reporting of outcomes (Success, Failed, Skipped, Pending) and error aggregation.

- **Overall Execution Flow:**
    1. `main.go` (Cobra command) parses flags, including the `-f <config.yaml>` path.
    2. `CliArgs` struct is populated from command-line flags.
    3. `config.NewLoader(configFilePath).Load()` parses the YAML into a raw `*config.ClusterConfig`.
    4. `runtime.NewClusterRuntime(rawCfg, cliArgs, ...)` is called. This constructor:
        a. Internally calls `config.SetDefaultClusterSpec()` to apply defaults to the spec and process `Spec.Hosts` into `connector.Host` objects with roles assigned.
        b. Initializes an embedded `connector.BaseRuntime` with core functionalities (dialer, host/role lists, connector cache).
        c. Returns an initialized `*runtime.ClusterRuntime`.
    5. `main.go` retrieves the target `pipeline.Pipeline` (e.g., "cluster-install") from the `pipeline.DefaultRegistry` using `pipeline.GetPipeline(pipelineName, clusterRuntime)`. The factory receives the `clusterRuntime` and uses it to create and initialize the pipeline instance, including setting up its modules (calling module `Default` and `Init`).
    6. `main.go` calls `selectedPipeline.Start(logger)`.
    7. The pipeline's `Start()` method orchestrates its list of modules. For each module, it calls `pipeline.RunModule(module)`.
    8. `RunModule()` manages the module's lifecycle: `IsSkip()`, `Default()` (already called by factory), `AutoAssert()`, `Init()` (already called by factory), `Run()`, `Until()`, and `CallPostHook()`.
    9. A module's `Run()` method, if task-based, manages its tasks' lifecycle similarly (IsSkip, Default, AutoAssert, Init, Run, Until).
    10. A task's `Run()` method (often via `BaseTask`) executes its sequence of `step.Step`s, calling their `Init()`, `Execute()`, and `Post()` methods.
    11. Execution status and errors are propagated upwards using `ending.ModuleResult`.
```
