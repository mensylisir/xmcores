package containerd

import (
	"context"
	"fmt"
	"os"
	"path" // For remote paths
	"path/filepath" // For local paths
	"strings"
	"sync" // Added for WaitGroup

	"github.com/mensylisir/xmcores/connector" // Added for connector.Host
	"github.com/mensylisir/xmcores/executor"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/mensylisir/xmcores/step"
	"github.com/sirupsen/logrus"
)

// DeliveryContainerdStep handles the distribution of containerd binaries to remote hosts.
type DeliveryContainerdStep struct {
	step.BaseStep
	SourceDir     string   // Local directory where binaries are (e.g., after extraction)
	TargetDir     string   // Remote directory on each host (e.g., /usr/local/bin)
	Binaries      []string // List of binary names to deliver
	SubPathInSource string   // e.g., "bin" if binaries are in SourceDir/bin
}

// DefaultContainerdBinaries is a list of common containerd binary names.
var DefaultContainerdBinaries = []string{
	"containerd",
	"containerd-shim-runc-v1",
	"containerd-shim-runc-v2",
	"ctr",
	// "containerd-stress", // Usually not needed for basic setup
	// "containerd-shim", // Older, typically v1/v2 are used
}

// NewDeliveryContainerdStep creates a new DeliveryContainerdStep.
func NewDeliveryContainerdStep(sourceDir, targetDir string, binaries []string) step.Step {
	if len(binaries) == 0 {
		binaries = DefaultContainerdBinaries
	}
	return &DeliveryContainerdStep{
		BaseStep:    step.NewBaseStep("DeliveryContainerd", "Distribute Containerd binaries to target hosts"),
		SourceDir:   sourceDir,
		TargetDir:   targetDir,
		Binaries:    binaries,
		SubPathInSource: "bin", // Common structure for containerd tarballs
	}
}

func (s *DeliveryContainerdStep) Init(rt runtime.Runtime, log *logrus.Entry) error {
	if s.SourceDir == "" {
		return fmt.Errorf("source directory cannot be empty for DeliveryContainerd step")
	}
	if s.TargetDir == "" {
		s.TargetDir = "/usr/local/bin" // Default target directory
	}
	if len(s.Binaries) == 0 {
		return fmt.Errorf("binaries list cannot be empty for DeliveryContainerd step")
	}

	log.Infof("DeliveryContainerdStep initialized: SourceDir=%s, TargetDir=%s, Binaries=%v, SubPathInSource=%s",
		s.SourceDir, s.TargetDir, s.Binaries, s.SubPathInSource)
	return s.BaseStep.Init(rt, log)
}

func (s *DeliveryContainerdStep) Execute(rt runtime.Runtime, log *logrus.Entry) (output string, success bool, err error) {
	log.Infof("Starting DeliveryContainerd step: %s", s.Description())
	ctx := context.Background() // Context for operations

	var overallSuccess = true
	// Use a mutex to protect shared access to errors and outputs slices from goroutines
	var mu sync.Mutex
	var errors []string
	var outputs []string

	var wg sync.WaitGroup

	targetHosts := rt.AllHosts()
	if len(targetHosts) == 0 {
		log.Warn("No target hosts specified in runtime. Skipping delivery.")
		return "No target hosts specified, skipping delivery.", true, nil
	}

	for _, hostEntry := range targetHosts {
		wg.Add(1)
		go func(host connector.Host) { // Use connector.Host type for host
			defer wg.Done()
			hostLogger := log.WithField("host", host.GetName())

			hostLogger.Infof("Processing host %s (%s)", host.GetName(), host.GetAddress())

			hostConn, hostRunner, err := rt.GetHostConnectorAndRunner(host)
			if err != nil {
				hostErr := fmt.Errorf("host %s (%s): failed to get connector/runner: %w", host.GetName(), host.GetAddress(), err)
				hostLogger.Error(hostErr.Error())
				mu.Lock()
				errors = append(errors, hostErr.Error())
				overallSuccess = false
				mu.Unlock()
				// If not ignoring errors, we might want a way to signal overall failure early,
				// but goroutines make this complex. For now, collect all errors.
				return // End this goroutine
			}
			defer func() {
				if err_close := hostConn.Close(); err_close != nil {
					hostLogger.Warnf("Error closing connection for host %s: %v", host.GetName(), err_close)
				}
			}()


			hostLogger.Debugf("Successfully obtained connector and runner for host %s", host.GetName())

			remoteExec, err := executor.NewRemoteExecutor(hostConn, hostRunner)
			if err != nil {
				hostErr := fmt.Errorf("host %s (%s): failed to create remote executor: %w", host.GetName(), host.GetAddress(), err)
				hostLogger.Error(hostErr.Error())
				mu.Lock()
				errors = append(errors, hostErr.Error())
				overallSuccess = false
				mu.Unlock()
				return
			}

			hostLogger.Infof("Ensuring target directory %s exists on host %s", s.TargetDir, host.GetName())
			if err := remoteExec.CreateRemoteDirectory(ctx, s.TargetDir, 0755); err != nil {
				hostErr := fmt.Errorf("host %s (%s): failed to create remote directory %s: %w", host.GetName(), host.GetAddress(), s.TargetDir, err)
				hostLogger.Error(hostErr.Error())
				mu.Lock()
				errors = append(errors, hostErr.Error())
				overallSuccess = false
				mu.Unlock()
				return
			}

			for _, binaryName := range s.Binaries {
				localBinaryPath := filepath.Join(s.SourceDir, s.SubPathInSource, binaryName)
				remoteBinaryPath := path.Join(s.TargetDir, binaryName) // Use path.Join for remote paths

				hostLogger.Infof("Host %s (%s): Checking local binary %s", host.GetName(), host.GetAddress(), localBinaryPath)
				localExists, statErr := os.Stat(localBinaryPath)
				if os.IsNotExist(statErr) {
					msg := fmt.Sprintf("host %s (%s): local binary %s does not exist, skipping delivery for this binary", host.GetName(), host.GetAddress(), localBinaryPath)
					hostLogger.Warn(msg)
					mu.Lock()
					errors = append(errors, msg)
					mu.Unlock()
					continue
				}
				if statErr != nil {
					msg := fmt.Sprintf("host %s (%s): failed to stat local binary %s: %v, skipping", host.GetName(), host.GetAddress(), localBinaryPath, statErr)
					hostLogger.Error(msg)
					mu.Lock()
					errors = append(errors, msg)
					overallSuccess = false
					mu.Unlock()
					if !rt.IgnoreError() {
						// This is tricky in a goroutine context for immediate bailout.
						// For now, we continue processing other binaries/hosts and report all errors.
					}
					continue
				}
				if localExists.IsDir() {
                     msg := fmt.Sprintf("host %s (%s): local path %s is a directory, not a file, skipping", host.GetName(), host.GetAddress(), localBinaryPath)
					hostLogger.Error(msg)
					mu.Lock()
					errors = append(errors, msg)
					overallSuccess = false
					mu.Unlock()
					if !rt.IgnoreError() {
						// Similar to above, continue and report.
					}
					continue
                }

				hostLogger.Infof("Host %s (%s): Delivering binary %s to %s", host.GetName(), host.GetAddress(), localBinaryPath, remoteBinaryPath)
				err = remoteExec.PutRemoteFile(ctx, localBinaryPath, remoteBinaryPath, 0755) // 0755 for executable
				if err != nil {
					hostErr := fmt.Errorf("host %s (%s): failed to deliver binary %s to %s: %w", host.GetName(), host.GetAddress(), binaryName, remoteBinaryPath, err)
					hostLogger.Error(hostErr.Error())
					mu.Lock()
					errors = append(errors, hostErr.Error())
					overallSuccess = false
					mu.Unlock()
					if !rt.IgnoreError() {
						// Continue and report.
					}
				} else {
					msg := fmt.Sprintf("Host %s (%s): Successfully delivered %s to %s", host.GetName(), host.GetAddress(), binaryName, remoteBinaryPath)
					hostLogger.Info(msg)
					mu.Lock()
					outputs = append(outputs, msg)
					mu.Unlock()
				}
			}
		}(hostEntry)
	}
	wg.Wait()


	finalOutput := strings.Join(outputs, "\n")
	if len(errors) > 0 {
		errorSummary := strings.Join(errors, "\n")
		finalOutput = fmt.Sprintf("Completed with errors.\nOutputs:\n%s\nErrors:\n%s", finalOutput, errorSummary)
		log.Errorf("DeliveryContainerd step completed with errors:\n%s", errorSummary)
		// The overall error returned should reflect that multiple errors may have occurred if overallSuccess is false.
		var finalErr error
		if !overallSuccess {
			finalErr = fmt.Errorf("one or more errors occurred during binary delivery: %s", errorSummary)
		}
		return finalOutput, overallSuccess, finalErr
	}

	log.Infof("DeliveryContainerd step completed successfully for all processed hosts.")
	return finalOutput, true, nil
}
