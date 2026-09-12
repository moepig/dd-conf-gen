package config

import "github.com/moepig/dd-conf-gen/secrets"

// Holds resource search definitions and ordered output definitions.
type GenConfig struct {
	Resources []ResourceConfig `yaml:"resources"`
	Outputs   []OutputConfig   `yaml:"outputs"`
}

// Holds a named resource search definition with its type, region, and provider-specific filters.
type ResourceConfig struct {
	Name    string                 `yaml:"name"`
	Type    string                 `yaml:"type"`
	Region  string                 `yaml:"region"`
	Filters map[string]interface{} `yaml:"filters"`
}

// Holds a template path, output file path, and resource selection.
type OutputConfig struct {
	Template   string     `yaml:"template"`
	OutputFile string     `yaml:"output_file"`
	Data       OutputData `yaml:"data"`
	OnEmpty    string     `yaml:"on_empty,omitempty"`
}

// Holds resource selection and named secret references for an output.
type OutputData struct {
	ResourceName  string                       `yaml:"resource_name,omitempty"`
	ResourceNames []string                     `yaml:"resource_names,omitempty"`
	Secrets       map[string]secrets.Reference `yaml:"secrets,omitempty"`
}

// Returns the selected resource names in configured priority order.
func (d OutputData) Names() []string {
	if d.ResourceName != "" {
		return []string{d.ResourceName}
	}
	return d.ResourceNames
}
