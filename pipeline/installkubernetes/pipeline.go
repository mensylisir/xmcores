package installkubernetes

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/config" // For config.ClusterConfig
	"github.com/mensylisir/xmcores/connector"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// InstallKubernetesPipeline defines the structure for installing Kubernetes.
type InstallKubernetesPipeline struct{}

// NewInstallKubernetesPipelineFactory creates a new instance of InstallKubernetesPipeline.
// This function matches the pipeline.PipelineFactory signature.
func NewInstallKubernetesPipelineFactory() pipeline.Pipeline {
	return &InstallKubernetesPipeline{}
}

func init() {
	// Register this pipeline implementation.
	if err := pipeline.Register("cluster-install", NewInstallKubernetesPipelineFactory); err != nil {
		// Using panic here because registration failure at init time is a critical setup error.
		panic(fmt.Sprintf("Failed to register 'cluster-install' pipeline: %v", err))
	}
}

// Name returns the unique name of the pipeline.
func (p *InstallKubernetesPipeline) Name() string {
	return "cluster-install"
}

// Description provides a human-readable summary of the pipeline.
func (p *InstallKubernetesPipeline) Description() string {
	return "Installs a Kubernetes cluster based on the ClusterConfig specification."
}

// ExpectedParameters defines the parameters this pipeline expects.
// Returning nil as ClusterConfig is the primary definition.
func (p *InstallKubernetesPipeline) ExpectedParameters() []pipeline.ParameterDefinition {
	return nil
}

// Execute runs the pipeline's workflow.
func (p *InstallKubernetesPipeline) Execute(initialRuntime runtime.Runtime, configData map[string]interface{}, logger *logrus.Entry) error {
	logger.Infof("Starting execution of pipeline: %s (%s)", p.Name(), p.Description())

	// Type Assert Config
	cfg, ok := configData["clusterConfig"].(*config.ClusterConfig)
	if !ok || cfg == nil {
		errMsg := "InstallKubernetesPipeline expects 'clusterConfig' of type *config.ClusterConfig in configData, but it was not found or of wrong type"
		logger.Error(errMsg)
		if val, present := configData["clusterConfig"]; present {
			logger.Errorf("Actual type of 'clusterConfig': %T", val)
		}
		return fmt.Errorf(errMsg)
	}
	logger.Infof("Executing '%s' pipeline for cluster: %s (K8s Version: %s)",
		p.Name(), cfg.Metadata.Name, cfg.Spec.Kubernetes.Version)

	// Initialize Pipeline-Specific Runtime
	pipelineRtCfg := runtime.Config{
		WorkDir:     initialRuntime.WorkDir(),
		IgnoreError: initialRuntime.IgnoreError(),
		Verbose:     initialRuntime.Verbose(),
		ObjectName:  initialRuntime.ObjectName() + "/" + p.Name(),
		AllHosts:    make([]connector.Host, 0, len(cfg.Spec.Hosts)),
		RoleHosts:   make(map[string][]connector.Host),
	}

	hostMapByName := make(map[string]connector.Host)
	logger.Infof("Processing %d host definitions from ClusterConfig...", len(cfg.Spec.Hosts))
	for i, hostSpec := range cfg.Spec.Hosts {
		host := connector.NewHost()
		host.SetName(hostSpec.Name)
		host.SetAddress(hostSpec.Address)
		host.SetInternalAddress(hostSpec.InternalAddress)

		port := hostSpec.Port
		if port == 0 {
			port = 22 // Default SSH port
		}
		host.SetPort(port)
		host.SetUser(hostSpec.User)
		host.SetPassword(hostSpec.Password)
		host.SetPrivateKeyPath(hostSpec.PrivateKeyPath)

		if err := host.Validate(); err != nil {
			errMsg := fmt.Sprintf("Host %d ('%s', Address: '%s') validation failed: %v. This host will be skipped.", i+1, hostSpec.Name, hostSpec.Address, err)
			logger.Error(errMsg)
			continue
		}
		pipelineRtCfg.AllHosts = append(pipelineRtCfg.AllHosts, host)
		if host.GetName() == "" {
			logger.Warnf("Host at index %d with address %s has no name, cannot be used in roleGroups by name.", i, host.GetAddress())
		} else {
			hostMapByName[host.GetName()] = host
		}
		logger.Debugf("Loaded and validated host: %s (%s)", host.GetName(), host.GetAddress())
	}
	logger.Infof("Successfully processed %d valid hosts into pipeline runtime config.", len(pipelineRtCfg.AllHosts))

	if cfg.Spec.RoleGroups != nil {
		logger.Info("Processing roleGroups...")
		for role, hostNames := range cfg.Spec.RoleGroups {
			var hostsInRole []connector.Host
			for _, hostName := range hostNames {
				host, found := hostMapByName[hostName]
				if !found {
					logger.Warnf("Host '%s' defined in role '%s' not found among validated hosts. Skipping for this role.", hostName, role)
					continue
				}
				hostsInRole = append(hostsInRole, host)
			}
			if len(hostsInRole) > 0 {
				pipelineRtCfg.RoleHosts[role] = hostsInRole
				logger.Debugf("Role '%s' assigned to hosts: %v", role, hostNames)
			} else {
				logger.Warnf("No valid hosts found or assigned for role '%s'. This role will be empty.", role)
			}
		}
	}
	// Log all defined roles and the number of hosts in them for clarity
	definedRoles := []string{}
	for roleName, hostsInRole := range pipelineRtCfg.RoleHosts {
		definedRoles = append(definedRoles, fmt.Sprintf("%s (%d hosts)", roleName, len(hostsInRole)))
	}
	if len(definedRoles) > 0 {
		logger.Infof("Processed roles: %s.", strings.Join(definedRoles, ", "))
	} else {
		logger.Info("No roles were effectively defined or populated with valid hosts.")
	}


	pipelineRt, err := runtime.NewRuntime(pipelineRtCfg)
	if err != nil {
		logger.Errorf("Failed to create pipeline-specific runtime: %v", err)
		return fmt.Errorf("failed to create pipeline-specific runtime: %w", err)
	}

	logger.Infof("Pipeline-specific runtime initialized. ObjectName: %s. Total hosts: %d. Roles defined: %d.",
		pipelineRt.ObjectName(), len(pipelineRt.AllHosts()), len(pipelineRt.RoleHosts()))


	// Module Orchestration (Logging Placeholders)
	networkPlugin := cfg.Spec.Network.Plugin
	var blockSizeStr string = "N/A (not set)"
	if cfg.Spec.Network.BlockSize != nil {
		blockSizeStr = fmt.Sprintf("%d", *cfg.Spec.Network.BlockSize)
	}
	logger.Infof("Network plugin: %s, BlockSize: %s", networkPlugin, blockSizeStr)

	// --- Etcd Module Preparation ---
	logger.Infof("Preparing for Etcd setup. Configured Etcd type: %s", cfg.Spec.Etcd.Type)
	if cfg.Spec.Etcd.Type == "external" {
		logger.Infof("Using external etcd. Endpoints: %v", cfg.Spec.Etcd.Endpoints)
		if cfg.Spec.Etcd.CAFile != "" {
			logger.Infof("External etcd CAFile configured: %s", cfg.Spec.Etcd.CAFile)
		}
		if cfg.Spec.Etcd.CertFile != "" && cfg.Spec.Etcd.KeyFile != "" {
			logger.Info("External etcd client certificate and key are configured.")
		} else if cfg.Spec.Etcd.CertFile != "" || cfg.Spec.Etcd.KeyFile != "" {
			logger.Warn("External etcd client certificate or key is partially configured. Both are usually needed.")
		}
	} else if cfg.Spec.Etcd.Type == "xm" {
		logger.Info("Etcd type 'xm': This pipeline would perform binary installation and management of etcd.")
	} else if cfg.Spec.Etcd.Type == "kubeadm" || cfg.Spec.Etcd.Type == "" { // Default to kubeadm if empty
		logger.Info("Etcd type 'kubeadm' (or default): Etcd will be managed by kubeadm as stacked on control-plane nodes.")
	} else {
		logger.Warnf("Unknown Etcd type specified: '%s'. Proceeding as if 'kubeadm'.", cfg.Spec.Etcd.Type)
	}
	logger.Info("Executing EtcdModule (conceptual)...")
	// etcdModule.Execute(pipelineRt, &cfg.Spec.Etcd, pipelineRt.RoleHosts()["etcd"], logger.WithField("module", "Etcd"))


	// --- Other Conceptual Module Executions ---
	if hosts, ok := pipelineRt.RoleHosts()["loadbalancer"]; ok && len(hosts) > 0 {
		logger.Info("Executing LoadBalancerModule (conceptual)...")
		// lbModule.Execute(pipelineRt, &cfg.Spec.ControlPlaneEndpoint, hosts, logger.WithField("module", "LoadBalancer"))
	} else {
		logger.Info("Skipping LoadBalancerModule: no hosts assigned to 'loadbalancer' role or role not defined.")
	}

	logger.Info("Executing ContainerRuntimeModule (conceptual)...")
	// crModule.Execute(pipelineRt, &cfg.Spec, logger.WithField("module", "ContainerRuntime")) // Needs broader spec parts

	logger.Info("Executing ControlPlaneModule (conceptual)...")
	// cpModule.Execute(pipelineRt, &cfg.Spec, logger.WithField("module", "ControlPlane")) // Needs broader spec parts
	logger.Info("Executing WorkerNodeJoinModule (conceptual)...")
	logger.Info("Executing CNIModule (conceptual)...")
	logger.Info("Executing RegistryModule (conceptual)...")

	logger.Infof("Pipeline '%s' finished conceptual execution.", p.Name())
	return nil
}
