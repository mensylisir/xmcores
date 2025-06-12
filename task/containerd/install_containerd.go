package containerd

import (
	"fmt"
	"path/filepath" // For joining paths for tarball and extraction
	"strings"

	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	containerdSteps "github.com/mensylisir/xmcores/step/containerd" // Alias to avoid name collision
	"github.com/mensylisir/xmcores/task"                             // Import the task package
	"github.com/sirupsen/logrus"
)

// InstallContainerdTask orchestrates the steps to install containerd.
type InstallContainerdTask struct {
	task.BaseTask // Embed BaseTask

	// Configuration for the containerd installation
	Version               string
	Arch                  string
	DownloadDir           string // If empty, DownloadStep will use a default from runtime.WorkDir()
	DownloadFilename      string // If empty, DownloadStep will derive it from URL
	DownloadURL           string // Optional: Override download URL in DownloadStep
	ExtractDir            string // If empty, UntarStep will use a default based on tarball name in runtime.WorkDir()
	DeliveryTargetDir     string // Default: /usr/local/bin
	ContainerdBinPath     string // Calculated based on DeliveryTargetDir, default: /usr/local/bin/containerd
	SystemdCgroup         bool
	SandboxImage          string
	ReloadSystemdOnEnable bool // For EnableContainerdStep
	EnableNowOnEnable     bool // For EnableContainerdStep
	StartService          bool // Explicitly start service if not enabled now
}

// NewInstallContainerdTask creates a new task for installing containerd.
func NewInstallContainerdTask(version, arch string, systemdCgroup bool, sandboxImage string) task.Task {
	// Sensible defaults
	ict := &InstallContainerdTask{
		Version:               version,
		Arch:                  arch,
		DeliveryTargetDir:     "/usr/local/bin", // Common default
		SystemdCgroup:         systemdCgroup,
		SandboxImage:          sandboxImage,
		ReloadSystemdOnEnable: true, // Usually needed after new service file
		EnableNowOnEnable:     true, // Common to enable and start
		StartService:          true, // Fallback if EnableNow is false
	}
	// Set BaseTask fields
	ict.SetName("InstallContainerd")
	ict.SetDescription(fmt.Sprintf("Install containerd version %s for %s", version, arch))
	// ContainerdBinPath will be set in Init based on DeliveryTargetDir
	return ict
}

// Init initializes the task, setting up all the necessary steps.
func (t *InstallContainerdTask) Init(rt runtime.Runtime, log *logrus.Entry) error {
	log.Infof("Initializing InstallContainerdTask: Version=%s, Arch=%s, SystemdCgroup=%t", t.Version, t.Arch, t.SystemdCgroup)

	if t.Version == "" {
		return fmt.Errorf("containerd version is required for InstallContainerdTask")
	}
	if t.Arch == "" {
		return fmt.Errorf("CPU architecture is required for InstallContainerdTask")
	}
	if t.SandboxImage == "" {
		// Set a default if not provided by constructor or specific setter
		t.SandboxImage = "registry.k8s.io/pause:3.9"
		log.Warnf("SandboxImage not specified, defaulting to %s", t.SandboxImage)
	}

	// Determine actual ContainerdBinPath based on DeliveryTargetDir
	// Use filepath.Join for OS-agnostic path construction, though remote paths are usually POSIX.
	// For remote paths, direct string concatenation or path.Join might be more appropriate
	// if DeliveryTargetDir is always a POSIX path. Given it's a remote path, path.Join is better.
	t.ContainerdBinPath = filepath.ToSlash(filepath.Join(t.DeliveryTargetDir, "containerd"))
	log.Debugf("Containerd binary path set to: %s", t.ContainerdBinPath)

	// --- Step Instantiation & Configuration ---

	// Step 1: Download Containerd
	downloadStepConcrete := containerdSteps.NewDownloadContainerdStep(t.Version, t.Arch, t.DownloadDir).(*containerdSteps.DownloadContainerdStep)
	if t.DownloadURL != "" {
		downloadStepConcrete.DownloadURL = t.DownloadURL
	}
	if t.DownloadFilename != "" {
		downloadStepConcrete.TargetFilename = t.DownloadFilename
	}

	tempStepInitLog := log.WithField("pre_init_step", downloadStepConcrete.Name())
	if err := downloadStepConcrete.Init(rt, tempStepInitLog); err != nil {
		return fmt.Errorf("pre-initialization of download step failed: %w", err)
	}
	tarballPath := filepath.Join(downloadStepConcrete.DownloadDir, downloadStepConcrete.TargetFilename)
	log.Debugf("Calculated tarball path: %s", tarballPath)


	// Step 2: Untar Containerd
	untarStepConcrete := containerdSteps.NewUntarContainerdStep(tarballPath, t.ExtractDir).(*containerdSteps.UntarContainerdStep)
	tempStepInitLog = log.WithField("pre_init_step", untarStepConcrete.Name())
	if err := untarStepConcrete.Init(rt, tempStepInitLog); err != nil {
		return fmt.Errorf("pre-initialization of untar step failed: %w", err)
	}
	actualExtractDir := untarStepConcrete.ExtractDir
	log.Debugf("Calculated actual extract directory: %s", actualExtractDir)


	// Step 3: Delivery Containerd binaries
	deliverySourceDir := actualExtractDir
	deliveryStep := containerdSteps.NewDeliveryContainerdStep(deliverySourceDir, t.DeliveryTargetDir, containerdSteps.DefaultContainerdBinaries)
	// DeliveryStep's SubPathInSource defaults to "bin", which is usually correct for containerd archives.

	// Step 4: Chmod Containerd binaries
	chmodStep := containerdSteps.NewChmodContainerdStep(t.DeliveryTargetDir, containerdSteps.DefaultContainerdBinaries, 0755)

	// Step 5: Generate systemd service file
	// Ensure ContainerdBinPath used here is the final remote path
	generateServiceStep := containerdSteps.NewGenerateServiceContainerdStep(t.ContainerdBinPath)

	// Step 6: Configure Containerd (config.toml)
	configureStep := containerdSteps.NewConfigureContainerdStep(t.SystemdCgroup, t.SandboxImage)

	// Step 7: Enable Containerd service (and optionally start)
	enableStep := containerdSteps.NewEnableContainerdStep(t.ReloadSystemdOnEnable, t.EnableNowOnEnable)

	allSteps := []step.Step{
		downloadStepConcrete,
		untarStepConcrete,
		deliveryStep,
		chmodStep,
		generateServiceStep,
		configureStep,
		enableStep,
	}

	if t.StartService && !t.EnableNowOnEnable {
		startStep := containerdSteps.NewStartContainerdStep()
		allSteps = append(allSteps, startStep)
		log.Debug("StartContainerdStep added as EnableNowOnEnable is false and StartService is true.")
	}

	t.SetSteps(allSteps)

	return t.BaseTask.Init(rt, log)
}

// Execute uses the BaseTask's Execute method by default.
// No need to override if BaseTask.Execute is sufficient.

// Post uses the BaseTask's Post method by default.
// No need to override if BaseTask.Post is sufficient.
