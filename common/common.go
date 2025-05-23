package common

import (
	"io/fs"
	"path/filepath"
)

const (
	AppName    = "xmcores"
	TmpDirBase = "/tmp/"
)

func GetTmpDir() string {
	return filepath.Join(TmpDirBase, AppName) + "/"
}

const (
	PipelineName  = "Pipeline"
	ModuleName    = "Module"
	TaskName      = "Task"
	StepName      = "Step"
	NodeName      = "Node"
	LocalHostname = "LocalHost"
)

const (
	// FileMode0755 represents rwxr-xr-x
	FileMode0755 fs.FileMode = 0755
	// FileMode0644 represents rw-r--r--
	FileMode0644 fs.FileMode = 0644
	// FileMode0600 represents rw-------
	FileMode0600 fs.FileMode = 0600
	// FileMode0700 represents rwx------
	FileMode0700 fs.FileMode = 0700
)

const (
	// CopyCmdTpl is a template for copying files/directories.
	// Example: fmt.Sprintf(CopyCmdTpl, src, dst)
	CopyCmdTpl = "cp -r %s %s"
	// MoveCmdTpl is a template for moving files/directories.
	MoveCmdTpl = "mv -f %s %s"
	// MkdirCmdTpl is a template for creating directories.
	MkdirCmdTpl = "mkdir -p %s" // Added common command
	// TarCmdTpl is a template for creating a tar archive.
	// Example: fmt.Sprintf(TarCmdTpl, archiveName, sourceDir)
	TarCmdTpl = "tar -czvf %s %s" // Added common command
	// UntarCmdTpl is a template for extracting a tar archive.
	// Example: fmt.Sprintf(UntarCmdTpl, archiveName, destinationDir)
	UntarCmdTpl = "tar -xzvf %s -C %s" // Added common command
	// ChmodCmdTpl is a template for changing file permissions.
	// Example: fmt.Sprintf(ChmodCmdTpl, "0755", "/path/to/file")
	ChmodCmdTpl = "chmod %s %s" // Added common command
	// ChownCmdTpl is a template for changing file ownership.
	// Example: fmt.Sprintf(ChownCmdTpl, "user:group", "/path/to/file")
	ChownCmdTpl = "chown %s %s" // Added common command
)

// Add other categories of constants as needed, for example:
// Network related constants
const (
	DefaultSSHPort = 22
)

// Status or State constants (iota can be useful here)
type OperationState int

const (
	StatePending OperationState = iota // 0
	StateRunning                       // 1
	StateSuccess                       // 2
	StateFailed                        // 3
	StateSkipped                       // 4
)

func (s OperationState) String() string {
	switch s {
	case StatePending:
		return "Pending"
	case StateRunning:
		return "Running"
	case StateSuccess:
		return "Success"
	case StateFailed:
		return "Failed"
	case StateSkipped:
		return "Skipped"
	default:
		return "Unknown"
	}
}

// Example of iota for bitmasks (if applicable)
type FeatureFlags uint32

const (
	FeatureAlpha FeatureFlags = 1 << iota // 1
	FeatureBeta                           // 2
	FeatureGamma                          // 4
)

const (
	NanosPerMicrosecond int64 = 1000
	NanosPerMillisecond int64 = 1000 * NanosPerMicrosecond
	NanosPerSecond      int64 = 1000 * NanosPerMillisecond
)

type Arch string

const (
	ArchAmd64   Arch = "amd64"
	ArchX86_64  Arch = "x86_64"
	ArchArm64   Arch = "arm64"
	ArchArm     Arch = "arm"
	ArchUnknown Arch = "unknown"
)
