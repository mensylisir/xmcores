package installkubernetes

import (
	"fmt"

	"github.com/mensylisir/xmcores/config" // For config.ClusterConfig
	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// InstallKubernetesPipeline defines the structure for installing Kubernetes.
type InstallKubernetesPipeline struct{}

// NewInstallKubernetesPipelineFactory creates a new factory for InstallKubernetesPipeline.
// The name change from NewInstallKubernetesPipeline to NewInstallKubernetesPipelineFactory
// is to make it clear this returns a factory function.
// However, the registry expects `PipelineFactory func() Pipeline`.
// So, the function registered should be `NewInstallKubernetesPipeline` which returns `pipeline.Pipeline`.
func NewInstallKubernetesPipeline() pipeline.Pipeline {
	return &InstallKubernetesPipeline{}
}

func init() {
	// Register this pipeline implementation.
	// The name "cluster-install" is chosen as per the subtask.
	if err := pipeline.Register("cluster-install", NewInstallKubernetesPipeline); err != nil {
		panic(fmt.Sprintf("Failed to register 'cluster-install' pipeline: %v", err))
	}
}

// Name returns the unique name of the pipeline.
func (p *InstallKubernetesPipeline) Name() string {
	return "cluster-install"
}

// Description provides a human-readable summary of what the pipeline does.
func (p *InstallKubernetesPipeline) Description() string {
	return "Installs a Kubernetes cluster based on the ClusterConfig specification."
}

// ExpectedParameters defines the parameters this pipeline expects.
// For this iteration, returning nil as ClusterConfig is the primary definition.
func (p *InstallKubernetesPipeline) ExpectedParameters() []pipeline.ParameterDefinition {
	return nil
}

// Execute runs the pipeline's workflow.
func (p *InstallKubernetesPipeline) Execute(initialRuntime runtime.Runtime, configData map[string]interface{}, logger *logrus.Entry) error {
	logger.Infof("Starting execution of pipeline: %s (%s)", p.Name(), p.Description())

	// Type Assert Config
	cfg, ok := configData["clusterConfig"].(*config.ClusterConfig)
	if !ok || cfg == nil { // Ensure cfg is not nil either
		errMsg := "InstallKubernetesPipeline expects 'clusterConfig' of type *config.ClusterConfig in configData, but it was not found or of wrong type"
		logger.Error(errMsg)
		// Optionally log the actual type if present:
		// if val, present := configData["clusterConfig"]; present {
		//    logger.Errorf("Actual type of 'clusterConfig': %T", val)
		// }
		return fmt.Errorf(errMsg)
	}
	logger.Infof("Successfully received ClusterConfig for cluster: %s (API: %s, Kind: %s)",
		cfg.Metadata.Name, cfg.APIVersion, cfg.Kind)

	// Initialize Pipeline-Specific Runtime
	// Assuming initialRuntime is a base runtime and we create/configure a new one for this pipeline execution.
	// If initialRuntime is already fully configured (e.g. from main.go's interpretation of global params),
	// then some of these Set calls might be redundant or need adjustment.
	// For now, let's assume we are configuring a fresh runtime instance or a clone.

	// The subtask suggests `runtime.NewNoOpRuntime()`. Let's assume this exists or use the existing NewRuntime.
	// If NewRuntime needs a runtime.Config, we'd build that first.
	// Let's assume initialRuntime is the one from main.go and we'll use its setters if available.
	// This part depends heavily on runtime's design.
	// For this step, we'll focus on the logic of populating it.
	// The prompt implies using initialRuntime and calling setters on it.

	// Ensure initialRuntime is not nil
	if initialRuntime == nil {
		return fmt.Errorf("initialRuntime provided to pipeline Execute is nil")
	}

	// The runtime passed from main is already configured with WorkDir, IgnoreError, ObjectName.
	// We might want to refine ObjectName or specific logger fields.
	pipelineScopedLogger := logger.WithField("pipeline", p.Name()).WithField("cluster", cfg.Metadata.Name)

	// Instead of pipelineRt := runtime.NewNoOpRuntime(), we use the passed 'initialRuntime'.
	// We need to ensure 'initialRuntime' has the necessary setters.
	// This casting is to access specific implementation methods if setters are not on the interface.
	// This is a common issue; ideally setters are on the interface or it's configured via a Config struct.

	// Let pipelineRuntime be the runtime instance we are configuring for this pipeline.
	// If initialRuntime is mutable and we want to scope changes, cloning would be best.
	// For now, assuming initialRuntime can be directly configured or these are global settings.

	// The subtask asks to create a new `runtime.Runtime` instance `pipelineRt`.
	// This implies `initialRuntime` might be a base/global one, and `pipelineRt` is for this specific pipeline run.
	// Let's assume runtime.NewRuntime is the standard constructor.
	// We need to populate a runtime.Config for it.

	rtCfg := runtime.Config{
		WorkDir:     initialRuntime.WorkDir(),     // Inherit global workdir
		IgnoreError: initialRuntime.IgnoreError(), // Inherit ignoreError
		ObjectName:  initialRuntime.ObjectName() + "/" + p.Name(), // Append pipeline name
		Verbose:     initialRuntime.Verbose(),     // Inherit verbose
		// AllHosts and RoleHosts will be populated below.
	}

	// Populate AllHosts
	allHosts := make([]connector.Host, 0, len(cfg.Spec.Hosts))
	hostMapByName := make(map[string]connector.Host) // For quick lookup when building RoleHosts

	pipelineScopedLogger.Infof("Processing %d host definitions from ClusterConfig...", len(cfg.Spec.Hosts))
	for i, hostSpec := range cfg.Spec.Hosts {
		host := connector.NewHost()
		host.SetName(hostSpec.Name)
		host.SetAddress(hostSpec.Address)
		host.SetInternalAddress(hostSpec.InternalAddress)
		host.SetPort(hostSpec.Port) // Assuming 0 is a valid default handled by connector or later logic if not set
		if hostSpec.Port == 0 { // Explicitly default to 22 if not set in YAML
			host.SetPort(22)
		}
		host.SetUser(hostSpec.User)
		host.SetPassword(hostSpec.Password)
		host.SetPrivateKeyPath(hostSpec.PrivateKeyPath)
		// host.SetArch(hostSpec.Arch) // If Arch field is added

		if err := host.Validate(); err != nil {
			errMsg := fmt.Sprintf("Host %d ('%s', Address: '%s') validation failed: %v. Skipping host.", i+1, hostSpec.Name, hostSpec.Address, err)
			pipelineScopedLogger.Error(errMsg)
			// Depending on strictness, might return error here or just skip faulty host.
			// For now, let's log and skip. If a required host (e.g. for etcd/cp) is skipped, later stages should fail.
			continue
		}
		allHosts = append(allHosts, host)
		if host.GetName() == "" {
             pipelineScopedLogger.Warnf("Host at index %d with address %s has no name, cannot be used in roleGroups by name.", i, host.GetAddress())
        } else {
		    hostMapByName[host.GetName()] = host
        }
		pipelineScopedLogger.Debugf("Loaded host: %s (%s)", host.GetName(), host.GetAddress())
	}
	rtCfg.AllHosts = allHosts
	pipelineScopedLogger.Infof("Successfully processed %d valid hosts.", len(allHosts))


	// Parse RoleGroups
	roleHostsMap := make(map[string][]connector.Host)
	if cfg.Spec.RoleGroups != nil {
		pipelineScopedLogger.Info("Processing roleGroups...")
		for role, hostNames := range cfg.Spec.RoleGroups {
			var hostsInRole []connector.Host
			for _, hostName := range hostNames {
				host, found := hostMapByName[hostName]
				if !found {
					pipelineScopedLogger.Warnf("Host '%s' defined in role '%s' not found in the list of valid hosts. Skipping.", hostName, role)
					continue
				}
				hostsInRole = append(hostsInRole, host)
			}
			if len(hostsInRole) > 0 {
				roleHostsMap[role] = hostsInRole
				pipelineScopedLogger.Debugf("Role '%s' assigned to hosts: %v", role, hostNames)
			} else {
				pipelineScopedLogger.Warnf("No valid hosts found or assigned for role '%s'.", role)
			}
		}
	}
	rtCfg.RoleHosts = roleHostsMap
	pipelineScopedLogger.Infof("Processed %d roles.", len(roleHostsMap))

	// Create the pipeline-specific runtime instance
	pipelineRt, err := runtime.NewRuntime(rtCfg)
	if err != nil {
		return fmt.Errorf("failed to create pipeline-specific runtime: %w", err)
	}
	// If runtime needs a logger explicitly set (NewRuntime doesn't take it)
	// And if runtime has a SetLog method (it should be part of its config or init)
	// For now, assuming logger passed to modules/steps is sufficient.


	// Module Orchestration (Logging Placeholders)
	// These modules would now use pipelineRt.
	pipelineScopedLogger.Info("Executing EtcdModule (conceptual)...")
	// etcdModule.Execute(pipelineRt, cfg.Spec.Etcd, pipelineScopedLogger.WithField("module", "etcd"))

	pipelineScopedLogger.Info("Executing ContainerRuntimeModule (conceptual)...")
	// containerRuntimeModule.Execute(pipelineRt, cfg.Spec.Kubernetes.ContainerManager, cfg.Spec.Registry, pipelineScopedLogger.WithField("module", "containerRuntime"))

	pipelineScopedLogger.Info("Executing KubernetesControlPlaneModule (conceptual)...")
	// controlPlaneModule.Execute(pipelineRt, cfg.Spec.Kubernetes, cfg.Spec.ControlPlaneEndpoint, pipelineScopedLogger.WithField("module", "controlPlane"))

	pipelineScopedLogger.Info("Executing WorkerNodeJoinModule (conceptual)...")
	// workerModule.Execute(pipelineRt, cfg.Spec.Kubernetes, cfg.Spec.ControlPlaneEndpoint, pipelineScopedLogger.WithField("module", "worker"))

	pipelineScopedLogger.Info("Executing CNIModule (conceptual)...")
	// cniModule.Execute(pipelineRt, cfg.Spec.Network, pipelineScopedLogger.WithField("module", "cni"))

	pipelineScopedLogger.Info("Executing RegistryModule (conceptual)...")
	// registryModule.Execute(pipelineRt, cfg.Spec.Registry, pipelineScopedLogger.WithField("module", "registry"))


	pipelineScopedLogger.Infof("Pipeline %s finished conceptual execution.", p.Name())
	return nil
}
