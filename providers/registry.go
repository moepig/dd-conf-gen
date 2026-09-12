package providers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Creates a provider instance without performing resource discovery.
type Factory func() Provider

// Stores provider factories by resource type. The zero value is ready for use.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Factory
}

// Registers a factory by type, replacing an existing registration of that type.
func (r *Registry) Register(resourceType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[string]Factory)
	}
	r.providers[resourceType] = factory
}

// Creates a provider for resourceType, or returns an error if it is unregistered or its factory is invalid.
func (r *Registry) Get(resourceType string) (Provider, error) {
	r.mu.RLock()
	factory, ok := r.providers[resourceType]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider not found for resource type: %s (available types: %s)", resourceType, strings.Join(r.List(), ", "))
	}
	if factory == nil {
		return nil, fmt.Errorf("nil provider factory for resource type: %s", resourceType)
	}
	provider := factory()
	if provider == nil || provider.Type() != resourceType {
		return nil, fmt.Errorf("invalid provider factory result for resource type: %s", resourceType)
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
