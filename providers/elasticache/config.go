package elasticache

import (
	"context"
	"fmt"

	"github.com/moepig/dd-conf-gen/internal/tagfilter"
	"github.com/moepig/dd-conf-gen/providers"
)

// Holds a validated region and independently owned tag filters.
type discoveryConfig struct {
	region     string
	tags       map[string]string
	conditions tagfilter.Conditions
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

// Converts cfg into independently owned settings, or returns zero settings and an error for a missing region or invalid filters.
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
