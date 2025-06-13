package pipeline

import (
	"fmt"
	"sync"
	"github.com/mensylisir/xmcores/runtime" // For KubeRuntime in factory and GetPipeline
)

// PipelineFactory is already defined in pipeline/interface.go
// type PipelineFactory func(kr *runtime.KubeRuntime) (Pipeline, error)

var (
	DefaultRegistry = make(map[string]PipelineFactory)
	registryMutex   = &sync.RWMutex{}
)

// Register adds a pipeline factory to the DefaultRegistry.
// It returns an error if a pipeline with the same name is already registered.
func Register(name string, factory PipelineFactory) error {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	if _, exists := DefaultRegistry[name]; exists {
		return fmt.Errorf("pipeline with name '%s' already registered", name)
	}
	DefaultRegistry[name] = factory
	return nil
}

// GetPipeline retrieves a new pipeline instance from the registry using its factory.
// It now requires a KubeRuntime to pass to the factory and can return an error from the factory.
func GetPipeline(name string, kr *runtime.KubeRuntime) (Pipeline, error) {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	factory, exists := DefaultRegistry[name]
	if !exists {
		return nil, fmt.Errorf("pipeline with name '%s' not found in registry", name)
	}
	// Call the factory, passing KubeRuntime, and handle potential error from factory.
	pipelineInstance, err := factory(kr)
	if err != nil {
		return nil, fmt.Errorf("factory for pipeline '%s' failed: %w", name, err)
	}
	return pipelineInstance, nil
}

// GetRegisteredPipelineNames returns a slice of names of all registered pipelines.
func GetRegisteredPipelineNames() []string {
	registryMutex.RLock()
	defer registryMutex.RUnlock()
	names := make([]string, 0, len(DefaultRegistry))
	for name := range DefaultRegistry {
		names = append(names, name)
	}
	return names
}
