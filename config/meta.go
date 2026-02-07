package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadMetaConfig loads and parses a meta configuration file
func LoadMetaConfig(path string) (*MetaConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read meta config file: %w", err)
	}

	var cfg MetaConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse meta config: %w", err)
	}

	if err := validateMetaConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid meta config: %w", err)
	}

	return &cfg, nil
}

// validateMetaConfig validates the meta configuration
func validateMetaConfig(cfg *MetaConfig) error {
	if cfg.Version == "" {
		return fmt.Errorf("version is required")
	}

	if len(cfg.Resources) == 0 {
		return fmt.Errorf("at least one resource must be defined")
	}

	if len(cfg.Outputs) == 0 {
		return fmt.Errorf("at least one output must be defined")
	}

	// Validate resources
	resourceNames := make(map[string]bool)
	for i, res := range cfg.Resources {
		if res.Name == "" {
			return fmt.Errorf("resource[%d]: name is required", i)
		}
		if res.Type == "" {
			return fmt.Errorf("resource[%d]: type is required", i)
		}
		if res.Region == "" {
			return fmt.Errorf("resource[%d]: region is required", i)
		}
		if resourceNames[res.Name] {
			return fmt.Errorf("resource[%d]: duplicate resource name: %s", i, res.Name)
		}
		resourceNames[res.Name] = true
	}

	// Validate outputs
	for i, out := range cfg.Outputs {
		if out.Template == "" {
			return fmt.Errorf("output[%d]: template is required", i)
		}
		if out.OutputFile == "" {
			return fmt.Errorf("output[%d]: output_file is required", i)
		}
		if out.Data.ResourceName == "" {
			return fmt.Errorf("output[%d]: data.resource_name is required", i)
		}
		// Check resource reference
		if !resourceNames[out.Data.ResourceName] {
			return fmt.Errorf("output[%d]: resource_name '%s' not found in resources", i, out.Data.ResourceName)
		}
	}

	return nil
}
