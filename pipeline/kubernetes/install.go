package kubernetes

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/pipeline/ending" // For ModuleResult in RunModule
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"

	// Module imports for instantiation in factory
	"github.com/mensylisir/xmcores/module/containerruntime"
	"github.com/mensylisir/xmcores/module/etcd"
	controlplane "github.com/mensylisir/xmcores/module/kubernetes/controlplane"
	"github.com/mensylisir/xmcores/module/kubernetes/worker"
	"github.com/mensylisir/xmcores/module/loadbalancer"
	"github.com/mensylisir/xmcores/module/network"
)

// InstallPipeline defines the structure for the Kubernetes installation pipeline.
type InstallPipeline struct {
	NameField        string
	DescriptionField string
	KubeRuntime      *runtime.KubeRuntime
	modules          []module.Module
}

// NewInstallPipelineFactory creates a new instance of InstallPipeline.
// It accepts KubeRuntime and initializes the pipeline with it, including module setup.
func NewInstallPipelineFactory(kr *runtime.KubeRuntime) (pipeline.Pipeline, error) {
	if kr == nil {
		return nil, fmt.Errorf("KubeRuntime cannot be nil for NewInstallPipelineFactory")
	}
	if kr.Cluster == nil { // ClusterSpec is within KubeRuntime
		return nil, fmt.Errorf("KubeRuntime.Cluster (ClusterSpec) cannot be nil")
	}

	p := &InstallPipeline{
		NameField:        "cluster-install",
		DescriptionField: "Installs a Kubernetes cluster based on the ClusterConfig specification.",
		KubeRuntime:      kr,
		modules:          make([]module.Module, 0),
	}

	logger := kr.Log.WithField("pipeline_factory", p.NameField)
	logger.Info("Initializing modules for InstallPipeline...")

	var currentModule module.Module
	var err error

	// Load Balancer Module (Conditional)
	if kr.Cluster.ControlPlaneEndpoint.LoadBalancer.Enable {
		if kr.Cluster.ControlPlaneEndpoint.LoadBalancer.Type == "haproxy-keepalived" {
			lbMod := &loadbalancer.HAProxyKeepalivedModule{}
			currentModule = lbMod
			if err = currentModule.Init(kr, &kr.Cluster.ControlPlaneEndpoint, logger.WithField("module", currentModule.Name())); err != nil {
				return nil, fmt.Errorf("factory: failed to init module %s: %w", currentModule.Name(), err)
			}
			p.modules = append(p.modules, currentModule)
			logger.Infof("Factory: Module %s initialized.", currentModule.Name())
		} else if kr.Cluster.ControlPlaneEndpoint.LoadBalancer.Type == "external" {
			logger.Info("Factory: External load balancer is configured. Skipping internal LoadBalancerModule.")
		} else if kr.Cluster.ControlPlaneEndpoint.LoadBalancer.Type != "" { // Non-empty but unknown type
			logger.Warnf("Factory: Load balancer enabled but type '%s' is unknown or not supported for automatic setup.", kr.Cluster.ControlPlaneEndpoint.LoadBalancer.Type)
		}
	} else {
		logger.Info("Factory: Internal load balancer setup is disabled via config.")
	}

	// Etcd Module
	etcdMod := &etcd.EtcdModule{}
	currentModule = etcdMod
	if err = currentModule.Init(kr, kr.Cluster.Etcd, logger.WithField("module", currentModule.Name())); err != nil { // Pass EtcdSpec directly
		return nil, fmt.Errorf("factory: failed to init module %s: %w", currentModule.Name(), err)
	}
	p.modules = append(p.modules, currentModule)
	logger.Infof("Factory: Module %s initialized.", currentModule.Name())

	// Container Runtime Module
	if kr.Cluster.Kubernetes.ContainerManager == "containerd" {
		containerdMod := &containerruntime.ContainerdModule{}
		currentModule = containerdMod
		if err = currentModule.Init(kr, kr.Cluster.Kubernetes, logger.WithField("module", currentModule.Name())); err != nil { // Pass KubernetesSpec
			return nil, fmt.Errorf("factory: failed to init module %s: %w", currentModule.Name(), err)
		}
		p.modules = append(p.modules, currentModule)
		logger.Infof("Factory: Module %s initialized.", currentModule.Name())
	} else {
		return nil, fmt.Errorf("factory: unsupported container manager: %s", kr.Cluster.Kubernetes.ContainerManager)
	}

	// Control Plane Module
	cpMod := &controlplane.ControlPlaneModule{}
	currentModule = cpMod
	controlPlaneModuleSpec := controlplane.ControlPlaneModuleSpec{ // Use the defined struct from the module
		Kubernetes:           *kr.Cluster.Kubernetes,
		ControlPlaneEndpoint: *kr.Cluster.ControlPlaneEndpoint,
	}
	if err = currentModule.Init(kr, &controlPlaneModuleSpec, logger.WithField("module", currentModule.Name())); err != nil {
		return nil, fmt.Errorf("factory: failed to init module %s: %w", currentModule.Name(), err)
	}
	p.modules = append(p.modules, currentModule)
	logger.Infof("Factory: Module %s initialized.", currentModule.Name())

	// Worker Module
	workerMod := &worker.WorkerModule{}
	currentModule = workerMod
	// Define a spec for worker module if it needs one, e.g. combining Kubernetes and CP Endpoint
	workerModuleSpec := struct {
		Kubernetes           config.KubernetesSpec
		ControlPlaneEndpoint config.ControlPlaneEndpointSpec
	}{
		Kubernetes:           *kr.Cluster.Kubernetes,
		ControlPlaneEndpoint: *kr.Cluster.ControlPlaneEndpoint,
	}
	if err = currentModule.Init(kr, &workerModuleSpec, logger.WithField("module", currentModule.Name())); err != nil {
		return nil, fmt.Errorf("factory: failed to init module %s: %w", currentModule.Name(), err)
	}
	p.modules = append(p.modules, currentModule)
	logger.Infof("Factory: Module %s initialized.", currentModule.Name())

	// CNI Module
	if kr.Cluster.Network.Plugin == "calico" { // Example: specific module for calico
		cniMod := &network.CalicoCNIModule{}
		currentModule = cniMod
		if err = currentModule.Init(kr, kr.Cluster.Network, logger.WithField("module", currentModule.Name())); err != nil { // Pass NetworkSpec
			return nil, fmt.Errorf("factory: failed to init module %s: %w", currentModule.Name(), err)
		}
		p.modules = append(p.modules, currentModule)
		logger.Infof("Factory: Module %s initialized.", currentModule.Name())
	} else if kr.Cluster.Network.Plugin != "" { // Other non-empty plugin specified
		logger.Warnf("Factory: CNI plugin '%s' is configured but no specific module is available for it in this pipeline. CNI might not be installed.", kr.Cluster.Network.Plugin)
	} else { // Plugin is empty
		logger.Info("Factory: No CNI plugin specified. Skipping CNI module.")
	}

	logger.Info("Factory: InstallPipeline instance created and all modules initialized.")
	return p, nil
}

func init() {
	if err := pipeline.Register("cluster-install", NewInstallPipelineFactory); err != nil {
		panic(fmt.Sprintf("Failed to register 'cluster-install' pipeline: %v", err))
	}
}

func (p *InstallPipeline) Name() string {
	return p.NameField
}

func (p *InstallPipeline) Description() string {
	return p.DescriptionField
}

func (p *InstallPipeline) ExpectedParameters() []pipeline.ParameterDefinition {
	return nil
}

func (p *InstallPipeline) Start(logger *logrus.Entry) error {
	if p.KubeRuntime == nil || p.KubeRuntime.Cluster == nil || p.modules == nil {
		return fmt.Errorf("pipeline not properly initialized before Start: KubeRuntime, ClusterSpec, or modules list is nil")
	}

	pipelineExecLogger := logger // Use the logger passed from main, already scoped from KubeRuntime.Log
	pipelineExecLogger.Infof("Executing pipeline for cluster: %s, K8s version: %s",
		p.KubeRuntime.Cluster.Kubernetes.ClusterName,
		p.KubeRuntime.Cluster.Kubernetes.Version)

	for _, mod := range p.modules {
		moduleLogger := pipelineExecLogger.WithField("module", mod.Name())

		skip, errSkip := mod.IsSkip(p.KubeRuntime)
		if errSkip != nil {
			return fmt.Errorf("error checking IsSkip for module %s: %w", mod.Name(), errSkip)
		}
		if skip {
			moduleLogger.Info("Skipping module execution.")
			continue
		}

		// For this iteration, Default & AutoAssert are assumed to be part of module's Init or Run.
		// The full rich lifecycle (Default, AutoAssert, Init, Run, Until, CallPostHook)
		// would be managed by a more sophisticated RunModule implementation.

		moduleLogger.Infof("Executing module: %s (%s)", mod.Name(), mod.Description())
		moduleLogger.Info(mod.Slogan()) // Print slogan before run

		moduleResult := ending.NewModuleResult()
		mod.Run(moduleResult)

		if errHook := mod.CallPostHook(moduleResult); errHook != nil {
			moduleLogger.Warnf("Error during post-hook for module %s: %v", mod.Name(), errHook)
		}

		if moduleResult.IsFailed() {
			moduleLogger.Errorf("Module %s execution failed: %s. Errors: %v", mod.Name(), moduleResult.Message, moduleResult.CombinedError())
			if !p.KubeRuntime.IgnoreError {
				return fmt.Errorf("module '%s' execution failed: %w. Message: %s", mod.Name(), moduleResult.CombinedError(), moduleResult.Message)
			}
			moduleLogger.Warnf("Error in module %s ignored due to pipeline IgnoreError setting.", mod.Name())
		} else if moduleResult.Status == ending.ModuleResultSkipped {
			moduleLogger.Infof("Module %s was skipped. Message: %s", mod.Name(), moduleResult.Message)
		} else {
			moduleLogger.Infof("Module %s completed successfully. Message: %s", mod.Name(), moduleResult.Message)
		}
	}

	pipelineExecLogger.Infof("Pipeline %s finished execution.", p.Name())
	return nil
}

func (p *InstallPipeline) RunModule(mod module.Module) *ending.ModuleResult {
    moduleLogger := p.KubeRuntime.Log.WithFields(logrus.Fields{
        "pipeline": p.Name(),
        "module":   mod.Name(),
        "phase":    "RunModule_direct_call",
    })
    moduleLogger.Infof("Directly running module via RunModule: %s (%s)", mod.Name(), mod.Description())

	// This direct RunModule should ideally also respect the full lifecycle for a module.
	// For now, it's simplified.
	// It assumes Init was already called for modules stored in p.modules.
	// If `mod` is an arbitrary module not from p.modules, its Init state is unknown.

	// Call Default, AutoAssert, Init if not already done.
	// This is complex if module can be Init'd multiple times or if Init relies on order.
	// For this skeleton, let's assume Init has been handled (e.g., in factory).
	// if err := mod.Init(p.KubeRuntime, relevantSpec, moduleLogger); err != nil { ... }

    moduleLogger.Info(mod.Slogan())
    result := ending.NewModuleResult()
    mod.Run(result)

    if errHook := mod.CallPostHook(result); errHook != nil {
        moduleLogger.Warnf("Error during post-hook for module %s (direct RunModule call): %v", mod.Name(), errHook)
    }

    if result.IsFailed() {
        moduleLogger.Errorf("Module %s (direct RunModule call) failed: %s. Errors: %v", mod.Name(), result.Message, result.CombinedError())
    } else {
        moduleLogger.Infof("Module %s (direct RunModule call) completed with status: %s. Message: %s", mod.Name(), result.Status, result.Message)
    }
    return result
}
