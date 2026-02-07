package resources

import (
	"fmt"
	"sync"
)

var (
	registry = make(map[string]Provider)
	mu       sync.RWMutex
)

// Register registers a provider for a specific resource type
func Register(provider Provider) {
	mu.Lock()
	defer mu.Unlock()
	registry[provider.Type()] = provider
}

// Get retrieves a provider for a specific resource type
func Get(resourceType string) (Provider, error) {
	mu.RLock()
	defer mu.RUnlock()
	provider, ok := registry[resourceType]
	if !ok {
		return nil, fmt.Errorf("provider not found for resource type: %s", resourceType)
	}
	return provider, nil
}

// List returns all registered resource types
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
