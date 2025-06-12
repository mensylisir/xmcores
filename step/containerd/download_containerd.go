package containerd

import (
	"context" // Added for context.Background()
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/logger" // Using the global logger for initial messages
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step" // Import the step package
	"github.com/sirupsen/logrus"
)

const (
	DefaultContainerdDownloadURLTemplate = "https://github.com/containerd/containerd/releases/download/v%s/containerd-%s-linux-%s.tar.gz"
)

// DownloadContainerdStep handles the download of containerd binaries.
type DownloadContainerdStep struct {
	step.BaseStep
	Version          string // e.g., "1.7.12"
	Arch             string // e.g., "amd64", "arm64"
	DownloadDir      string // Directory to download the tarball to
	TargetFilename   string // Optional: specify a target filename. If empty, derived from URL.
	DownloadURL      string // Optional: override the default URL
	localExecutor    executor.Executor
}

// NewDownloadContainerdStep creates a new DownloadContainerdStep.
func NewDownloadContainerdStep(version, arch, downloadDir string) step.Step {
	return &DownloadContainerdStep{
		BaseStep:    step.NewBaseStep("DownloadContainerd", "Download Containerd binaries"),
		Version:     version,
		Arch:        arch,
		DownloadDir: downloadDir,
	}
}

func (s *DownloadContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.Version == "" {
		return fmt.Errorf("containerd version cannot be empty for DownloadContainerd step")
	}
	if s.Arch == "" {
		return fmt.Errorf("architecture cannot be empty for DownloadContainerd step")
	}
	if s.DownloadDir == "" {
		s.DownloadDir = filepath.Join(rt.WorkDir(), "downloads") // Default to a subdirectory in runtime's workdir
	}

	if s.DownloadURL == "" {
		s.DownloadURL = fmt.Sprintf(DefaultContainerdDownloadURLTemplate, s.Version, s.Version, s.Arch)
	}

	if s.TargetFilename == "" {
		urlParts := strings.Split(s.DownloadURL, "/")
		s.TargetFilename = urlParts[len(urlParts)-1]
	}

	s.localExecutor = executor.NewLocalExecutor()

	log.Infof("DownloadContainerdStep initialized: Version=%s, Arch=%s, URL=%s, DownloadDir=%s, Filename=%s",
		s.Version, s.Arch, s.DownloadURL, s.DownloadDir, s.TargetFilename)
	return nil
}

func (s *DownloadContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting DownloadContainerd step: %s", s.Description())

	targetPath := filepath.Join(s.DownloadDir, s.TargetFilename)

	// Using context.Background() for local operations not tied to a specific remote context
	ctx := context.Background()

	// Ensure download directory exists
	if err := s.localExecutor.CreateRemoteDirectory(ctx, s.DownloadDir, 0755); err != nil {
		return fmt.Sprintf("Failed to create download directory %s", s.DownloadDir), false, fmt.Errorf("failed to create download directory %s: %w", s.DownloadDir, err)
	}

	// Check if file already exists
	exists, err := s.localExecutor.RemoteFileExists(ctx, targetPath)
	if err != nil {
		log.Warnf("Failed to check if file %s exists: %v. Proceeding with download.", targetPath, err)
	}
	if exists {
		log.Infof("File %s already exists. Skipping download.", targetPath)
		// Optionally, add checksum validation here if it exists
		return fmt.Sprintf("File %s already exists. Skipped download.", targetPath), true, nil
	}

	log.Infof("Downloading from %s to %s", s.DownloadURL, targetPath)

	// Create the file
	out, err := os.Create(targetPath)
	if err != nil {
		return fmt.Sprintf("Failed to create file %s for download", targetPath), false, fmt.Errorf("failed to create file %s: %w", targetPath, err)
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(s.DownloadURL)
	if err != nil {
		// Attempt to remove partially downloaded file
		_ = os.Remove(targetPath)
		return fmt.Sprintf("Failed to download from %s", s.DownloadURL), false, fmt.Errorf("failed to HTTP GET %s: %w", s.DownloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Attempt to remove partially downloaded file
		_ = os.Remove(targetPath)
		return fmt.Sprintf("Download failed: server returned status %s for %s", resp.Status, s.DownloadURL), false, fmt.Errorf("bad status: %s from %s", resp.Status, s.DownloadURL)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		// Attempt to remove partially downloaded file
		_ = os.Remove(targetPath)
		return fmt.Sprintf("Failed to write downloaded content to %s", targetPath), false, fmt.Errorf("failed to write to file %s: %w", targetPath, err)
	}

	// Explicitly sync to disk
	err = out.Sync()
    if err != nil {
        log.Warnf("Failed to sync downloaded file %s to disk: %v", targetPath, err)
        // Not necessarily a fatal error for the step, but good to log
    }

	log.Infof("Successfully downloaded %s to %s", s.TargetFilename, targetPath)
	return fmt.Sprintf("Successfully downloaded %s", s.TargetFilename), true, nil
}

// Post implementation (optional, can be default from BaseStep if no specific cleanup is needed)
// func (s *DownloadContainerdStep) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
//	 log.Debugf("DownloadContainerdStep.Post called for %s", s.Name())
//	 return s.BaseStep.Post(rt, log, executeErr)
// }
