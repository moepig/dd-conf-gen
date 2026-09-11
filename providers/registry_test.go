package providers

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type registryMockProvider struct {
	mock.Mock
	kind string
}

func (p *registryMockProvider) Type() string { return p.kind }

func (p *registryMockProvider) ValidateConfig(cfg ProviderConfig) error {
	return p.Called(cfg).Error(0)
}

func (p *registryMockProvider) Discover(ctx context.Context, cfg ProviderConfig) ([]Resource, error) {
	args := p.Called(ctx, cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Resource), args.Error(1)
}

// Isolates registry entries for a test and restores the original entries after all workers finish.
func isolateRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	original := registry
	registry = make(map[string]Provider)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		registry = original
		mu.Unlock()
	})
}

// Registered mock providers must be retrievable by type and listed independently of registration order.
func TestRegistry(t *testing.T) {
	isolateRegistry(t)
	assert.Empty(t, List())
	p, err := Get("missing")
	require.ErrorContains(t, err, "provider not found for resource type: missing")
	assert.Nil(t, p)
	first := &registryMockProvider{kind: "first"}
	second := &registryMockProvider{kind: "second"}
	Register(first)
	Register(second)
	for _, expected := range []*registryMockProvider{first, second} {
		actual, err := Get(expected.Type())
		require.NoError(t, err)
		assert.Same(t, expected, actual)
	}
	assert.ElementsMatch(t, []string{"first", "second"}, List())
}

// Concurrent registration, lookup, and listing must preserve every provider without data races.
func TestRegistryConcurrentAccess(t *testing.T) {
	isolateRegistry(t)
	const count = 32
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &registryMockProvider{kind: fmt.Sprintf("provider-%d", i)}
			Register(p)
			actual, err := Get(p.Type())
			assert.NoError(t, err)
			assert.Same(t, p, actual)
			assert.Contains(t, List(), p.Type())
		}()
	}
	wg.Wait()
	assert.Len(t, List(), count)
}
