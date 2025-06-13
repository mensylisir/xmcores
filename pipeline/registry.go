package pipeline

import (
	"fmt"
	"sync"
)

// PipelineFactory defines the function signature for creating pipeline instances.
type PipelineFactory func() Pipeline

var (
	// DefaultRegistry holds the registered pipeline factories.
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
// It returns an error if the pipeline name is not found in the registry.
func GetPipeline(name string) (Pipeline, error) {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	factory, exists := DefaultRegistry[name]
	if !exists {
		return nil, fmt.Errorf("pipeline with name '%s' not found in registry", name)
	}
	return factory(), nil
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
