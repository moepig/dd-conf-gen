package elasticache

import (
	"context"
	"fmt"

	"github.com/moepig/dd-conf-gen/providers"
)

// Holds a validated region and an independently owned map of tag filters.
type discoveryConfig struct {
	region string
	tags   map[string]string
}

// Prepares a resource search from cfg without external access or mutation of cfg.
//
// Returns a search that retains independently owned settings and the current client references, or nil and a configuration error. The search accepts a context for cancellation and logging and returns resources or a discovery error.
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

// Converts cfg into independently owned, typed discovery settings.
//
// Returns settings, or a zero value and an error for a missing region, unsupported filters, a tags value other than map[string]interface{}, or non-string tag values.
func parseConfig(cfg providers.ProviderConfig) (discoveryConfig, error) {
	if cfg.Region == "" {
		return discoveryConfig{}, fmt.Errorf("region is required")
	}
	result := discoveryConfig{region: cfg.Region, tags: make(map[string]string)}
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
			result.tags[key] = tag
		}
	}
	return result, nil
}
