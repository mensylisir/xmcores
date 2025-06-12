package containerd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"text/template" // For more complex configurations if needed

	"github.com/mensylisir/xmcores/connector" // Required for connector.Host
	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

const (
	DefaultContainerdConfigDirPath  = "/etc/containerd"
	DefaultContainerdConfigFilePath = "/etc/containerd/config.toml"
	// A basic default config.toml.
	// For production, this might be more complex and templated.
	// Key things:
	// - version = 2 (current config format)
	// - SystemdCgroup = true (if using systemd as cgroup driver, common for k8s)
	// - pause image (sandbox_image)
	// - registry mirrors
	// - plugin."io.containerd.grpc.v1.cri".containerd_runtime_engine.snapshotter (usually "overlayfs")
	DefaultContainerdConfigTemplate = `version = 2
root = "/var/lib/containerd"
state = "/run/containerd"
oom_score = -999

[grpc]
  address = "/run/containerd/containerd.sock"
  uid = 0
  gid = 0
  max_recv_message_size = 16777216
  max_send_message_size = 16777216

[debug]
  address = "" #  "/run/containerd/containerd-debug.sock"
  uid = 0
  gid = 0
  level = "" # "info" or "debug"

[metrics]
  address = "" # "127.0.0.1:1338"
  grpc_histogram = false

[cgroup]
  path = "" # For systemd cgroup driver, this should be empty. For cgroupfs, typically "/containerd" or similar.

[plugins]
  [plugins."io.containerd.grpc.v1.cri"]
    sandbox_image = "{{.SandboxImage}}"
    [plugins."io.containerd.grpc.v1.cri".containerd]
      default_runtime_name = "runc"
      snapshotter = "overlayfs" # Ensure this is supported by the kernel
      [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
        [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
          runtime_type = "io.containerd.runc.v2"
          [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
            SystemdCgroup = {{.SystemdCgroup}}
    [plugins."io.containerd.grpc.v1.cri".cni]
      bin_dir = "/opt/cni/bin"
      conf_dir = "/etc/cni/net.d"
      # max_conf_num = 1 # Max number of CNI config files to load
      # conf_template = "" # Path to CNI config template
    # Add registry mirror configuration if needed
    # [plugins."io.containerd.grpc.v1.cri".registry]
    #   [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    #     [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
    #       endpoint = ["https://<your-mirror-for-docker-io>"]
    #     [plugins."io.containerd.grpc.v1.cri".registry.mirrors."k8s.gcr.io"]
    #       endpoint = ["https://<your-mirror-for-k8s-gcr-io>"]
    #   [plugins."io.containerd.grpc.v1.cri".registry.configs]
    #     # Add auth configs for private registries if needed
    #     [plugins."io.containerd.grpc.v1.cri".registry.configs."your.private.registry"]
    #       [plugins."io.containerd.grpc.v1.cri".registry.configs."your.private.registry".auth]
    #         username = "your_username"
    #         password = "your_password"

`
)

// ConfigureContainerdStep creates the config.toml file for containerd.
type ConfigureContainerdStep struct {
	step.BaseStep
	ConfigDirPath       string // Remote path for the containerd config directory
	ConfigFilePath      string // Remote path for the config.toml file
	ConfigFileTemplate  string // Template for the config.toml content
	SystemdCgroup       bool   // Whether to use systemd cgroup driver
	SandboxImage        string // Pause image, e.g., "registry.k8s.io/pause:3.9"
	// Add other fields for registry mirrors, private registry configs etc. as needed
}

// NewConfigureContainerdStep creates a new ConfigureContainerdStep.
func NewConfigureContainerdStep(systemdCgroup bool, sandboxImage string) step.Step {
	// A more robust default sandbox image would be ideal, or make it mandatory.
	// For example, "registry.aliyuncs.com/google_containers/pause:3.9" for users in China.
	// Or "registry.k8s.io/pause:3.9"
	defaultSandboxImage := "registry.k8s.io/pause:3.9"
	if sandboxImage != "" {
		defaultSandboxImage = sandboxImage
	}

	return &ConfigureContainerdStep{
		BaseStep:           step.NewBaseStep("ConfigureContainerd", "Generate config.toml for Containerd"),
		ConfigDirPath:      DefaultContainerdConfigDirPath,
		ConfigFilePath:     DefaultContainerdConfigFilePath,
		ConfigFileTemplate: DefaultContainerdConfigTemplate,
		SystemdCgroup:      systemdCgroup,
		SandboxImage:       defaultSandboxImage,
	}
}

func (s *ConfigureContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.ConfigDirPath == "" {
		return fmt.Errorf("config directory path cannot be empty")
	}
	if s.ConfigFilePath == "" {
		return fmt.Errorf("config file path cannot be empty")
	}
	if s.ConfigFileTemplate == "" {
		return fmt.Errorf("config file template cannot be empty")
	}
	if s.SandboxImage == "" {
		return fmt.Errorf("sandbox image (pause image) cannot be empty")
	}

	log.Infof("ConfigureContainerdStep initialized: ConfigFilePath=%s, SystemdCgroup=%t, SandboxImage=%s",
		s.ConfigFilePath, s.SystemdCgroup, s.SandboxImage)
	return s.BaseStep.Init(rt, log)
}

func (s *ConfigureContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting ConfigureContainerd step: %s", s.Description())
	ctx := context.Background()

	tmpl, err := template.New("containerdConfig").Parse(s.ConfigFileTemplate)
	if err != nil {
		return "Failed to parse config file template", false, fmt.Errorf("error parsing containerd config template: %w", err)
	}

	templateData := struct {
		SystemdCgroup bool
		SandboxImage  string
	}{
		SystemdCgroup: s.SystemdCgroup,
		SandboxImage:  s.SandboxImage,
	}

	var configFileContentBuffer bytes.Buffer
	if err := tmpl.Execute(&configFileContentBuffer, templateData); err != nil {
		return "Failed to execute config file template", false, fmt.Errorf("error executing containerd config template: %w", err)
	}
	configFileContent := configFileContentBuffer.String()

	processingTracker := struct {
		mu             sync.Mutex
		errors         []string
		outputs        []string
		overallSuccess bool
	}{
		overallSuccess: true,
	}
	var wg sync.WaitGroup

	targetHosts := rt.AllHosts()
	if len(targetHosts) == 0 {
		log.Warn("No target hosts specified in runtime. Skipping config generation.")
		return "No target hosts specified, skipping config generation.", true, nil
	}

	for _, hostEntry := range targetHosts {
		wg.Add(1)
		go func(host connector.Host) { // Use connector.Host from import
			defer wg.Done()
			hostLog := log.WithField("host", host.GetName())

			hostConn, hostRunner, err := rt.GetHostConnectorAndRunner(host)
			if err != nil {
				err := fmt.Errorf("host %s (%s): failed to get connector/runner: %w", host.GetName(), host.GetAddress(), err)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				return
			}
			defer func() {
				if err_close := hostConn.Close(); err_close != nil {
					hostLog.Warnf("Error closing connection: %v", err_close)
				}
			}()

			hostLog.Debugf("Successfully obtained connector and runner for host %s", host.GetName())

			remoteExec, err := executor.NewRemoteExecutor(hostConn, hostRunner)
			if err != nil {
				err := fmt.Errorf("host %s (%s): failed to create remote executor: %w", host.GetName(), host.GetAddress(), err)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				return
			}

			// Ensure config directory exists
			hostLog.Infof("Ensuring config directory %s exists on host %s", s.ConfigDirPath, host.GetName())
			if err := remoteExec.CreateRemoteDirectory(ctx, s.ConfigDirPath, 0755); err != nil {
				err := fmt.Errorf("host %s (%s): failed to create config directory %s: %w", host.GetName(), host.GetAddress(), s.ConfigDirPath, err)
				hostLog.Error(err.Error())
				processingTracker.mu.Lock()
				processingTracker.errors = append(processingTracker.errors, err.Error())
				processingTracker.overallSuccess = false
				processingTracker.mu.Unlock()
				// if !rt.IgnoreError() { return } // This logic is complex with goroutines
			}

			// Write the config file only if directory creation was successful or if we are ignoring errors
			if err == nil || rt.IgnoreError() { // Check if previous critical error occurred for this host
				hostLog.Infof("Writing containerd config file to %s on host %s", s.ConfigFilePath, host.GetName())
				contentReader := strings.NewReader(configFileContent)
				// Using Scp from connector as it takes io.Reader and mode
				// Permissions for config.toml should be 0644
				configErr := hostConn.Scp(ctx, contentReader, s.ConfigFilePath, int64(len(configFileContent)), 0644)

				if configErr != nil {
					err_scp := fmt.Errorf("host %s (%s): failed to write config file %s: %w", host.GetName(), host.GetAddress(), s.ConfigFilePath, configErr)
					hostLog.Error(err_scp.Error())
					processingTracker.mu.Lock()
					processingTracker.errors = append(processingTracker.errors, err_scp.Error())
					processingTracker.overallSuccess = false
					processingTracker.mu.Unlock()
				} else {
					msg := fmt.Sprintf("Host %s (%s): Successfully wrote config file to %s", host.GetName(), host.GetAddress(), s.ConfigFilePath)
					hostLog.Info(msg)
					processingTracker.mu.Lock()
					processingTracker.outputs = append(processingTracker.outputs, msg)
					processingTracker.mu.Unlock()
				}
			}
		}(hostEntry)
	}
	wg.Wait()

	finalOutput := strings.Join(processingTracker.outputs, "\n")
	if len(processingTracker.errors) > 0 {
		errorSummary := strings.Join(processingTracker.errors, "\n")
		finalOutput = fmt.Sprintf("Completed with errors.\nOutputs:\n%s\nErrors:\n%s", finalOutput, errorSummary)
		log.Errorf("ConfigureContainerd step completed with errors:\n%s", errorSummary)
		return finalOutput, processingTracker.overallSuccess, fmt.Errorf("one or more errors occurred during config file generation: %s", errorSummary)
	}

	log.Infof("ConfigureContainerd step completed successfully for all processed hosts.")
	return finalOutput, true, nil
}
