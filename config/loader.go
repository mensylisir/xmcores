package config

import (
	"fmt"
	"os"
	// No import for runtime.CliArgs needed if Loader only loads the file.
	// If it were to merge CLI args with file config, then it would be needed.
	"gopkg.in/yaml.v3"
)

// Loader handles loading and initial parsing of the ClusterConfig from a file.
type Loader struct {
	filePath string
}

// NewLoader creates a new configuration loader for the given file path.
func NewLoader(filePath string) *Loader {
	return &Loader{
		filePath: filePath,
	}
}

// Load reads the configuration file, unmarshals it into ClusterConfig,
// and performs basic structural validation.
// Defaulting and further processing are handled separately.
func (l *Loader) Load() (*ClusterConfig, error) {
	if l.filePath == "" {
		return nil, fmt.Errorf("configuration file path is empty")
	}
	content, err := os.ReadFile(l.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file '%s': %w", l.filePath, err)
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("configuration file '%s' is empty", l.filePath)
	}

	var cfg ClusterConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config YAML from '%s': %w", l.filePath, err)
	}

	// Basic validation (as done before in LoadClusterConfig)
	if cfg.APIVersion == "" {
		return nil, fmt.Errorf("config validation failed: apiVersion is a required field in '%s'", l.filePath)
	}
	if cfg.Kind == "" {
		return nil, fmt.Errorf("config validation failed: kind is a required field in '%s'", l.filePath)
	}
	// Assuming "Cluster" is the primary kind for now, could be more flexible
	if cfg.Kind != "Cluster" && cfg.Kind != "ClusterConfig" { // Allow both for now, standardize later
		return nil, fmt.Errorf("config validation failed: kind must be 'Cluster' or 'ClusterConfig' in '%s', got '%s'", l.filePath, cfg.Kind)
	}
	if cfg.Metadata.Name == "" {
		return nil, fmt.Errorf("config validation failed: metadata.name is a required field in '%s'", l.filePath)
	}
	if cfg.Spec == nil { // Spec itself being nil is an issue
	    return nil, fmt.Errorf("config validation failed: spec section is missing or empty in '%s'", l.filePath)
    }


	return &cfg, nil
}
