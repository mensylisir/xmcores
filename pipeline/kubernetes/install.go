package kubernetes

import (
	"fmt"
	"os" // For os.Exit example
	"time" // For time.Sleep in Until loop example

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/pipeline/ending"
	krt "github.com/mensylisir/xmcores/runtime" // Alias for ClusterRuntime
	"github.com/sirupsen/logrus"
	"github.com/pkg/errors" // For errors.Wrapf

	// Import actual module packages - ensure these paths are correct
	// and that the modules implement the rich module.Module interface.
	"github.com/mensylisir/xmcores/module/containerruntime"
	"github.com/mensylisir/xmcores/module/etcd"
	"github.com/mensylisir/xmcores/module/kubernetes/controlplane"
	"github.com/mensylisir/xmcores/module/kubernetes/worker"
	"github.com/mensylisir/xmcores/module/loadbalancer"
	"github.com/mensylisir/xmcores/module/network"
	// Conceptual imports for other modules mentioned in KubeKey example:
	// "github.com/mensylisir/xmcores/module/precheck"
	// "github.com/mensylisir/xmcores/module/customscripts"
	// "github.com/mensylisir/xmcores/module/kubekeyaddons"
	// "github.com/mensylisir/xmcores/module/kubesphere"
)

// InstallPipeline defines the structure for the Kubernetes installation pipeline.
type InstallPipeline struct {
	pipeline.ConcretePipeline // Embeds NameField, DescriptionField, Runtime, Modules, etc.
}

// NewInstallPipelineFactory is the factory function for InstallPipeline.
// It receives an initialized ClusterRuntime and constructs the pipeline with its modules.
func NewInstallPipelineFactory(clusterRuntime *krt.ClusterRuntime) (pipeline.Pipeline, error) {
	logger := clusterRuntime.Log.WithField("pipeline_factory", "InstallPipeline")
	logger.Info("Constructing InstallPipeline and initializing modules...")

	// Create the base concrete pipeline
	p := &InstallPipeline{
		ConcretePipeline: pipeline.NewConcretePipeline("cluster-install", "Installs a Kubernetes Cluster", clusterRuntime),
	}

	// List of modules to initialize and add to the pipeline
	// This sequence and the moduleSpec passed to Default will mirror KubeKey's approach.
	// Module constructors (e.g., etcd.NewEtcdModule()) return a module.Module interface.

	// Helper function to instantiate, Default, AutoAssert, and Init a module
	addModule := func(mod module.Module, moduleSpec interface{}) error {
		// 1. Default: Pass runtime, spec, and caches
		// PipelineCache is on ConcretePipeline.Runtime.PipelineCache
		// NewModuleCache is a method on ConcretePipeline (delegates to Runtime)
		moduleCache := p.NewModuleCache() // Each module gets its own cache
		defer p.ReleaseModuleCache(moduleCache) // Schedule cleanup

		if err := mod.Default(p.Runtime, moduleSpec, p.Runtime.PipelineCache, moduleCache); err != nil {
			return errors.Wrapf(err, "failed to Default module %s", mod.Name())
		}

		// 2. AutoAssert (using runtime passed in Default)
		if err := mod.AutoAssert(p.Runtime); err != nil {
			return errors.Wrapf(err, "AutoAssert failed for module %s", mod.Name())
		}

		// 3. Init (module's internal task/step assembly)
		if err := mod.Init(); err != nil {
			return errors.Wrapf(err, "failed to Init module %s", mod.Name())
		}
		p.AddModule(mod) // AddModule is on ConcretePipeline
		logger.Infof("Module %s initialized and added to pipeline.", mod.Name())
		return nil
	}

	// --- Instantiate and Initialize Modules ---
	// The order follows a logical cluster setup sequence.
	// Actual module structs (e.g. etcd.EtcdModule) must exist.

	// Example: Precheck Module (conceptual)
	// if err := addModule(&precheck.SomePrecheckModule{}, nil); err != nil { return nil, err }

	// LoadBalancer Module (conditional)
	if p.Runtime.Cluster.ControlPlaneEndpoint.LoadBalancer.Enable &&
		p.Runtime.Cluster.ControlPlaneEndpoint.LoadBalancer.Type == "haproxy-keepalived" {
		if err := addModule(loadbalancer.NewHAProxyKeepalivedModule(), &p.Runtime.Cluster.ControlPlaneEndpoint); err != nil {
			return nil, err
		}
	}

	// Etcd Module
	if err := addModule(etcd.NewEtcdModule(), p.Runtime.Cluster.Etcd); err != nil {
		return nil, err
	}

	// Container Runtime Module
	if p.Runtime.Cluster.Kubernetes.ContainerManager == "containerd" {
		// Pass KubernetesSpec because ContainerdModule checks ContainerManager and might need other k8s details
		if err := addModule(containerruntime.NewContainerdModule(), p.Runtime.Cluster.Kubernetes); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("unsupported container manager in factory: %s", p.Runtime.Cluster.Kubernetes.ContainerManager)
	}

	// Kubernetes ControlPlane Module
	cpSpec := controlplane.ControlPlaneModuleSpec{ // As defined in controlplane_module.go
		Kubernetes:           *p.Runtime.Cluster.Kubernetes,
		ControlPlaneEndpoint: *p.Runtime.Cluster.ControlPlaneEndpoint,
	}
	if err := addModule(controlplane.NewControlPlaneModule(), &cpSpec); err != nil {
		return nil, err
	}

	// Kubernetes Worker Module
	workerSpec := worker.WorkerModuleSpec{ // As defined in worker_module.go
		Kubernetes:           *p.Runtime.Cluster.Kubernetes,
		ControlPlaneEndpoint: *p.Runtime.Cluster.ControlPlaneEndpoint,
	}
	if err := addModule(worker.NewWorkerModule(), &workerSpec); err != nil {
		return nil, err
	}

	// Network (CNI) Module
	if p.Runtime.Cluster.Network.Plugin == "calico" { // Example for Calico
		if err := addModule(network.NewCalicoCNIModule(), p.Runtime.Cluster.Network); err != nil {
			return nil, err
		}
	} else if p.Runtime.Cluster.Network.Plugin != "" {
		logger.Warnf("CNI plugin '%s' is configured but no specific module handles it in this pipeline.", p.Runtime.Cluster.Network.Plugin)
	}


	// Example: Addons Module (conceptual)
	// if !p.Runtime.Arg.SkipInstallAddons {
	//    if err := addModule(&kubekeyaddons.SomeAddonsModule{}, p.Runtime.Cluster); err != nil { return nil, err }
	// }

	// Example: KubeSphere Integration (conceptual)
	// if p.Runtime.Cluster.KubeSphere.Enabled {
	//    if err := addModule(&kubesphere.DeployModule{}, p.Runtime.Cluster.KubeSphere); err != nil { return nil, err }
	// }

	logger.Info("InstallPipeline constructed and all modules initialized successfully.")
	return p, nil
}

func init() {
	if err := pipeline.Register("cluster-install", NewInstallPipelineFactory); err != nil {
		panic(fmt.Sprintf("Failed to register 'cluster-install' pipeline: %v", err))
	}
}

// Start begins the pipeline execution.
func (p *InstallPipeline) Start(logger *logrus.Entry) error {
	// Call embedded ConcretePipeline's Init for any common pre-module-loop setup
	if err := p.ConcretePipeline.Init(logger); err != nil {
		return errors.Wrapf(err, "failed to run ConcretePipeline Init for %s", p.Name())
	}

	logger.Infof("Starting pipeline: %s (%s)", p.Name(), p.Description())
	logger.Infof("Cluster: %s, K8s Version: %s", p.Runtime.Cluster.Kubernetes.ClusterName, p.Runtime.Cluster.Kubernetes.Version)

	for _, mod := range p.Modules { // p.Modules is from ConcretePipeline
		moduleScopedLogger := logger.WithField("module", mod.Name())

		skip, errSkip := mod.IsSkip(p.Runtime)
		if errSkip != nil {
			return errors.Wrapf(errSkip, "error during IsSkip for module %s", mod.Name())
		}
		if skip {
			moduleScopedLogger.Info("Skipping module execution.")
			continue
		}

		// Default, AutoAssert, and module.Init were called in the factory.
		// Now call Run, Until, CallPostHook.

		moduleScopedLogger.Info(mod.Slogan())
		moduleResult := p.RunModule(mod) // Use pipeline's RunModule to manage lifecycle

		if moduleResult.IsFailed() {
			moduleScopedLogger.Errorf("Module execution failed. Message: %s. Errors: %v", moduleResult.Message, moduleResult.CombinedError())
			if !p.Runtime.IgnoreErr { // Check IgnoreErr from ClusterRuntime (via BaseRuntime)
				return errors.Wrapf(moduleResult.CombinedError(), "module '%s' failed. Message: %s", mod.Name(), moduleResult.Message)
			}
			moduleScopedLogger.Warnf("Error in module %s ignored due to pipeline IgnoreErr setting.", mod.Name())
		} else if moduleResult.Status == ending.ModuleResultSkipped {
			moduleScopedLogger.Infof("Module was skipped. Message: %s", moduleResult.Message)
		} else {
			moduleScopedLogger.Infof("Module completed successfully. Message: %s", moduleResult.Message)
		}
	}

	p.ReleasePipelineCache() // From ConcretePipeline (delegates to Runtime)
	p.Runtime.Base.CloseAllConnectors() // Assuming Base is public on ClusterRuntime or has a delegation method

	logger.Infof("Pipeline %s finished execution successfully.", p.Name())
	return nil
}

// RunModule executes a single module's lifecycle: Run, Until, CallPostHook.
// It assumes Default, AutoAssert, and Init for the module have already been successfully called (e.g., in factory).
func (p *InstallPipeline) RunModule(mod module.Module) *ending.ModuleResult {
	moduleLogger := p.Runtime.Log.WithFields(logrus.Fields{
		"pipeline": p.Name(),
		"module":   mod.Name(),
	})
	moduleLogger.Infof("Pipeline is now running module: %s (%s)", mod.Name(), mod.Description())

	result := ending.NewModuleResult()
	result.SetStatus(ending.ModuleResultPending) // Start as pending

	// Main execution
	mod.Run(result)

	// Handle Until loop for modules that might be asynchronous or need retries
	if !result.IsFailed() && result.Status != ending.ModuleResultSkipped { // Only run Until if Run didn't fail hard or skip
		startTime := time.Now()
		timeout := time.Minute * 5 // Example: 5 minute timeout for Until loop, should be configurable

		for {
			done, err := mod.Until(p.Runtime)
			if err != nil {
				result.SetError(err, fmt.Sprintf("error in Until for module %s", mod.Name()))
				break
			}
			if done {
				if result.Status == ending.ModuleResultPending { // If Run didn't set a final status
					result.SetStatus(ending.ModuleResultSuccess)
				}
				result.SetMessage(fmt.Sprintf("Module %s completed 'Until' condition successfully. %s", mod.Name(), result.Message))
				break
			}
			if time.Since(startTime) > timeout {
				errMsg := fmt.Sprintf("timeout waiting for module %s to complete 'Until' condition", mod.Name())
				result.SetError(errors.New(errMsg), errMsg)
				break
			}
			moduleLogger.Debugf("Module %s 'Until' condition not met, waiting...", mod.Name())
			time.Sleep(5 * time.Second) // Poll interval
		}
	}

	// Call PostHooks regardless of Run/Until status, passing the result
	if errHook := mod.CallPostHook(result); errHook != nil {
		moduleLogger.Warnf("Error during post-hook for module %s: %v", mod.Name(), errHook)
		// Optionally add to result.Errors if post-hook failures are critical
		// result.AddError(errors.Wrapf(errHook, "post-hook error for %s", mod.Name()), true)
	}

	// Final status logging
	if result.IsFailed() {
		moduleLogger.Errorf("Module %s finished with status FAILED. Message: %s. Errors: %v", mod.Name(), result.Message, result.CombinedError())
	} else if result.Status == ending.ModuleResultSkipped {
		moduleLogger.Infof("Module %s finished with status SKIPPED. Message: %s", mod.Name(), result.Message)
	} else if result.Status == ending.ModuleResultSuccess {
		moduleLogger.Infof("Module %s finished with status SUCCESS. Message: %s", mod.Name(), result.Message)
	} else { // Pending or other
		moduleLogger.Warnf("Module %s finished with status %s. Message: %s. Errors: %v", mod.Name(), result.Status, result.Message, result.CombinedError())
	}
	return result
}
