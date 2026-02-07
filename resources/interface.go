package resources

import "context"

// Provider discovers and returns cloud resources
type Provider interface {
	// Type returns the resource type handled by this provider
	Type() string

	// Discover retrieves resources based on the configuration
	Discover(ctx context.Context, config ProviderConfig) ([]Resource, error)

	// ValidateConfig checks if the provider configuration is valid
	ValidateConfig(config ProviderConfig) error
}

// ProviderConfig represents configuration for a provider
type ProviderConfig struct {
	Region     string
	Filters    map[string]interface{}
	TagMapping map[string]string
}
