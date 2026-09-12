package providers

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Creates a provider instance without performing resource discovery.
type Factory func() Provider

// Holds an immutable set of provider factories. The zero value is an empty registry.
type Registry struct {
	providers map[string]Factory
}

// Copies factory registrations so subsequent changes to the input map cannot affect lookups.
func NewRegistry(factories map[string]Factory) *Registry {
	return &Registry{providers: maps.Clone(factories)}
}

// Creates a provider for resourceType, or returns an error if it is unregistered or its factory is invalid.
func (r *Registry) Get(resourceType string) (Provider, error) {
	factory, ok := r.providers[resourceType]
	if !ok {
		return nil, fmt.Errorf("provider not found for resource type: %s (available types: %s)", resourceType, strings.Join(r.List(), ", "))
	}
	if factory == nil {
		return nil, fmt.Errorf("nil provider factory for resource type: %s", resourceType)
	}
	provider := factory()
	if isNilProvider(provider) || provider.Type() != resourceType {
		return nil, fmt.Errorf("invalid provider factory result for resource type: %s", resourceType)
	}
	return provider, nil
}

// Detects both nil interfaces and nil values held by provider interfaces without invoking provider methods.
func isNilProvider(provider Provider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Returns the registered resource types in lexical order.
func (r *Registry) List() []string {
	return slices.Sorted(maps.Keys(r.providers))
}
