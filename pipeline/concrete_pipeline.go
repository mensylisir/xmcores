package pipeline

import (
	"github.com/mensylisir/xmcores/module"
	// "github.com/mensylisir/xmcores/pipeline/ending" // Not directly used by ConcretePipeline methods here, but by RunModule in specific pipeline
	krt "github.com/mensylisir/xmcores/runtime" // Using krt as alias for ClusterRuntime
	"github.com/sirupsen/logrus"
	"sync" // For pipelineCacheLock if managing cache here, though prompt put it on ClusterRuntime
)

// ConcretePipeline provides a base implementation for common pipeline functionalities.
// It can be embedded by specific pipeline implementations like InstallPipeline.
type ConcretePipeline struct {
	NameField        string // Using NameField to avoid conflict with Name() method if embedding
	DescriptionField string
	Runtime          *krt.ClusterRuntime
	Modules          []module.Module
	ModulePostHooks  []module.HookFn // Assuming module.HookFn is defined in module/interface.go

	// PipelineCache is now part of ClusterRuntime. This struct will use the one from ClusterRuntime.
	// pipelineCache      map[string]interface{}
	// pipelineCacheLock  sync.Mutex
}

// NewConcretePipeline is a constructor for ConcretePipeline.
// Specific pipelines like InstallPipeline will call this.
func NewConcretePipeline(name, description string, runtime *krt.ClusterRuntime) ConcretePipeline {
	return ConcretePipeline{
		NameField:        name,
		DescriptionField: description,
		Runtime:          runtime,
		Modules:          make([]module.Module, 0),
		ModulePostHooks:  make([]module.HookFn, 0),
		// PipelineCache is accessed via runtime.GetPipelineCacheValue / SetPipelineCacheValue
	}
}

// Name returns the name of the pipeline.
func (p *ConcretePipeline) Name() string {
	return p.NameField
}

// Description returns the description of the pipeline.
func (p *ConcretePipeline) Description() string {
	return p.DescriptionField
}


// --- Cache methods that delegate to ClusterRuntime ---

// NewModuleCache creates a new cache instance for a module, delegating to ClusterRuntime.
func (p *ConcretePipeline) NewModuleCache() map[string]interface{} {
	if p.Runtime == nil {
		// This should not happen if pipeline is constructed correctly
		logrus.Error("ConcretePipeline: Attempted to call NewModuleCache with nil Runtime")
		return make(map[string]interface{})
	}
	return p.Runtime.NewModuleCache()
}

// ReleaseModuleCache handles cleanup of a module cache, delegating to ClusterRuntime.
func (p *ConcretePipeline) ReleaseModuleCache(moduleCache map[string]interface{}) {
	if p.Runtime == nil {
		logrus.Error("ConcretePipeline: Attempted to call ReleaseModuleCache with nil Runtime")
		return
	}
	p.Runtime.ReleaseModuleCache(moduleCache)
}

// GetPipelineCacheValue retrieves a value from the pipeline-level cache via ClusterRuntime.
func (p *ConcretePipeline) GetPipelineCacheValue(key string) (interface{}, bool) {
    if p.Runtime == nil {
		logrus.Error("ConcretePipeline: Attempted to call GetPipelineCacheValue with nil Runtime")
        return nil, false
    }
    return p.Runtime.GetPipelineCacheValue(key)
}

// SetPipelineCacheValue sets a value in the pipeline-level cache via ClusterRuntime.
func (p *ConcretePipeline) SetPipelineCacheValue(key string, value interface{}) {
    if p.Runtime == nil {
		logrus.Error("ConcretePipeline: Attempted to call SetPipelineCacheValue with nil Runtime")
        return
    }
    p.Runtime.SetPipelineCacheValue(key, value)
}

// ReleasePipelineCache clears the pipeline-level cache via ClusterRuntime.
func (p *ConcretePipeline) ReleasePipelineCache() {
    if p.Runtime == nil {
		logrus.Error("ConcretePipeline: Attempted to call ReleasePipelineCache with nil Runtime")
        return
    }
    p.Runtime.ReleasePipelineCache()
}

// Placeholder for pipeline's own Init, if any, called by its Start() method.
// This Init is for setting up things internal to the pipeline *after* its factory has run
// and *before* modules are processed by Start().
// Most module setup is now in the pipeline factory.
func (p *ConcretePipeline) Init(logger *logrus.Entry) error {
	if p.Runtime == nil || p.Runtime.Log == nil {
		// Fallback if logger somehow not set on runtime. This is defensive.
		if logger == nil {
			logger = logrus.NewEntry(logrus.New())
		}
		logger.Warnf("Pipeline [%s] internal Init called, but runtime or runtime logger is nil. Using provided/default logger.", p.NameField)
	} else {
		// Use the runtime's logger, scoped further if needed.
		logger = p.Runtime.Log.WithField("pipeline_init", p.NameField)
	}
	logger.Infof("Pipeline [%s] internal Init()... (placeholder for common pipeline pre-module-loop setup)", p.NameField)
	return nil
}

// AddModule is a helper for concrete pipelines to add modules to their sequence.
func (p *ConcretePipeline) AddModule(mod module.Module) {
    p.Modules = append(p.Modules, mod)
}

// AppendModulePostHook adds a post hook for the current module being processed.
// KubeKey's example implies hooks are added to the pipeline globally, not per module.
// Let's follow that: hooks are on the pipeline, called after each module.
func (p *ConcretePipeline) AppendModulePostHook(hookFn module.HookFn) {
    p.ModulePostHooks = append(p.ModulePostHooks, hookFn)
}

// ClearModulePostHooks clears all registered module post hooks.
func (p *ConcretePipeline) ClearModulePostHooks() {
    p.ModulePostHooks = make([]module.HookFn, 0)
}
