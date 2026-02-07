package config

// MetaConfig represents the entire meta configuration file
type MetaConfig struct {
	Version   string           `yaml:"version"`
	Resources []ResourceConfig `yaml:"resources"`
	Outputs   []OutputConfig   `yaml:"outputs"`
}

// ResourceConfig represents a resource definition
type ResourceConfig struct {
	Name    string                 `yaml:"name"`
	Type    string                 `yaml:"type"`
	Region  string                 `yaml:"region"`
	Filters map[string]interface{} `yaml:"filters"`
}

// OutputConfig represents an output definition
type OutputConfig struct {
	Template   string     `yaml:"template"`
	OutputFile string     `yaml:"output_file"`
	Data       OutputData `yaml:"data"`
}

// OutputData represents data passed to templates
type OutputData struct {
	ResourceName string `yaml:"resource_name"`
}
