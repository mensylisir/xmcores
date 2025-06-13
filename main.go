package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mensylisir/xmcores/common"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/logger"
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	_ "github.com/mensylisir/xmcores/pipeline/kubernetes"
)

var (
	// Global operational flags
	logLevelFlag     string
	verboseFlag      bool
	workDirFlag      string
	ignoreErrorsFlag bool

	// Flags for runtime.CliArgs
	artifactFlag               string
	skipPushImagesFlag         bool
	deployLocalStorageFlagActual bool
	installPackagesFlag        bool
	skipPullImagesFlag         bool
	securityEnhancementFlag    bool
	skipInstallAddonsFlag      bool

	clusterConfigFilePath string
)

var rootCmd = &cobra.Command{
	Use:   "xm",
	Short: "xm is a cluster manager CLI tool",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		effectiveLogLevel := "info"
		userSetLogLevel := cmd.Flags().Changed("log-level")

		if userSetLogLevel && logLevelFlag != "" {
			effectiveLogLevel = logLevelFlag
		}
		if verboseFlag {
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

var createCmd = &cobra.Command{Use: "create", Short: "Create various resources"}

var createClusterCmd = &cobra.Command{
	Use:     "cluster",
	Short:   "Create a Kubernetes cluster",
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

	// 1. Load raw configuration using config.Loader
	loader := config.NewLoader(clusterConfigFilePath)
	rawCfg, err := loader.Load()
	if err != nil {
		appLog.Errorf("Failed to load cluster configuration: %v", err)
		return fmt.Errorf("failed to load cluster configuration from '%s': %w", clusterConfigFilePath, err)
	}
	appLog.Infof("Successfully loaded raw configuration for cluster: %s (API: %s, Kind: %s)",
		rawCfg.Metadata.Name, rawCfg.APIVersion, rawCfg.Kind)

	// 2. Populate CliArgs from global flags
	cliArgs := runtime.NewCliArgs()
	cliArgs.Artifact = artifactFlag
	cliArgs.SkipPushImages = skipPushImagesFlag
	cliArgs.InstallPackages = installPackagesFlag
	cliArgs.SkipPullImages = skipPullImagesFlag
	cliArgs.SecurityEnhancement = securityEnhancementFlag
	cliArgs.SkipInstallAddons = skipInstallAddonsFlag
	cliArgs.Debug = verboseFlag
	cliArgs.IgnoreErr = ignoreErrorsFlag

	if cmd.Flags().Changed("deploy-local-storage") {
		cliArgs.DeployLocalStorage = &deployLocalStorageFlagActual
	}
	appLog.Debugf("Collected CliArgs: %+v", cliArgs)
	if cliArgs.DeployLocalStorage != nil {
		appLog.Debugf("CliArgs.DeployLocalStorage explicitly set by user to: %v", *cliArgs.DeployLocalStorage)
	}

	// 3. Initialize ClusterRuntime
	// NewClusterRuntime now takes rawCfg and cliArgs (which contains Debug and IgnoreErr internally used by BaseRuntime)
	// It will internally call SetDefaultClusterSpec.
	clusterRt, err := runtime.NewClusterRuntime(
		rawCfg,
		cliArgs,
		workDirFlag, // workDir is still passed directly as it's a global operational param not specific to a single pipeline's "args"
		logger.Log,
	)
	if err != nil {
		appLog.Errorf("Failed to initialize ClusterRuntime: %v", err)
		return fmt.Errorf("failed to initialize ClusterRuntime: %w", err)
	}
	appLog.Info("ClusterRuntime initialized successfully.")
	// ClusterRuntime now has its own scoped logger: clusterRt.Log
	// and the defaulted cluster spec: clusterRt.Cluster (*config.ClusterSpec)

	// 4. Pipeline Selection & Instantiation
	pipelineName := "cluster-install"
	appLog.Infof("Attempting to get and initialize pipeline: '%s'", pipelineName)

	selectedPipeline, err := pipeline.GetPipeline(pipelineName, clusterRt) // Pass ClusterRuntime
	if err != nil {
		availablePipelines := pipeline.GetRegisteredPipelineNames()
		errMsg := fmt.Sprintf("Failed to get pipeline '%s': %v.", pipelineName, err)
		if len(availablePipelines) > 0 {
			errMsg += fmt.Sprintf(" Available registered pipelines: %s", strings.Join(availablePipelines, ", "))
		} else {
			errMsg += " No pipelines are currently registered."
		}
		if selectedPipeline == nil { // Check if factory itself failed
		    errMsg += " Ensure the pipeline factory for 'cluster-install' is correctly implemented."
		}
		appLog.Error(errMsg)
		return fmt.Errorf(errMsg)
	}
	appLog.Infof("Successfully obtained and initialized pipeline: %s (%s)", selectedPipeline.Name(), selectedPipeline.Description())

	// 5. Call Pipeline Start()
	// The logger for the pipeline execution is taken from ClusterRuntime.Log
	pipelineExecutionLogger := clusterRt.Log.WithField("pipeline_execution_id", selectedPipeline.Name())
	appLog.Infof("Starting pipeline execution: %s", selectedPipeline.Name())

	if err = selectedPipeline.Start(pipelineExecutionLogger); err != nil {
		appLog.Errorf("Pipeline '%s' execution failed: %v", selectedPipeline.Name(), err)
		return fmt.Errorf("pipeline '%s' execution failed: %w", selectedPipeline.Name(), err)
	}

	appLog.Infof("Pipeline '%s' executed successfully.", selectedPipeline.Name())
	appLog.Info("'create cluster' command finished successfully.")
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "", "Log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose (debug) logging (maps to CliArgs.Debug)")
	rootCmd.PersistentFlags().StringVar(&workDirFlag, "work-dir", "./.xm_work_data", "Working directory")
	rootCmd.PersistentFlags().BoolVar(&ignoreErrorsFlag, "ignore-errors", false, "Ignore errors in execution (maps to CliArgs.IgnoreErr)")

	rootCmd.PersistentFlags().StringVar(&artifactFlag, "artifact", "", "Path to artifact (e.g., offline package tarball)")
	rootCmd.PersistentFlags().BoolVar(&skipPushImagesFlag, "skip-push-images", false, "Skip pushing images to a private registry")
	rootCmd.PersistentFlags().BoolVar(&deployLocalStorageFlagActual, "deploy-local-storage", false, "Deploy local storage provisioner")
	rootCmd.PersistentFlags().BoolVar(&installPackagesFlag, "install-packages", true, "Allow installation of OS packages")
	rootCmd.PersistentFlags().BoolVar(&skipPullImagesFlag, "skip-pull-images", false, "Skip pulling images (assume pre-loaded)")
	rootCmd.PersistentFlags().BoolVar(&securityEnhancementFlag, "security-enhancement", false, "Apply additional security enhancements")
	rootCmd.PersistentFlags().BoolVar(&skipInstallAddonsFlag, "skip-install-addons", false, "Skip installing default addons")

	rootCmd.AddCommand(createCmd)
	createCmd.AddCommand(createClusterCmd)

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
