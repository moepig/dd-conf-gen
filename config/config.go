package config

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadGenConfig loads and parses a generation configuration file
func LoadGenConfig(path string) (*GenConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read generation config file: %w", err)
	}

	slog.Debug("Read generation config file", "path", path, "content", string(data))

	var cfg GenConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse generation config: %w", err)
	}

	slog.Debug("Parsed generation config", "resources_count", len(cfg.Resources), "outputs_count", len(cfg.Outputs))

	if err := validateGenConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid generation config: %w", err)
	}

	return &cfg, nil
}

// validateGenConfig validates the generation configuration
func validateGenConfig(cfg *GenConfig) error {
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
