package aurora

import (
	"context"
	"fmt"

	"github.com/moepig/dd-conf-gen/providers"
)

// Holds a validated region and independently owned cluster tag filters.
type discoveryConfig struct {
	region string
	tags   map[string]string
}

// Validates cfg without external access or mutation and returns a search retaining independent settings and the current client reference, or a configuration error.
func (p *Provider) Prepare(cfg providers.ProviderConfig) (providers.Discovery, error) {
	settings, err := parseConfig(cfg)
	if err != nil {
		return nil, err
	}
	local := *p
	return func(ctx context.Context) ([]providers.Resource, error) {
		return local.discover(ctx, settings)
	}, nil
}

// Converts cfg into independently owned settings, or returns zero settings and an error for a missing region, unsupported filters, or invalid tag types.
func parseConfig(cfg providers.ProviderConfig) (discoveryConfig, error) {
	if cfg.Region == "" {
		return discoveryConfig{}, fmt.Errorf("region is required")
	}
	settings := discoveryConfig{region: cfg.Region, tags: make(map[string]string)}
	for key, value := range cfg.Filters {
		if key != "tags" {
			return discoveryConfig{}, fmt.Errorf("unsupported filter: %s", key)
		}
		tags, ok := value.(map[string]interface{})
		if !ok {
			return discoveryConfig{}, fmt.Errorf("filters.tags must be a map")
		}
		for key, value := range tags {
			tag, ok := value.(string)
			if !ok {
				return discoveryConfig{}, fmt.Errorf("filters.tags.%s must be a string", key)
			}
			settings.tags[key] = tag
		}
	}
	return settings, nil
}
