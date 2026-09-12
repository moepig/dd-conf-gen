package providers

import "context"

// Prepares resource searches without external access or mutation of the supplied configuration.
type Provider interface {
	// Type returns the resource type handled by this provider
	Type() string

	// Returns a search with privately owned, validated settings, or an error for invalid configuration.
	Prepare(config ProviderConfig) (Discovery, error)
}

// Executes a prepared search with cancellation and returns resources or a discovery error.
type Discovery func(context.Context) ([]Resource, error)

// ProviderConfig represents configuration for a provider
type ProviderConfig struct {
	Region  string
	Filters map[string]interface{}
}
