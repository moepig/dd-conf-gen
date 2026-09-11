package elasticache

import (
	"fmt"

	"github.com/moepig/dd-conf-gen/providers"
)

type discoveryConfig struct {
	region string
	tags   map[string]string
}

// Validates provider settings without loading AWS configuration or calling APIs.
func (p *Provider) ValidateConfig(cfg providers.ProviderConfig) error {
	_, err := parseConfig(cfg)
	return err
}

// Converts raw provider settings into independently owned, typed discovery settings.
// Missing regions, unsupported filters, and non-string tag values return an error.
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
