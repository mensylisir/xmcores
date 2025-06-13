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
	_ "github.com/mensylisir/xmcores/pipeline/kubernetes" // Corrected import path
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
		effectiveLogLevel := "info"

		userSetLogLevel := false
		if cmd.Flags().Changed("log-level") {
			userSetLogLevel = true
		}

		if userSetLogLevel && logLevelFlag != "" {
			effectiveLogLevel = logLevelFlag
		}
		if verboseFlag {
			effectiveLogLevel = "debug"
		}

		level, err := logrus.ParseLevel(effectiveLogLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to 'info'. Error: %v\n", effectiveLogLevel, err)
			logger.Log.SetLevel(logrus.InfoLevel) // Use the global logger instance
		} else {
			logger.Log.SetLevel(level) // Use the global logger instance
		}
		// Assign the global logger to a package-level variable for easy access if needed,
		// or ensure all parts of the application use logger.Log directly.
		// For now, mainLog in runCreateClusterCmd will derive from logger.Log.
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
	RunE:    runCreateClusterCmd,
}

// runCreateClusterCmd is the actual execution logic for 'create cluster'
func runCreateClusterCmd(cmd *cobra.Command, args []string) error {
	// Use the globally configured logger.Log
	mainLog := logger.Log.WithField(common.LogFieldApp, "xm-create-cluster")
	mainLog.Infof("Starting 'create cluster' command...")

	if clusterConfigFilePath == "" {
		return fmt.Errorf("cluster configuration file path (-f, --file) is required")
	}
	mainLog.Infof("Loading cluster configuration from: %s", clusterConfigFilePath)

	clusterCfg, err := config.LoadClusterConfig(clusterConfigFilePath)
	if err != nil {
		mainLog.Errorf("Failed to load cluster configuration: %v", err)
		return fmt.Errorf("failed to load cluster configuration from '%s': %w", clusterConfigFilePath, err)
	}
	mainLog.Infof("Successfully loaded configuration for cluster: %s (API: %s, Kind: %s)",
		clusterCfg.Metadata.Name, clusterCfg.APIVersion, clusterCfg.Kind)

	pipelineName := "cluster-install"
	mainLog.Infof("Attempting to get pipeline: '%s'", pipelineName)

	selectedPipeline, err := pipeline.GetPipeline(pipelineName)
	if err != nil {
		availablePipelines := pipeline.GetRegisteredPipelineNames()
		errMsg := fmt.Sprintf("Failed to get pipeline '%s': %v.", pipelineName, err)
		if len(availablePipelines) > 0 {
			errMsg += fmt.Sprintf(" Available registered pipelines: %s", strings.Join(availablePipelines, ", "))
		} else {
			errMsg += " No pipelines are currently registered. Ensure the 'cluster-install' pipeline is registered (e.g., via blank import)."
		}
		mainLog.Error(errMsg)
		return fmt.Errorf(errMsg)
	}
	mainLog.Infof("Using pipeline: %s (%s)", selectedPipeline.Name(), selectedPipeline.Description())

	// Initial Runtime Configuration (Global settings only)
	rtCfg := runtime.Config{
		WorkDir:     workDirFlag,
		IgnoreError: ignoreErrorsFlag,
		Verbose:     logger.Log.IsLevelEnabled(logrus.DebugLevel) || logger.Log.IsLevelEnabled(logrus.TraceLevel),
		ObjectName:  clusterCfg.Kind + "-" + clusterCfg.Metadata.Name, // e.g., ClusterConfig-ks-installer-cluster
		AllHosts:    nil, // Pipeline's Init method is responsible for host setup
		RoleHosts:   nil, // Pipeline's Init method is responsible for role setup
	}

	initialRt, err := runtime.NewRuntime(rtCfg)
	if err != nil {
		mainLog.Errorf("Failed to initialize core runtime: %v", err)
		return fmt.Errorf("failed to initialize core runtime: %w", err)
	}
	// It's assumed NewRuntime sets up its own logger based on global logger.Log state,
	// or initialRt.Log() will provide a logger derived from the global one.
	mainLog.Info("Core runtime context created successfully.")

	// Pipeline Initialization
	pipelineLogger := mainLog.WithField("pipeline", selectedPipeline.Name())
	mainLog.Infof("Initializing pipeline: %s", selectedPipeline.Name())
	if err = selectedPipeline.Init(clusterCfg, initialRt, pipelineLogger); err != nil {
		mainLog.Errorf("Failed to initialize pipeline '%s': %v", selectedPipeline.Name(), err)
		return fmt.Errorf("failed to initialize pipeline '%s': %w", selectedPipeline.Name(), err)
	}
	mainLog.Infof("Pipeline '%s' initialized successfully.", selectedPipeline.Name())

	// Execute Pipeline
	mainLog.Infof("Executing pipeline: %s", selectedPipeline.Name())
	// The pipelineLogger is already scoped for the pipeline
	err = selectedPipeline.Execute(pipelineLogger)
	if err != nil {
		mainLog.Errorf("Pipeline '%s' execution failed: %v", selectedPipeline.Name(), err)
		return fmt.Errorf("pipeline '%s' execution failed: %w", selectedPipeline.Name(), err)
	}

	mainLog.Infof("Pipeline '%s' executed successfully.", selectedPipeline.Name())
	mainLog.Info("'create cluster' command finished successfully.")
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "", "Log level (trace, debug, info, warn, error, fatal, panic)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "Enable verbose (debug) logging (shorthand for --log-level=debug)")
	rootCmd.PersistentFlags().StringVar(&workDirFlag, "work-dir", "./.xm_work_data", "Working directory for temporary files and operational data")
	rootCmd.PersistentFlags().BoolVar(&ignoreErrorsFlag, "ignore-errors", false, "Ignore errors in individual steps and continue task/module/pipeline execution")

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
		// Cobra's Execute prints errors from RunE to stderr by default.
		// No need for additional fmt.Fprintf(os.Stderr, "Error: %v\n", err) here.
		os.Exit(1)
	}
}
