package containerd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step" // Import the step package
	"github.com/sirupsen/logrus"
)

// UntarContainerdStep handles the extraction of containerd binaries from a tarball.
type UntarContainerdStep struct {
	step.BaseStep
	TarballPath string // Full path to the .tar.gz file
	ExtractDir  string // Directory where contents should be extracted
	localExecutor executor.Executor
}

// NewUntarContainerdStep creates a new UntarContainerdStep.
// tarballPath is the path to the downloaded containerd tar.gz file.
// extractDir is the directory where the binaries will be extracted.
func NewUntarContainerdStep(tarballPath, extractDir string) step.Step {
	return &UntarContainerdStep{
		BaseStep:    step.NewBaseStep("UntarContainerd", "Extract Containerd binaries from tarball"),
		TarballPath: tarballPath,
		ExtractDir:  extractDir,
	}
}

func (s *UntarContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.TarballPath == "" {
		return fmt.Errorf("tarball path cannot be empty for UntarContainerd step")
	}
	if s.ExtractDir == "" {
		// Default to a subdirectory in runtime's workdir, perhaps named after the tarball
		// For example, if TarballPath is /work/downloads/containerd-1.7.12-linux-amd64.tar.gz
		// ExtractDir could be /work/extract/containerd-1.7.12-linux-amd64
		baseName := filepath.Base(s.TarballPath)
		fileNameNoExt := baseName[:len(baseName)-len(filepath.Ext(baseName))] // remove .gz
		if filepath.Ext(fileNameNoExt) == ".tar" {
			fileNameNoExt = fileNameNoExt[:len(fileNameNoExt)-len(filepath.Ext(fileNameNoExt))] // remove .tar
		}
		s.ExtractDir = filepath.Join(rt.WorkDir(), "extract", fileNameNoExt)
	}

	s.localExecutor = executor.NewLocalExecutor()

	log.Infof("UntarContainerdStep initialized: TarballPath=%s, ExtractDir=%s",
		s.TarballPath, s.ExtractDir)
	return s.BaseStep.Init(rt, log) // Call embedded BaseStep's Init
}

func (s *UntarContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting UntarContainerd step: %s", s.Description())
	ctx := context.Background() // Context for local operations

	// Ensure tarball exists
	exists, err := s.localExecutor.RemoteFileExists(ctx, s.TarballPath)
	if err != nil {
		return fmt.Sprintf("Failed to check if tarball %s exists", s.TarballPath), false, fmt.Errorf("failed to check for tarball %s: %w", s.TarballPath, err)
	}
	if !exists {
		return fmt.Sprintf("Tarball %s does not exist", s.TarballPath), false, fmt.Errorf("tarball %s not found", s.TarballPath)
	}

	// Ensure extraction directory exists
	if err := s.localExecutor.CreateRemoteDirectory(ctx, s.ExtractDir, 0755); err != nil {
		return fmt.Sprintf("Failed to create extraction directory %s", s.ExtractDir), false, fmt.Errorf("failed to create extraction directory %s: %w", s.ExtractDir, err)
	}

	// Construct the tar command
	// tar -C /target/dir -xzf /path/to/file.tar.gz
	tarCmd := fmt.Sprintf("tar -C %s -xzf %s", s.ExtractDir, s.TarballPath)
	log.Infof("Executing untar command: %s", tarCmd)

	stdout, stderr, exitCode, err := s.localExecutor.Execute(ctx, tarCmd)
	output = fmt.Sprintf("stdout:\n%s\nstderr:\n%s", stdout, stderr) // Combine outputs for logging/return

	if err != nil {
		log.Errorf("Untar command execution failed: %v\n%s", err, output)
		return output, false, fmt.Errorf("untar command '%s' failed: %w. Exit code: %d", tarCmd, err, exitCode)
	}
	if exitCode != 0 {
		log.Errorf("Untar command failed with exit code %d:\n%s", exitCode, output)
		return output, false, fmt.Errorf("untar command '%s' failed with exit code %d", tarCmd, exitCode)
	}

	log.Infof("Successfully extracted %s to %s", s.TarballPath, s.ExtractDir)
	return fmt.Sprintf("Successfully extracted %s to %s.\n%s", s.TarballPath, s.ExtractDir, output), true, nil
}
