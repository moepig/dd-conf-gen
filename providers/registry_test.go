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

// Registered mock providers must be retrievable by type and listed independently of registration order.
func TestRegistry(t *testing.T) {
	t.Parallel()
	r := &Registry{}
	assert.Empty(t, r.List())
	p, err := r.Get("missing")
	require.ErrorContains(t, err, "provider not found for resource type: missing")
	assert.Nil(t, p)
	first := &registryMockProvider{kind: "first"}
	second := &registryMockProvider{kind: "second"}
	r.Register(first)
	r.Register(second)
	for _, expected := range []*registryMockProvider{first, second} {
		actual, err := r.Get(expected.Type())
		require.NoError(t, err)
		assert.Same(t, expected, actual)
	}
	assert.ElementsMatch(t, []string{"first", "second"}, r.List())
	other := &Registry{}
	other.Register(&registryMockProvider{kind: "first"})
	actual, err := r.Get("first")
	require.NoError(t, err)
	assert.Same(t, first, actual)
}

// Concurrent registration, lookup, and listing must preserve every provider without data races.
func TestRegistryConcurrentAccess(t *testing.T) {
	t.Parallel()
	r := &Registry{}
	const count = 32
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &registryMockProvider{kind: fmt.Sprintf("provider-%d", i)}
			r.Register(p)
			actual, err := r.Get(p.Type())
			assert.NoError(t, err)
			assert.Same(t, p, actual)
			assert.Contains(t, r.List(), p.Type())
		}()
	}
	wg.Wait()
	assert.Len(t, r.List(), count)
}
