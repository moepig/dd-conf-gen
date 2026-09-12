package aurora

import (
	"context"
	"fmt"

	"github.com/moepig/dd-conf-gen/internal/providers"
	"github.com/moepig/dd-conf-gen/internal/tagfilter"
)

// Holds a validated region and independently owned cluster tag filters.
type discoveryConfig struct {
	region     string
	tags       map[string]string
	conditions tagfilter.Conditions
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
	tags, conditions, err := tagfilter.Parse(cfg.Filters)
	if err != nil {
		return discoveryConfig{}, err
	}
	return discoveryConfig{region: cfg.Region, tags: tags, conditions: conditions}, nil
}
