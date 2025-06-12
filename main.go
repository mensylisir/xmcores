package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mensylisir/xmcores/common" // For logger field constants
	"github.com/mensylisir/xmcores/connector" // Required for Host definition if used directly
	"github.com/mensylisir/xmcores/logger"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/task/containerd" // Example: Using the InstallContainerdTask
	"github.com/sirupsen/logrus"
)

func main() {
	// 1. Initialize Logger (global logger.Log is initialized in its package's init())
	// Allow overriding log level via flag for debugging.
	logLevelStr := flag.String("log-level", "info", "Log level (trace, debug, info, warn, error, fatal, panic)")
	verbose := flag.Bool("verbose", false, "Enable verbose (debug) logging (shorthand for -log-level=debug)")

	// --- Configuration Flags (Example for InstallContainerdTask) ---
	containerdVersion := flag.String("containerd-version", "1.7.13", "Containerd version to install")
	targetArch := flag.String("arch", "amd64", "Target architecture (amd64, arm64)")
	useSystemdCgroup := flag.Bool("systemd-cgroup", true, "Use systemd as cgroup driver for containerd")
	sandboxImage := flag.String("sandbox-image", "registry.k8s.io/pause:3.9", "Sandbox (pause) image for containerd")

	// Host configuration (simplified for this example)
	// In a real app, this would come from a config file or more detailed flags
	hostAddr := flag.String("host-addr", "", "Target host address (e.g., 192.168.1.100)")
	hostUser := flag.String("host-user", "root", "Target host user")
	hostPort := flag.Int("host-port", 22, "Target host SSH port")
	hostPassword := flag.String("host-password", "", "Target host password (not recommended for production)")
	hostKeyPath := flag.String("host-key", "", "Path to SSH private key for target host")

	workDir := flag.String("work-dir", "./.xm_work_data", "Working directory for downloads and temporary files")
	ignoreErrors := flag.Bool("ignore-errors", false, "Ignore errors in steps and continue task/module execution")

	flag.Parse()

	// Apply log level from flag
	if *verbose && *logLevelStr == "info" { // if -verbose is set and -log-level is default
		*logLevelStr = "debug"
	}
	level, err := logrus.ParseLevel(*logLevelStr)
	if err != nil {
		logger.Log.Warnf("Invalid log level '%s', defaulting to 'info'. Error: %v", *logLevelStr, err)
		logger.Log.SetLevel(logrus.InfoLevel)
	} else {
		logger.Log.SetLevel(level)
	}

	mainLog := logger.Log.WithField(common.LogFieldApp, "xmcores-cli") // Main application logger entry
	mainLog.Infof("Application starting with log level: %s", logger.Log.GetLevel().String())

	// 2. Create Runtime (Simplified Example)
	if *hostAddr == "" {
		mainLog.Error("Target host address (-host-addr) is required to run the example InstallContainerdTask.")
		fmt.Println("\nUsage example for InstallContainerdTask:")
		fmt.Printf("  go run main.go -host-addr=<ip_address> [-host-user=<user>] [-host-key=<path_to_ssh_key>] [-containerd-version=%s]\n", *containerdVersion)
		os.Exit(1)
	}

	// Create a single host for the example
	// In a real scenario, you'd parse multiple hosts from a config file.
	exampleHost := connector.NewHost()
	exampleHost.SetName("target1") // Name is important for caching in runtime
	exampleHost.SetAddress(*hostAddr)
	exampleHost.SetPort(*hostPort)
	exampleHost.SetUser(*hostUser)
	if *hostPassword != "" {
		exampleHost.SetPassword(*hostPassword)
	}
	if *hostKeyPath != "" {
		exampleHost.SetPrivateKeyPath(*hostKeyPath)
	}
	// It's good practice to validate the host object
	if err := exampleHost.Validate(); err != nil {
		mainLog.Fatalf("Invalid host configuration for %s: %v", exampleHost.GetName(), err)
	}

	rtConfig := runtime.Config{
		AllHosts:         []connector.Host{exampleHost}, // Runtime expects a slice of connector.Host
		RoleHosts:        map[string][]connector.Host{"all": {exampleHost}}, // Example role
		Verbose:          *verbose || logger.Log.GetLevel() == logrus.DebugLevel || logger.Log.GetLevel() == logrus.TraceLevel,
		IgnoreError:      *ignoreErrors,
		WorkDir:          *workDir,
		ObjectName:       "InstallContainerdExample", // Name for this execution context
		// PrimaryConnector and PrimaryRunner can be nil if not used directly by top-level logic
	}
	rt, err := runtime.NewRuntime(rtConfig)
	if err != nil {
		mainLog.Fatalf("Failed to initialize runtime: %v", err)
	}
	mainLog.Info("Runtime initialized successfully.")

	// 3. Instantiate the Task
	mainLog.Infof("Preparing to run InstallContainerdTask for version %s on host %s", *containerdVersion, *hostAddr)
	installTask := containerd.NewInstallContainerdTask(
		*containerdVersion,
		*targetArch,
		*useSystemdCgroup,
		*sandboxImage,
	)
	// We can further configure the task if its struct fields are public or via setters
	// For example, if InstallContainerdTask has a field `ReloadSystemdOnEnable`:
	// if itask, ok := installTask.(*containerd.InstallContainerdTask); ok {
	// itask.ReloadSystemdOnEnable = true // or from a flag
	// }

	taskLog := mainLog.WithFields(logrus.Fields{
		common.LogFieldTaskName: installTask.Name(),
	})

	// 4. Initialize and Execute the Task
	taskLog.Info("Initializing task...")
	if err := installTask.Init(rt, taskLog); err != nil {
		taskLog.Fatalf("Failed to initialize task '%s': %v", installTask.Name(), err)
	}
	taskLog.Info("Task initialized successfully.")

	taskLog.Info("Executing task...")
	executeErr := installTask.Execute(rt, taskLog)
	if executeErr != nil {
		taskLog.Errorf("Task '%s' execution failed: %v", installTask.Name(), executeErr)
		// Post execution even on failure
		taskLog.Info("Running post-execution steps for task (due to error)...")
		if postErr := installTask.Post(rt, taskLog, executeErr); postErr != nil {
			taskLog.Errorf("Error during post-execution of task '%s': %v", installTask.Name(), postErr)
		}
		mainLog.Fatalf("Task execution resulted in failure.") // Ensure main exits with error status
	} else {
		taskLog.Info("Task execution completed successfully.")
		// Post execution on success
		taskLog.Info("Running post-execution steps for task (successful execution)...")
		if postErr := installTask.Post(rt, taskLog, nil); postErr != nil {
			taskLog.Errorf("Error during post-execution of task '%s': %v", installTask.Name(), postErr)
			mainLog.Errorf("Post-execution steps failed.") // May not be fatal for overall success
		}
	}

	mainLog.Info("Application finished.")
}
