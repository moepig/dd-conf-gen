package providers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Stores providers by resource type. The zero value is ready for use.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// Registers a provider by type, replacing an existing registration of that type.
func (r *Registry) Register(provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[string]Provider)
	}
	r.providers[provider.Type()] = provider
}

// Returns the provider for resourceType, or an error if it is unregistered.
func (r *Registry) Get(resourceType string) (Provider, error) {
	r.mu.RLock()
	provider, ok := r.providers[resourceType]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider not found for resource type: %s (available types: %s)", resourceType, strings.Join(r.List(), ", "))
	}
	return provider, nil
}

// Returns the registered resource types in lexical order.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}
