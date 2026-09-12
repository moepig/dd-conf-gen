package providers

import "context"

// Prepares resource searches without external access or mutation of the supplied configuration.
type Provider interface {
	// Returns the resource type handled by this provider.
	Type() string

	// Validates config without external access or mutation and returns a search with independently owned settings, or an error for invalid configuration.
	Prepare(config ProviderConfig) (Discovery, error)
}

// Executes a prepared search using the supplied context for cancellation and returns resources or a discovery error.
type Discovery func(context.Context) ([]Resource, error)

// Holds a discovery region and unvalidated provider-specific filters.
type ProviderConfig struct {
	Region  string
	Filters map[string]interface{}
}
