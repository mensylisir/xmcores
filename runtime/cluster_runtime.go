package runtime

import (
	"fmt"
	"strings" // Required for Join in log messages
	// "time" // No longer directly needed here if BaseRuntime handles timeouts via Host

	"github.com/mensylisir/xmcores/config"
	"github.com/mensylisir/xmcores/connector"
	"github.com/sirupsen/logrus"
	// sync is now handled within BaseRuntime for its cache
)

// ClusterRuntime holds all runtime information specific to a cluster operation.
// It embeds connector.BaseRuntime for common functionalities like host and connector management.
type ClusterRuntime struct {
	Base        *connector.BaseRuntime  // Embedded BaseRuntime
	Cluster     *config.ClusterSpec   // Defaulted and validated cluster specification
	Arg         *CliArgs              // Parsed CLI arguments
	Log         *logrus.Entry         // Logger scoped specifically for this ClusterRuntime instance
	WorkDir     string                // Global working directory for this specific runtime context

	// PipelineCache is specific to this level of runtime, for pipeline-scoped data.
	// Module caches are handled by NewModuleCache/ReleaseModuleCache.
	PipelineCache      map[string]interface{}
	pipelineCacheLock  sync.Mutex // Separate lock for pipeline cache
}

// NewClusterRuntime creates and initializes a new ClusterRuntime instance.
func NewClusterRuntime(
	rawCfg *config.ClusterConfig, // Raw config from file
	args *CliArgs,
	workDir string,
	// ignoreError and verbose are now sourced from args for BaseRuntime
	baseLogger *logrus.Entry,
) (*ClusterRuntime, error) {

	if rawCfg == nil {
		return nil, fmt.Errorf("ClusterConfig (rawCfg) cannot be nil when creating ClusterRuntime")
	}
	if rawCfg.Spec == nil { // Spec within rawCfg might be nil before defaulting
		// Initialize an empty spec if it's nil to allow SetDefaultClusterSpec to work
		rawCfg.Spec = &config.ClusterSpec{}
	}
	if args == nil {
		args = NewCliArgs() // Use default CliArgs if none provided
	}
	if baseLogger == nil {
		baseLogger = logrus.NewEntry(logrus.New()) // Fallback, though main should provide
	}

	// Step 1: Apply defaults and process host specifications from rawCfg.Spec.Hosts
	// SetDefaultClusterSpec needs a mutable spec, so we pass the address of rawCfg.Spec.
	// It will modify rawCfg.Spec in place with defaults.
	var rawHostSpecs []config.HostSpec
	if rawCfg.Spec != nil { // Should not be nil due to check above, but good practice
		rawHostSpecs = rawCfg.Spec.Hosts
	}

	// The defaultedSpec IS rawCfg.Spec after modification by SetDefaultClusterSpec
	defaultedSpec, roleMapFromDefaults, allHostsFromDefaults, err := config.SetDefaultClusterSpec(rawCfg.Spec, rawHostSpecs)
	if err != nil {
		return nil, fmt.Errorf("failed to apply default cluster specifications: %w", err)
	}

	// Step 2: Initialize connector.BaseRuntime
	// KubeKey example uses args.Debug for debug and args.IgnoreErr for ignoreErr
	// Assuming CliArgs has Debug and IgnoreErr fields. If not, adjust.
	// For now, let's assume CliArgs has Debug (for verbose) and IgnoreErr.
	// The prompt for CliArgs did not include Debug or IgnoreErr. Let's add them.
	// For this iteration, we'll use the 'verbose' and 'ignoreError' passed to NewClusterRuntime
	// which main.go sources from global flags. These can later be moved to CliArgs.

	// The verbose flag passed to NewClusterRuntime here is from main.go, reflecting CLI flags.
	// The ignoreError flag passed is also from main.go CLI flags.
	baseRt := connector.NewBaseRuntime(rawCfg.Metadata.Name, connector.NewDialer(), args.Debug, args.IgnoreErr, baseLogger)


	// Step 3: Populate BaseRuntime's hosts and roles from SetDefaultClusterSpec results
	baseRt.SetAllHosts(allHostsFromDefaults) // Assuming a method like this on BaseRuntime
	baseRt.SetRoleMap(roleMapFromDefaults)   // Assuming a method like this on BaseRuntime

	// Step 4: Initialize ClusterRuntime specific fields
	scopedLogger := baseLogger.WithFields(logrus.Fields{
		"runtime_scope": "cluster",
		"cluster_name":  rawCfg.Metadata.Name,
	})
	scopedLogger.Infof("Initializing ClusterRuntime for cluster '%s'...", rawCfg.Metadata.Name)


	cr := &ClusterRuntime{
		Base:        baseRt,
		Cluster:     defaultedSpec, // Store the spec that has defaults applied
		Arg:         args,
		Log:         scopedLogger,
		WorkDir:     workDir,    // This is the global workDir from flags/config
		IgnoreErr:   args.IgnoreErr, // Store from CliArgs
		Verbose:     args.Debug,     // Store from CliArgs
		PipelineCache: make(map[string]interface{}),
	}

	cr.Log.Infof("ClusterRuntime initialized. Processed %d hosts. Roles defined: %s.",
		len(cr.Base.GetAllHosts()), strings.Join(cr.Base.GetRoleNames(), ", "), // Add GetRoleNames to BaseRuntime
	)
	return cr, nil
}

// Delegated methods to BaseRuntime for host and connector management
func (cr *ClusterRuntime) GetAllHosts() []connector.Host {
	return cr.Base.GetAllHosts()
}

func (cr *ClusterRuntime) GetRoleHosts(role string) []connector.Host {
	return cr.Base.GetRoleHosts(role)
}

func (cr *ClusterRuntime) GetConnector(host connector.Host) (connector.Connector, error) {
	return cr.Base.GetConnector(host)
}

func (cr *ClusterRuntime) CloseConnector(host connector.Host) error {
	return cr.Base.CloseConnector(host)
}

func (cr *ClusterRuntime) CloseAllConnectors() {
	cr.Base.CloseAllConnectors()
}

// LogEntry returns the scoped logger for this ClusterRuntime.
func (cr *ClusterRuntime) LogEntry() *logrus.Entry {
	return cr.Log
}

// Cache Management Methods (specific to ClusterRuntime's PipelineCache)
func (cr *ClusterRuntime) NewModuleCache() map[string]interface{} {
	return make(map[string]interface{})
}

func (cr *ClusterRuntime) ReleaseModuleCache(moduleCache map[string]interface{}) {
	cr.Log.Debugf("Releasing module cache (currently a no-op). Cache size: %d", len(moduleCache))
}

func (cr *ClusterRuntime) GetPipelineCacheValue(key string) (interface{}, bool) {
	cr.pipelineCacheLock.Lock()
	defer cr.pipelineCacheLock.Unlock()
	val, found := cr.PipelineCache[key]
	return val, found
}

func (cr *ClusterRuntime) SetPipelineCacheValue(key string, value interface{}) {
	cr.pipelineCacheLock.Lock()
	defer cr.pipelineCacheLock.Unlock()
	if cr.PipelineCache == nil {
		cr.PipelineCache = make(map[string]interface{})
	}
	cr.PipelineCache[key] = value
}

func (cr *ClusterRuntime) ReleasePipelineCache() {
	cr.pipelineCacheLock.Lock()
	defer cr.pipelineCacheLock.Unlock()
	cr.Log.Info("Releasing pipeline cache.")
	cr.PipelineCache = make(map[string]interface{})
}

// Getters for operational parameters, potentially satisfying parts of an evolved runtime.Runtime interface
func (cr *ClusterRuntime) GetWorkDir() string { return cr.WorkDir }
func (cr *ClusterRuntime) GetIgnoreError() bool { return cr.IgnoreErr } // From BaseRuntime via Arg
func (cr *ClusterRuntime) GetVerbose() bool { return cr.Verbose }     // From BaseRuntime via Arg
func (cr *ClusterRuntime) GetClusterSpec() *config.ClusterSpec { return cr.Cluster }
func (cr *ClusterRuntime) GetCliArgs() *CliArgs { return cr.Arg }

// Getters from embedded BaseRuntime (if not directly embedding its fields)
// func (cr *ClusterRuntime) BaseLogger() *logrus.Entry { return cr.Base.Logger }
// func (cr *ClusterRuntime) ClusterName() string { return cr.Base.ClusterName }
// func (cr *ClusterRuntime) Dialer() connector.Dialer { return cr.Base.Dialer }

// Ensure ClusterRuntime could satisfy a more generic runtime interface if needed in future.
// var _ Runtime = (*ClusterRuntime)(nil) // If a new Runtime interface is defined.
