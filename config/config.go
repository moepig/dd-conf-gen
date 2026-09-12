package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/moepig/dd-conf-gen/internal/logging"
	"gopkg.in/yaml.v3"
)

// LoadGenConfig loads and parses a generation configuration file
func LoadGenConfig(path string) (*GenConfig, error) {
	return LoadGenConfigContext(context.Background(), path)
}

// Reads and validates a configuration file, sending diagnostics to the context logger.
func LoadGenConfigContext(ctx context.Context, path string) (*GenConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read generation config file: %w", err)
	}

	var cfg GenConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse generation config: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("failed to parse generation config: %w", err)
		}
		return nil, fmt.Errorf("generation config must contain exactly one YAML document")
	}

	logging.FromContext(ctx).Debug("Loaded generation config", "config", cfg)

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
	outputPaths := make(map[string]int)
	for i, out := range cfg.Outputs {
		if out.Template == "" {
			return fmt.Errorf("output[%d]: template is required", i)
		}
		if out.OutputFile == "" {
			return fmt.Errorf("output[%d]: output_file is required", i)
		}
		path, err := canonicalOutputPath(out.OutputFile)
		if err != nil {
			return fmt.Errorf("output[%d]: invalid output_file: %w", i, err)
		}
		if previous, ok := outputPaths[path]; ok {
			return fmt.Errorf("output[%d]: duplicate output_file with output[%d]: %s", i, previous, out.OutputFile)
		}
		outputPaths[path] = i
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

// Normalizes an output path and resolves existing symlink ancestors for duplicate detection.
func canonicalOutputPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current, suffix := abs, ""
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		if !os.IsNotExist(err) {
			return abs, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		suffix = filepath.Join(filepath.Base(current), suffix)
		current = parent
	}
}
