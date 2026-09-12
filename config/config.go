package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/moepig/dd-conf-gen/internal/logging"
	"gopkg.in/yaml.v3"
)

// Reads and validates the generation configuration at path.
//
// Returns the configuration, or nil and an error for file access, YAML decoding, unknown fields, multiple documents, or invalid definitions.
func LoadGenConfig(path string) (*GenConfig, error) {
	return LoadGenConfigContext(context.Background(), path)
}

// Reads and validates the generation configuration at path, using ctx for logging.
//
// Returns the configuration, or nil and an error for file access, YAML decoding, unknown fields, multiple documents, or invalid definitions.
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

	logging.FromContext(ctx).Debug("Loaded generation config", "resources_count", len(cfg.Resources), "outputs_count", len(cfg.Outputs))

	if err := validateGenConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid generation config: %w", err)
	}

	return &cfg, nil
}

// Validates required fields and resource references in non-nil cfg.
//
// Returns an error for empty resource or output lists, missing required fields, duplicate resource names, or unresolved resource references; otherwise returns nil.
func validateGenConfig(cfg *GenConfig) error {
	if len(cfg.Resources) == 0 {
		return fmt.Errorf("at least one resource must be defined")
	}

	if len(cfg.Outputs) == 0 {
		return fmt.Errorf("at least one output must be defined")
	}

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

	for i, out := range cfg.Outputs {
		for name, ref := range out.Data.Secrets {
			if name == "" {
				return fmt.Errorf("output[%d]: secret name must not be empty", i)
			}
			if err := ref.Validate(); err != nil {
				return fmt.Errorf("output[%d]: secret %q: %w", i, name, err)
			}
		}
		switch out.OnEmpty {
		case "", "render", "error", "keep":
		default:
			return fmt.Errorf("output[%d]: on_empty must be render, error, or keep", i)
		}
		if out.Template == "" {
			return fmt.Errorf("output[%d]: template is required", i)
		}
		if out.OutputFile == "" {
			return fmt.Errorf("output[%d]: output_file is required", i)
		}
		if out.Data.ResourceName != "" && len(out.Data.ResourceNames) != 0 {
			return fmt.Errorf("output[%d]: data.resource_name and data.resource_names are mutually exclusive", i)
		}
		if len(out.Data.Names()) == 0 {
			return fmt.Errorf("output[%d]: data.resource_name is required when data.resource_names is empty", i)
		}
		seen := make(map[string]bool)
		for _, name := range out.Data.Names() {
			if !resourceNames[name] {
				return fmt.Errorf("output[%d]: resource_name '%s' not found in resources", i, name)
			}
			if seen[name] {
				return fmt.Errorf("output[%d]: duplicate resource reference: %s", i, name)
			}
			seen[name] = true
		}
	}

	return nil
}
