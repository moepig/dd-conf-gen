package providers

import (
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

func (p *registryMockProvider) Prepare(cfg ProviderConfig) (Discovery, error) {
	args := p.Called(cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(Discovery), args.Error(1)
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
	r = NewRegistry(map[string]Factory{
		first.Type():  func() Provider { return first },
		second.Type(): func() Provider { return second },
	})
	for _, expected := range []*registryMockProvider{first, second} {
		actual, err := r.Get(expected.Type())
		require.NoError(t, err)
		assert.Same(t, expected, actual)
	}
	assert.ElementsMatch(t, []string{"first", "second"}, r.List())
	_, err = r.Get("missing")
	require.ErrorContains(t, err, "available types: first, second")
	other := NewRegistry(map[string]Factory{"first": func() Provider { return &registryMockProvider{kind: "first"} }})
	otherProvider, err := other.Get("first")
	require.NoError(t, err)
	assert.NotSame(t, first, otherProvider)
	actual, err := r.Get("first")
	require.NoError(t, err)
	assert.Same(t, first, actual)
}

// Each lookup must construct an independent provider; factories may inspect the registry without deadlocking.
func TestRegistryFactories(t *testing.T) {
	t.Parallel()
	var r *Registry
	r = NewRegistry(map[string]Factory{"test": func() Provider {
		assert.Contains(t, r.List(), "test")
		return &registryMockProvider{kind: "test"}
	}})
	first, err := r.Get("test")
	require.NoError(t, err)
	second, err := r.Get("test")
	require.NoError(t, err)
	assert.NotSame(t, first, second)
	for _, factory := range []Factory{nil, func() Provider { return nil }, func() Provider { return (*registryMockProvider)(nil) }, func() Provider { return &registryMockProvider{kind: "wrong"} }} {
		invalid := NewRegistry(map[string]Factory{"invalid": factory})
		_, err := invalid.Get("invalid")
		require.Error(t, err)
	}
}

// Looks up and lists an immutable set of factories concurrently and requires independent provider results.
func TestRegistryConcurrentAccess(t *testing.T) {
	t.Parallel()
	const count = 32
	factories := make(map[string]Factory, count)
	for i := range count {
		kind := fmt.Sprintf("provider-%d", i)
		factories[kind] = func() Provider { return &registryMockProvider{kind: kind} }
	}
	r := NewRegistry(factories)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			kind := fmt.Sprintf("provider-%d", i)
			actual, err := r.Get(kind)
			if assert.NoError(t, err) {
				assert.Equal(t, kind, actual.Type())
			}
			assert.Contains(t, r.List(), kind)
		}()
	}
	wg.Wait()
	assert.Len(t, r.List(), count)
}

// Mutates constructor input and a returned list and requires the registry to retain its original registrations.
func TestRegistryOwnsRegistrations(t *testing.T) {
	t.Parallel()
	p := &registryMockProvider{kind: "test"}
	factories := map[string]Factory{"test": func() Provider { return p }}
	r := NewRegistry(factories)
	delete(factories, "test")
	factories["other"] = nil
	list := r.List()
	require.Equal(t, []string{"test"}, list)
	list[0] = "changed"
	assert.Equal(t, []string{"test"}, r.List())
	actual, err := r.Get("test")
	require.NoError(t, err)
	assert.Same(t, p, actual)
	_, err = r.Get("other")
	require.Error(t, err)
}
