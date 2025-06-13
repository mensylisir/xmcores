package kubernetes

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"

	// Conceptual module imports - these packages/structs will be created if they don't exist
	// For now, assume they will have at least the struct and methods to satisfy module.Module
	"github.com/mensylisir/xmcores/module/containerruntime"
	"github.com/mensylisir/xmcores/module/etcd"
	controlplane "github.com/mensylisir/xmcores/module/kubernetes/controlplane" // Alias to avoid conflict if other 'kubernetes' pkgs are modules
	"github.com/mensylisir/xmcores/module/kubernetes/worker"                 // Conceptual new worker module
	"github.com/mensylisir/xmcores/module/loadbalancer"
	"github.com/mensylisir/xmcores/module/network" // For CNIModule (e.g. CalicoCNIModule)
)

// InstallPipeline defines the structure for the Kubernetes installation pipeline.
type InstallPipeline struct {
	logger          *logrus.Entry
	clusterSpec     *config.ClusterSpec // Store the spec from ClusterConfig
	pipelineRuntime runtime.Runtime     // Pipeline's own fully initialized runtime
	modules         []module.Module     // Slice to store initialized modules
}

// NewInstallPipelineFactory creates a new instance of InstallPipeline.
func NewInstallPipelineFactory() pipeline.Pipeline {
	return &InstallPipeline{}
}

func init() {
	if err := pipeline.Register("cluster-install", NewInstallPipelineFactory); err != nil {
		panic(fmt.Sprintf("Failed to register 'cluster-install' pipeline: %v", err))
	}
}

// Name returns the unique name of the pipeline.
func (p *InstallPipeline) Name() string {
	return "cluster-install"
}

// Description provides a human-readable summary of the pipeline.
func (p *InstallPipeline) Description() string {
	return "Installs a Kubernetes cluster based on the ClusterConfig specification."
}

// ExpectedParameters defines the parameters this pipeline expects.
func (p *InstallPipeline) ExpectedParameters() []pipeline.ParameterDefinition {
	return nil // ClusterConfig struct is the primary definition
}

// Init prepares the pipeline for execution.
func (p *InstallPipeline) Init(cfg *config.ClusterConfig, initialRuntime runtime.Runtime, logger *logrus.Entry) error {
	p.logger = logger.WithField("pipeline", p.Name())
	p.logger.Infof("Initializing pipeline for cluster: %s", cfg.Metadata.Name)

	if cfg == nil || cfg.Spec == nil {
		return fmt.Errorf("ClusterConfig or its Spec is nil")
	}
	p.clusterSpec = cfg.Spec // Store the spec

	// Initialize pipelineRuntime
	pipelineRtCfg := runtime.Config{
		WorkDir:     initialRuntime.WorkDir(),
		IgnoreError: initialRuntime.IgnoreError(),
		Verbose:     initialRuntime.Verbose(),
		ObjectName:  initialRuntime.ObjectName() + "/" + p.Name(),
		AllHosts:    make([]connector.Host, 0, len(p.clusterSpec.Hosts)),
		RoleHosts:   make(map[string][]connector.Host),
	}

	hostMapByName := make(map[string]connector.Host)
	p.logger.Infof("Processing %d host definitions from ClusterConfig...", len(p.clusterSpec.Hosts))
	for i, hostSpec := range p.clusterSpec.Hosts {
		host := connector.NewHost()
		host.SetName(hostSpec.Name)
		host.SetAddress(hostSpec.Address)
		host.SetInternalAddress(hostSpec.InternalAddress)
		port := hostSpec.Port
		if port == 0 {
			port = 22
		}
		host.SetPort(port)
		host.SetUser(hostSpec.User)
		host.SetPassword(hostSpec.Password)
		host.SetPrivateKeyPath(hostSpec.PrivateKeyPath)

		if err := host.Validate(); err != nil {
			p.logger.Errorf("Host %d ('%s', Address: '%s') validation failed: %v. This host will be skipped.", i+1, hostSpec.Name, hostSpec.Address, err)
			continue
		}
		pipelineRtCfg.AllHosts = append(pipelineRtCfg.AllHosts, host)
		if host.GetName() != "" {
			hostMapByName[host.GetName()] = host
		}
		p.logger.Debugf("Loaded and validated host: %s (%s)", host.GetName(), host.GetAddress())
	}
	p.logger.Infof("Successfully processed %d valid hosts into pipeline runtime config.", len(pipelineRtCfg.AllHosts))

	if p.clusterSpec.RoleGroups != nil {
		p.logger.Info("Processing roleGroups...")
		for role, hostNames := range p.clusterSpec.RoleGroups {
			var hostsInRole []connector.Host
			for _, hostName := range hostNames {
				host, found := hostMapByName[hostName]
				if !found {
					p.logger.Warnf("Host '%s' defined in role '%s' not found among validated hosts. Skipping for this role.", hostName, role)
					continue
				}
				hostsInRole = append(hostsInRole, host)
			}
			if len(hostsInRole) > 0 {
				pipelineRtCfg.RoleHosts[role] = hostsInRole
			} else {
				p.logger.Warnf("No valid hosts found or assigned for role '%s'. This role will be empty.", role)
			}
		}
	}
	// Log processed roles
	definedRoles := []string{}
	for roleName, hostsInRole := range pipelineRtCfg.RoleHosts {
		definedRoles = append(definedRoles, fmt.Sprintf("%s (%d hosts)", roleName, len(hostsInRole)))
	}
	if len(definedRoles) > 0 {
		p.logger.Infof("Processed roles: %s.", strings.Join(definedRoles, ", "))
	} else {
		p.logger.Info("No roles were effectively defined or populated with valid hosts.")
	}


	var err error
	p.pipelineRuntime, err = runtime.NewRuntime(pipelineRtCfg)
	if err != nil {
		return fmt.Errorf("failed to create pipeline-specific runtime: %w", err)
	}
	// If runtime needs explicit logger setting: p.pipelineRuntime.SetLog(p.logger.WithField("runtime_scope", "pipeline_operational"))
	p.logger.Infof("Pipeline runtime initialized. ObjectName: %s. Total hosts: %d. Roles defined: %d.",
		p.pipelineRuntime.ObjectName(), len(p.pipelineRuntime.AllHosts()), len(p.pipelineRuntime.RoleHosts()))

	// Instantiate and Init Modules
	p.modules = make([]module.Module, 0)
	var currentModule module.Module

	// Load Balancer Module (Conditional)
	if p.clusterSpec.ControlPlaneEndpoint.LoadBalancer.Enable {
		// Assuming HAProxyKeepalivedModule is the one if type is "haproxy-keepalived" or similar
		if p.clusterSpec.ControlPlaneEndpoint.LoadBalancer.Type == "haproxy-keepalived" { // Example type check
			lbMod := &loadbalancer.HAProxyKeepalivedModule{}
			currentModule = lbMod
			if err := currentModule.Init(p.pipelineRuntime, &p.clusterSpec.ControlPlaneEndpoint, p.logger.WithField("module", currentModule.Name())); err != nil {
				return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
			}
			p.modules = append(p.modules, currentModule)
			p.logger.Infof("Module %s initialized.", currentModule.Name())
		} else if p.clusterSpec.ControlPlaneEndpoint.LoadBalancer.Type == "external" {
			p.logger.Info("External load balancer is configured. Skipping internal LoadBalancerModule.")
		} else {
			p.logger.Warnf("Load balancer enabled but type '%s' is unknown or not supported for automatic setup. Manual setup may be required.", p.clusterSpec.ControlPlaneEndpoint.LoadBalancer.Type)
		}
	} else {
		p.logger.Info("Internal load balancer setup is disabled via config.")
	}

	// Etcd Module
	// Conceptual: Select etcd module based on p.clusterSpec.Etcd.Type
	etcdMod := &etcd.EtcdModule{} // Assuming a generic EtcdModule for now
	currentModule = etcdMod
	if err := currentModule.Init(p.pipelineRuntime, &p.clusterSpec.Etcd, p.logger.WithField("module", currentModule.Name())); err != nil {
		return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
	}
	p.modules = append(p.modules, currentModule)
	p.logger.Infof("Module %s initialized.", currentModule.Name())

	// Container Runtime Module
	if p.clusterSpec.Kubernetes.ContainerManager == "containerd" {
		containerdMod := &containerruntime.ContainerdModule{}
		currentModule = containerdMod
		// Pass relevant spec: could be KubernetesSpec, or a dedicated ContainerdSpec if it exists in ClusterSpec
		moduleSpecForCR := &p.clusterSpec.Kubernetes // Or p.clusterSpec.ContainerRuntime if that was the structure
		if err := currentModule.Init(p.pipelineRuntime, moduleSpecForCR, p.logger.WithField("module", currentModule.Name())); err != nil {
			return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
		}
		p.modules = append(p.modules, currentModule)
		p.logger.Infof("Module %s initialized.", currentModule.Name())
	} else {
		return fmt.Errorf("unsupported container manager: %s", p.clusterSpec.Kubernetes.ContainerManager)
	}

	// Control Plane Module
	cpMod := &controlplane.ControlPlaneModule{}
	currentModule = cpMod
	// ControlPlaneModule might need KubernetesSpec and ControlPlaneEndpointSpec
	// Pass a combined struct or handle multiple spec parts within the module if necessary.
	// For now, passing KubernetesSpec as an example.
	moduleSpecForCP := struct {
		Kubernetes           config.KubernetesSpec
		ControlPlaneEndpoint config.ControlPlaneEndpointSpec
	}{
		Kubernetes:           *p.clusterSpec.Kubernetes,
		ControlPlaneEndpoint: *p.clusterSpec.ControlPlaneEndpoint,
	}
	if err := currentModule.Init(p.pipelineRuntime, &moduleSpecForCP, p.logger.WithField("module", currentModule.Name())); err != nil {
		return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
	}
	p.modules = append(p.modules, currentModule)
	p.logger.Infof("Module %s initialized.", currentModule.Name())

	// Worker Module (Conceptual)
	// Assuming worker.WorkerModule struct exists
	workerMod := &worker.WorkerModule{}
	currentModule = workerMod
	// Worker module might need KubernetesSpec (for version, etc.) and ControlPlaneEndpoint (to join)
	moduleSpecForWorker := struct {
		Kubernetes           config.KubernetesSpec
		ControlPlaneEndpoint config.ControlPlaneEndpointSpec
	}{
		Kubernetes:           *p.clusterSpec.Kubernetes,
		ControlPlaneEndpoint: *p.clusterSpec.ControlPlaneEndpoint,
	}
	if err := currentModule.Init(p.pipelineRuntime, &moduleSpecForWorker, p.logger.WithField("module", currentModule.Name())); err != nil {
		return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
	}
	p.modules = append(p.modules, currentModule)
	p.logger.Infof("Module %s initialized.", currentModule.Name())


	// CNI Module
	// Conceptual: Select CNI module based on p.clusterSpec.Network.Plugin
	// For now, assume CalicoCNIModule if plugin is "calico"
	if p.clusterSpec.Network.Plugin == "calico" {
		cniMod := &network.CalicoCNIModule{} // Assuming this struct exists in network package
		currentModule = cniMod
		if err := currentModule.Init(p.pipelineRuntime, p.clusterSpec.Network, p.logger.WithField("module", currentModule.Name())); err != nil {
			return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
		}
		p.modules = append(p.modules, currentModule)
		p.logger.Infof("Module %s initialized.", currentModule.Name())
	} else {
		p.logger.Warnf("Unsupported CNI plugin or no CNI plugin specified: %s. CNI will not be installed by this pipeline.", p.clusterSpec.Network.Plugin)
	}

	// Registry Module (Conceptual - if needed as a distinct module)
	// registryMod := &registry.RegistryModule{} // Assuming a generic registry module
	// currentModule = registryMod
	// if err := currentModule.Init(p.pipelineRuntime, &p.clusterSpec.Registry, p.logger.WithField("module", currentModule.Name())); err != nil {
	// 	return fmt.Errorf("failed to init module %s: %w", currentModule.Name(), err)
	// }
	// p.modules = append(p.modules, currentModule)
	// p.logger.Infof("Module %s initialized.", currentModule.Name())


	p.logger.Info("All specified modules initialized successfully.")
	return nil
}

// Execute runs the main logic of the pipeline by iterating through its initialized modules.
func (p *InstallPipeline) Execute(logger *logrus.Entry) error {
	// Use the pipeline's own logger, which should have been set during Init.
	// If logger param is preferred, then use that one. For consistency with Module.Execute, let's use param.
	execLogger := p.logger // Or directly use logger param: execLogger := logger
	if p.clusterSpec == nil || p.pipelineRuntime == nil || p.modules == nil {
		return fmt.Errorf("pipeline not initialized properly before Execute. clusterSpec, pipelineRuntime, or modules list is nil")
	}

	execLogger.Infof("Executing pipeline for cluster: %s, K8s version: %s",
		p.clusterSpec.Kubernetes.ClusterName, // Using Kubernetes.ClusterName for user-facing name
		p.clusterSpec.Kubernetes.Version)

	for _, mod := range p.modules {
		moduleLogger := execLogger.WithField("module", mod.Name())
		moduleLogger.Infof("Executing module: %s (%s)", mod.Name(), mod.Description())

		if err := mod.Execute(moduleLogger); err != nil {
			moduleLogger.Errorf("Module %s execution failed: %v", mod.Name(), err)
			if !p.pipelineRuntime.IgnoreError() {
				return fmt.Errorf("module '%s' execution failed: %w", mod.Name(), err)
			}
			moduleLogger.Warnf("Error in module %s ignored due to pipeline IgnoreError setting.", mod.Name())
		} else {
			moduleLogger.Infof("Module %s completed successfully.", mod.Name())
		}
	}

	execLogger.Infof("Pipeline %s finished execution.", p.Name())
	return nil
}
