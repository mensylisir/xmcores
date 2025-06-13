package runtime

// CliArgs holds command-line arguments that can affect pipeline or module behavior.
// These are typically passed from the main Cobra command execution logic.
type CliArgs struct {
	Artifact            string // Path to an artifact, if any (e.g., a tarball for offline installation)
	SkipPushImages      bool   // If true, skip pushing images to a private registry
	DeployLocalStorage  *bool  // Use pointer for three-state: nil (not set), true (deploy), false (do not deploy)
	InstallPackages     bool   // If true, allow installation of OS packages
	SkipPullImages      bool   // If true, skip pulling images from registries (assume they are pre-loaded)
	SecurityEnhancement bool   // If true, apply additional security enhancements
	SkipInstallAddons   bool   // If true, skip installing default addons

	Debug               bool   // Corresponds to verbose logging (set by -v or --log-level=debug)
	IgnoreErr           bool   // Corresponds to --ignore-errors global flag

	// Future flags can be added here, for example:
	// Force           bool   // Force operations
	// Offline         bool   // Indicate offline mode, might affect how resources are fetched
	// CustomFlags     map[string]string // For arbitrary key-value flags
}

// NewCliArgs creates a new instance of CliArgs with default values.
// This can be used to initialize if no specific CLI flags are bound yet.
func NewCliArgs() *CliArgs {
	// Initialize with common defaults. Pointers like DeployLocalStorage default to nil.
	return &CliArgs{
		InstallPackages: true, // Example default: usually want to install packages
	}
}
