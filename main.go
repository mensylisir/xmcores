package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mensylisir/xmcores/common"
	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/logger" // Global logger
	"github.com/mensylisir/xmcores/pipeline"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	// Ensure pipeline implementations are imported to trigger their init() registration functions.
	// For now, we only have InstallKubernetesPipeline.
	_ "github.com/mensylisir/xmcores/pipeline/installkubernetes"
)

var (
	// Global flags
	logLevelFlag     string
	verboseFlag      bool
	workDirFlag      string
	ignoreErrorsFlag bool

	// `create cluster` command specific flags
	clusterConfigFilePath string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "xm",
	Short: "xm is a cluster manager CLI tool",
	Long: `xm (xm_cluster_manager) is a command-line interface tool
for orchestrating cluster lifecycle management, including creation,
deletion, and scaling of Kubernetes clusters.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize logger here based on global flags, so all subcommands benefit
		// Precedence: verboseFlag > logLevelFlag > default "info"
		effectiveLogLevel := "info"
		if logLevelFlag != "" {
			effectiveLogLevel = logLevelFlag
		}
		if verboseFlag {
			effectiveLogLevel = "debug"
		}

		level, err := logrus.ParseLevel(effectiveLogLevel)
		if err != nil {
			// Use fmt.Fprintf for pre-logger errors to ensure visibility
			fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to 'info'. Error: %v\n", effectiveLogLevel, err)
			logger.Log.SetLevel(logrus.InfoLevel)
		} else {
			logger.Log.SetLevel(level)
		}
		// Initial log message after logger is configured
		logger.Log.Infof("xm CLI starting. Log level set to: %s", logger.Log.GetLevel().String())
		return nil
	},
}

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create various resources like clusters",
	Long:  `The create command is used to provision new resources. Currently, it supports creating Kubernetes clusters.`,
}

// createClusterCmd represents the command for creating a Kubernetes cluster
var createClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Create a Kubernetes cluster using a configuration file",
	Long: `Create a Kubernetes cluster based on a specified YAML configuration file.
The configuration file defines all aspects of the cluster, including hosts,
Kubernetes version, CNI plugin, and more.`,
	Example: `  xm create cluster -f path/to/your/cluster-config.yaml`,
	RunE:    runCreateClusterCmd, // Function to execute
}

// runCreateClusterCmd is the actual execution logic for 'create cluster'
func runCreateClusterCmd(cmd *cobra.Command, args []string) error {
	mainLog := logger.Log.WithField(common.LogFieldApp, "xm-create-cluster")
	mainLog.Infof("Starting 'create cluster' command...")

	// Validate that the config file path flag was provided (Cobra's 'Required' should handle this, but good to double check)
	if clusterConfigFilePath == "" {
		return fmt.Errorf("cluster configuration file path (-f, --file) is required")
	}
	mainLog.Infof("Loading cluster configuration from: %s", clusterConfigFilePath)

	// Load ClusterConfig
	clusterCfg, err := config.LoadClusterConfig(clusterConfigFilePath)
	if err != nil {
		mainLog.Errorf("Failed to load cluster configuration: %v", err)
		return fmt.Errorf("failed to load cluster configuration from '%s': %w", clusterConfigFilePath, err)
	}
	mainLog.Infof("Successfully loaded configuration for cluster: %s (API: %s, Kind: %s)",
		clusterCfg.Metadata.Name, clusterCfg.APIVersion, clusterCfg.Kind)

	// Pipeline Selection (Hardcoded for this command)
	pipelineName := "cluster-install" // This command is dedicated to this pipeline
	mainLog.Infof("Attempting to get pipeline: '%s'", pipelineName)

	selectedPipeline, err := pipeline.GetPipeline(pipelineName)
	if err != nil {
		availablePipelines := pipeline.GetRegisteredPipelineNames()
		errMsg := fmt.Sprintf("Failed to get pipeline '%s': %v.", pipelineName, err)
		if len(availablePipelines) > 0 {
			errMsg += fmt.Sprintf(" Available registered pipelines: %s", strings.Join(availablePipelines, ", "))
		} else {
			errMsg += " No pipelines are currently registered. Ensure the 'cluster-install' pipeline is registered."
		}
		mainLog.Error(errMsg)
		return fmt.Errorf(errMsg)
	}
	mainLog.Infof("Using pipeline: %s (%s)", selectedPipeline.Name(), selectedPipeline.Description())

	// Initial Runtime Configuration
	// Global flags (workDirFlag, ignoreErrorsFlag, verboseFlag for Verbose) are already parsed by Cobra
	// and their values are in the global variables.
	rtCfg := runtime.Config{
		WorkDir:     workDirFlag,
		IgnoreError: ignoreErrorsFlag,
		Verbose:     logger.Log.IsLevelEnabled(logrus.DebugLevel) || logger.Log.IsLevelEnabled(logrus.TraceLevel), // Derived from effective log level
		ObjectName:  clusterCfg.Kind + "-" + clusterCfg.Metadata.Name,                                           // e.g., ClusterConfig-ks-installer-cluster
		// AllHosts and RoleHosts are not set here; the pipeline is responsible for parsing them from ClusterConfig.Spec
	}

	initialRt, err := runtime.NewRuntime(rtCfg)
	if err != nil {
		mainLog.Errorf("Failed to initialize runtime: %v", err)
		return fmt.Errorf("failed to initialize runtime: %w", err)
	}
	// The global logger.Log is already configured. If runtime needs its own scoped logger instance,
	// it would typically be configured during NewRuntime or via a method.
	// For now, modules/steps will receive logger instances derived from the mainLog passed to pipeline.Execute.
	mainLog.Info("Initial runtime context created successfully.")

	// Prepare configData for the pipeline
	pipelineConfigData := map[string]interface{}{
		"clusterConfig": clusterCfg,
		// Other global parameters derived from flags could be passed here if pipelines expect them at the top level
		// For example, if a pipeline's ExpectedParameters included "globalLogLevelFromCLI":
		// "globalLogLevelFromCLI": logLevelFlag, (though logger is already set globally)
	}

	// Execute Pipeline
	mainLog.Infof("Executing pipeline: %s", selectedPipeline.Name())
	// Pass a logger entry that the pipeline can further scope for its modules/steps
	pipelineLogger := mainLog.WithField("pipeline", selectedPipeline.Name())
	err = selectedPipeline.Execute(initialRt, pipelineConfigData, pipelineLogger)
	if err != nil {
		mainLog.Errorf("Pipeline '%s' execution failed: %v", selectedPipeline.Name(), err)
		return fmt.Errorf("pipeline '%s' execution failed: %w", selectedPipeline.Name(), err) // cobra prints errors returned from RunE
	}

	mainLog.Infof("Pipeline '%s' executed successfully.", selectedPipeline.Name())
	mainLog.Info("'create cluster' command finished successfully.")
	return nil
}

func init() {
	// Bind global flags to rootCmd's persistent flags
	rootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "info", "Log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose (debug) logging (shorthand for --log-level=debug)")
	rootCmd.PersistentFlags().StringVar(&workDirFlag, "work-dir", "./.xm_work_data", "Working directory for temporary files and operational data")
	rootCmd.PersistentFlags().BoolVar(&ignoreErrorsFlag, "ignore-errors", false, "Ignore errors in individual steps and continue task/module/pipeline execution")

	// Add createCmd as a subcommand to rootCmd
	rootCmd.AddCommand(createCmd)

	// Add createClusterCmd as a subcommand to createCmd
	createCmd.AddCommand(createClusterCmd)

	// Define flags for createClusterCmd
	createClusterCmd.Flags().StringVarP(&clusterConfigFilePath, "file", "f", "", "Path to the cluster configuration YAML file (required)")
	if err := createClusterCmd.MarkFlagRequired("file"); err != nil {
		// This error handling is at setup time. If MarkFlagRequired fails, it's a programming error.
		fmt.Fprintf(os.Stderr, "Error marking 'file' flag as required: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra's Execute already prints the error to stderr by default.
		// We might add additional global error handling or logging here if needed.
		// For now, just exit with an error code.
		os.Exit(1)
	}
}
