package pipeline

import (
	"fmt"
	"strings"

	"github.com/mensylisir/xmcores/common" // For logger field constants
	"github.com/mensylisir/xmcores/module"
	"github.com/mensylisir/xmcores/runtime"
	"github.com/sirupsen/logrus"
)

// BasePipeline provides a basic implementation for the Pipeline interface.
// It can be embedded in concrete pipeline implementations.
type BasePipeline struct {
	name        string
	description string
	modules     []module.Module
}

// NewBasePipeline creates a new BasePipeline.
func NewBasePipeline(name, description string) BasePipeline {
	return BasePipeline{
		name:        name,
		description: description,
		modules:     make([]module.Module, 0),
	}
}

// Name returns the name of the pipeline.
func (bp *BasePipeline) Name() string {
	return bp.name
}

// SetName sets the name of the pipeline.
func (bp *BasePipeline) SetName(name string) {
	bp.name = name
}

// Description returns the description of the pipeline.
func (bp *BasePipeline) Description() string {
	return bp.description
}

// SetDescription sets the description of the pipeline.
func (bp *BasePipeline) SetDescription(desc string) {
	bp.description = desc
}

// Modules returns the list of modules in the pipeline.
func (bp *BasePipeline) Modules() []module.Module {
	// Return a copy to prevent external modification
	m := make([]module.Module, len(bp.modules))
	copy(m, bp.modules)
	return m
}

// AddModule adds a module to the pipeline's execution list.
func (bp *BasePipeline) AddModule(m module.Module) {
	bp.modules = append(bp.modules, m)
}

// SetModules sets the list of modules for the pipeline.
func (bp *BasePipeline) SetModules(modules []module.Module) {
	bp.modules = make([]module.Module, len(modules))
	copy(bp.modules, modules)
}

// Init provides a default Init implementation that initializes all added modules.
func (bp *BasePipeline) Init(rt runtime.Runtime, log *logrus.Entry) error {
	log.Debugf("BasePipeline.Init called for pipeline: %s. Initializing %d modules.", bp.Name(), len(bp.modules))
	if len(bp.modules) == 0 {
		log.Warn("No modules defined for this pipeline.")
		// Depending on requirements, this could be an error.
	}
	for i, m := range bp.modules {
		moduleLog := log.WithFields(logrus.Fields{
			common.LogFieldModuleName: m.Name(),
			"module_index":            fmt.Sprintf("%d/%d", i+1, len(bp.modules)),
		})
		moduleLog.Infof("Initializing module: %s (%s)", m.Name(), m.Description())
		if err := m.Init(rt, moduleLog); err != nil {
			moduleLog.Errorf("Failed to initialize module %s: %v", m.Name(), err)
			return fmt.Errorf("failed to initialize module %s (index %d) in pipeline %s: %w", m.Name(), i, bp.Name(), err)
		}
	}
	log.Infof("All %d modules for pipeline %s initialized successfully.", len(bp.modules), bp.Name())
	return nil
}

// Execute provides a default Execute implementation that runs all modules sequentially.
func (bp *BasePipeline) Execute(rt runtime.Runtime, log *logrus.Entry) error {
	log.Infof("Executing pipeline: %s (%s)", bp.Name(), bp.Description())
	if len(bp.modules) == 0 {
		log.Warnf("Pipeline %s has no modules to execute.", bp.Name())
		return nil // Or an error if pipelines must have modules
	}

	var overallPipelineFailed bool
	var moduleErrors []string

	for i, currentModule := range bp.modules {
		moduleLog := log.WithFields(logrus.Fields{
			common.LogFieldModuleName: currentModule.Name(),
			"module_index":            fmt.Sprintf("%d/%d", i+1, len(bp.modules)),
		})
		moduleLog.Infof("Executing module: %s (%s)", currentModule.Name(), currentModule.Description())
		fmt.Printf("===> Executing Module: %s (%s)\n", currentModule.Name(), currentModule.Description())

		moduleErr := currentModule.Execute(rt, moduleLog)

		// Call Post for the current module, regardless of its success/failure
		postLog := moduleLog.WithField("sub_phase", "post_execute")
		if postErr := currentModule.Post(rt, postLog, moduleErr); postErr != nil {
			postLog.Errorf("Error during Post-Execute for module %s: %v", currentModule.Name(), postErr)
			moduleErrors = append(moduleErrors, fmt.Sprintf("post-execute error for module %s: %v", currentModule.Name(), postErr))
			overallPipelineFailed = true
		}

		if moduleErr != nil {
			moduleLog.Errorf("Module %s failed: %v", currentModule.Name(), moduleErr)
			fmt.Printf("===> Module FAILED: %s. Error: %v\n", currentModule.Name(), moduleErr)
			moduleErrors = append(moduleErrors, fmt.Sprintf("module %s error: %v", currentModule.Name(), moduleErr))
			overallPipelineFailed = true
			if !rt.IgnoreError() {
				log.Errorf("Pipeline %s failed at module %s due to error: %v. Halting pipeline execution.", bp.Name(), currentModule.Name(), moduleErr)
				return fmt.Errorf("pipeline %s failed at module %s: %w", bp.Name(), currentModule.Name(), moduleErr)
			}
			log.Warnf("Module %s failed but IgnoreError is true. Continuing pipeline execution.", currentModule.Name())
		} else {
			moduleLog.Infof("Module %s completed successfully.", currentModule.Name())
			fmt.Printf("===> Module SUCCEEDED: %s.\n", currentModule.Name())
		}
	}

	if overallPipelineFailed {
		log.Errorf("Pipeline %s completed with one or more errors: %s", bp.Name(), strings.Join(moduleErrors, "; "))
		return fmt.Errorf("pipeline %s failed with errors: %s", bp.Name(), strings.Join(moduleErrors, "; "))
	}

	log.Infof("Pipeline %s completed successfully.", bp.Name())
	return nil
}

// Post provides a default no-op Post implementation for the pipeline itself.
func (bp *BasePipeline) Post(rt runtime.Runtime, log *logrus.Entry, executeErr error) error {
	log.Debugf("BasePipeline.Post called for pipeline %s.", bp.Name())
	return nil
}
