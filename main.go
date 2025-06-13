package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mensylisir/xmcores/common"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/logger"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/runtime" // Will use KubeRuntime from here
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// Ensure pipeline implementations are imported to trigger their init() registration functions.
	_ "github.com/mensylisir/xmcores/pipeline/kubernetes" // For pipeline registration
)

var (
	// Global flags for Cobra binding
	logLevelFlag            string
	verboseFlag             bool
	workDirFlag             string
	ignoreErrorsFlag        bool
	artifactFlag            string
	skipPushImagesFlag      bool
	deployLocalStorageFlag  bool // Bound to cobra bool flag
	installPackagesFlag     bool
	skipPullImagesFlag      bool
	securityEnhancementFlag bool
	skipInstallAddonsFlag   bool

	// `create cluster` command specific flag
	clusterConfigFilePath string
)

var rootCmd = &cobra.Command{
	Use:   "xm",
	Short: "xm is a cluster manager CLI tool",
	Long: `xm (xm_cluster_manager) is a command-line interface tool
for orchestrating cluster lifecycle management, including creation,
deletion, and scaling of Kubernetes clusters.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		effectiveLogLevel := "info"
		userSetLogLevel := cmd.Flags().Changed("log-level")

		if userSetLogLevel && logLevelFlag != "" {
			effectiveLogLevel = logLevelFlag
		}
		if verboseFlag { // verbose overrides others to debug
			effectiveLogLevel = "debug"
		}

		level, err := logrus.ParseLevel(effectiveLogLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to 'info'. Error: %v\n", effectiveLogLevel, err)
			logger.Log.SetLevel(logrus.InfoLevel)
		} else {
			logger.Log.SetLevel(level)
		}
		logger.Log.Infof("xm CLI starting. Log level set to: %s", logger.Log.GetLevel().String())
		return nil
	},
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create various resources like clusters",
}

var createClusterCmd = &cobra.Command{
	Use:     "cluster",
	Short:   "Create a Kubernetes cluster using a configuration file",
	Example: `  xm create cluster -f path/to/your/cluster-config.yaml`,
	RunE:    runCreateClusterCmd,
}

func runCreateClusterCmd(cmd *cobra.Command, args []string) error {
	appLog := logger.Log.WithField(common.LogFieldApp, "xm-create-cluster")
	appLog.Info("Starting 'create cluster' command...")

	if clusterConfigFilePath == "" {
		return fmt.Errorf("cluster configuration file path (-f, --file) is required")
	}
	appLog.Infof("Loading cluster configuration from: %s", clusterConfigFilePath)

	clusterCfg, err := config.LoadClusterConfig(clusterConfigFilePath)
	if err != nil {
		appLog.Errorf("Failed to load cluster configuration: %v", err)
		return fmt.Errorf("failed to load cluster configuration from '%s': %w", clusterConfigFilePath, err)
	}
	appLog.Infof("Successfully loaded configuration for cluster: %s (API: %s, Kind: %s)",
		clusterCfg.Metadata.Name, clusterCfg.APIVersion, clusterCfg.Kind)

	// Populate CliArgs from global flags
	cliArgs := runtime.NewCliArgs() // Initialize with defaults
	cliArgs.Artifact = artifactFlag
	cliArgs.SkipPushImages = skipPushImagesFlag
	if cmd.Flags().Changed("deploy-local-storage") {
		// If the flag was set by the user, deployLocalStorageFlag holds its value (true or false).
		// We pass the address of this boolean value to the *bool field in CliArgs.
		cliArgs.DeployLocalStorage = &deployLocalStorageFlag
	} // If not changed, cliArgs.DeployLocalStorage remains nil (from NewCliArgs)
	cliArgs.InstallPackages = installPackagesFlag     // Default true from flag/NewCliArgs
	cliArgs.SkipPullImages = skipPullImagesFlag
	cliArgs.SecurityEnhancement = securityEnhancementFlag
	cliArgs.SkipInstallAddons = skipInstallAddonsFlag
	appLog.Debugf("Collected CliArgs: %+v", cliArgs)
	if cliArgs.DeployLocalStorage != nil { // For more explicit logging of the *bool
		appLog.Debugf("CliArgs.DeployLocalStorage explicitly set to: %v", *cliArgs.DeployLocalStorage)
	}

	// Initialize KubeRuntime
	isVerbose := logger.Log.IsLevelEnabled(logrus.DebugLevel) || logger.Log.IsLevelEnabled(logrus.TraceLevel)
	kubeRt, err := runtime.NewKubeRuntime(
		clusterCfg,
		cliArgs,
		workDirFlag,
		ignoreErrorsFlag,
		isVerbose,
		logger.Log, // Pass the configured global logger instance
	)
	if err != nil {
		appLog.Errorf("Failed to initialize KubeRuntime: %v", err)
		return fmt.Errorf("failed to initialize KubeRuntime: %w", err)
	}
	appLog.Info("KubeRuntime initialized successfully.")

	// Pipeline Selection & Instantiation
	pipelineName := "cluster-install" // This command is hardcoded to run the "cluster-install" pipeline
	appLog.Infof("Attempting to get and initialize pipeline: '%s'", pipelineName)

	selectedPipeline, err := pipeline.GetPipeline(pipelineName, kubeRt) // Pass KubeRuntime to factory
	if err != nil {
		availablePipelines := pipeline.GetRegisteredPipelineNames()
		errMsg := fmt.Sprintf("Failed to get pipeline '%s': %v.", pipelineName, err)
		if len(availablePipelines) > 0 {
			errMsg += fmt.Sprintf(" Available registered pipelines: %s", strings.Join(availablePipelines, ", "))
		} else {
			errMsg += " No pipelines are currently registered."
		}
		if selectedPipeline == nil { // Check if factory itself failed (e.g. during its internal Init steps)
			errMsg += " The pipeline factory may have failed during initialization."
		}
		appLog.Error(errMsg)
		return fmt.Errorf(errMsg)
	}
	appLog.Infof("Successfully obtained and initialized pipeline: %s (%s)", selectedPipeline.Name(), selectedPipeline.Description())

	// Execute Pipeline using Start() method
	// The logger for the pipeline execution can be derived from KubeRuntime's logger
	pipelineExecutionLogger := kubeRt.Log.WithField("pipeline_execution_id", selectedPipeline.Name()) // Example of further scoping
	appLog.Infof("Starting pipeline execution: %s", selectedPipeline.Name())

	if err = selectedPipeline.Start(pipelineExecutionLogger); err != nil { // Start method takes a logger
		appLog.Errorf("Pipeline '%s' execution failed: %v", selectedPipeline.Name(), err)
		return fmt.Errorf("pipeline '%s' execution failed: %w", selectedPipeline.Name(), err)
	}

	appLog.Infof("Pipeline '%s' executed successfully.", selectedPipeline.Name())
	appLog.Info("'create cluster' command finished successfully.")
	return nil
}

func init() {
	// Bind global operational flags
	rootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "", "Log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose (debug) logging (shorthand for --log-level=debug)")
	rootCmd.PersistentFlags().StringVar(&workDirFlag, "work-dir", "./.xm_work_data", "Working directory for temporary files and operational data")
	rootCmd.PersistentFlags().BoolVar(&ignoreErrorsFlag, "ignore-errors", false, "Ignore errors in execution where supported")

	// Bind flags for CliArgs to rootCmd persistent flags
	rootCmd.PersistentFlags().StringVar(&artifactFlag, "artifact", "", "Path to artifact (e.g., offline package tarball)")
	rootCmd.PersistentFlags().BoolVar(&skipPushImagesFlag, "skip-push-images", false, "Skip pushing images to a private registry")
	// For deployLocalStorageFlag (which corresponds to *bool CliArgs.DeployLocalStorage):
	// Default value false. If user specifies --deploy-local-storage, it's true.
	// If user specifies --deploy-local-storage=false, it's false.
	// Logic in runCreateClusterCmd checks cmd.Flags().Changed() to set the *bool appropriately.
	rootCmd.PersistentFlags().BoolVar(&deployLocalStorageFlag, "deploy-local-storage", false, "Deploy local storage provisioner (e.g., OpenEBS LocalPV). If not set, pipeline defaults apply.")
	rootCmd.PersistentFlags().BoolVar(&installPackagesFlag, "install-packages", true, "Allow installation of OS packages")
	rootCmd.PersistentFlags().BoolVar(&skipPullImagesFlag, "skip-pull-images", false, "Skip pulling images (assume pre-loaded)")
	rootCmd.PersistentFlags().BoolVar(&securityEnhancementFlag, "security-enhancement", false, "Apply additional security enhancements")
	rootCmd.PersistentFlags().BoolVar(&skipInstallAddonsFlag, "skip-install-addons", false, "Skip installing default addons")

	// Add subcommands
	rootCmd.AddCommand(createCmd)
	createCmd.AddCommand(createClusterCmd)

	// Flags for createClusterCmd
	createClusterCmd.Flags().StringVarP(&clusterConfigFilePath, "file", "f", "", "Path to the cluster configuration YAML file (required)")
	if err := createClusterCmd.MarkFlagRequired("file"); err != nil {
		fmt.Fprintf(os.Stderr, "Error marking 'file' flag as required: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
