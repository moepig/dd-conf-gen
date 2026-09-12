package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockOutputWriter struct{ mock.Mock }

func (w *mockOutputWriter) Validate(path string) error { return w.Called(path).Error(0) }

func (w *mockOutputWriter) Write(path string, content []byte) error {
	return w.Called(path, content).Error(0)
}

// A later invalid or unregistered resource must fail before any discovery or save occurs.
func TestApplicationPreflight(t *testing.T) {
	t.Parallel()
	for _, unknown := range []bool{false, true} {
		name := "invalid settings"
		if unknown {
			name = "unknown provider"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			app, provider := newTestApplication(t)
			provider.On("ValidateConfig", providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{}}).Return(nil).Once()
			secondType := provider.Type()
			if unknown {
				secondType = "unknown"
			} else {
				provider.On("ValidateConfig", providers.ProviderConfig{Region: "us-west-2", Filters: map[string]interface{}{}}).Return(assert.AnError).Once()
			}
			writer := new(mockOutputWriter)
			writer.Test(t)
			app.writer = writer
			dir := t.TempDir()
			path := writeRunConfig(t, dir, config.GenConfig{
				Resources: []config.ResourceConfig{
					{Name: "first", Type: provider.Type(), Region: "us-east-1"},
					{Name: "second", Type: secondType, Region: "us-west-2"},
				},
				Outputs: []config.OutputConfig{{Template: "unused.tmpl", OutputFile: filepath.Join(dir, "out.yaml"), Data: config.OutputData{ResourceName: "first"}}},
			})
			err := app.run(context.Background(), path)
			require.ErrorContains(t, err, "resource 'second'")
			if !unknown {
				assert.ErrorIs(t, err, assert.AnError)
			}
			provider.AssertNotCalled(t, "Discover", mock.Anything, mock.Anything)
			writer.AssertNotCalled(t, "Write", mock.Anything, mock.Anything)
		})
	}
}

// A later rendering failure must prevent every write; a save failure must be returned and stop subsequent writes.
func TestApplicationOutputFailures(t *testing.T) {
	t.Parallel()
	for _, renderFailure := range []bool{true, false} {
		name := "save failure"
		if renderFailure {
			name = "later template failure"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			app, provider := newTestApplication(t)
			provider.On("ValidateConfig", mock.Anything).Return(nil).Once()
			writer := new(mockOutputWriter)
			writer.Test(t)
			app.writer = writer
			dir := t.TempDir()
			first := filepath.Join(dir, "first.yaml")
			second := filepath.Join(dir, "second.yaml")
			require.NoError(t, os.WriteFile(first, []byte("original"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "first.tmpl"), []byte("generated"), 0600))
			secondTemplate := "generated"
			if renderFailure {
				secondTemplate = "{{"
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "second.tmpl"), []byte(secondTemplate), 0600))
			path := writeRunConfig(t, dir, config.GenConfig{
				Resources: []config.ResourceConfig{{Name: "redis", Type: provider.Type(), Region: "us-east-1"}},
				Outputs: []config.OutputConfig{
					{Template: "first.tmpl", OutputFile: first, Data: config.OutputData{ResourceName: "redis"}},
					{Template: "second.tmpl", OutputFile: second, Data: config.OutputData{ResourceName: "redis"}},
				},
			})
			if !renderFailure {
				provider.On("Discover", mock.Anything, mock.Anything).Return(nil, nil).Once()
				writer.On("Validate", first).Return(nil).Once()
				writer.On("Validate", second).Return(nil).Once()
				writer.On("Write", first, []byte("generated")).Return(assert.AnError).Once()
			}
			err := app.run(context.Background(), path)
			if renderFailure {
				require.ErrorContains(t, err, "failed to render template")
				provider.AssertNotCalled(t, "Discover", mock.Anything, mock.Anything)
				writer.AssertNotCalled(t, "Write", mock.Anything, mock.Anything)
			} else {
				require.ErrorIs(t, err, assert.AnError)
				writer.AssertNotCalled(t, "Write", second, mock.Anything)
			}
			writer.AssertExpectations(t)
			content, err := os.ReadFile(first)
			require.NoError(t, err)
			assert.Equal(t, "original", string(content))
			assert.NoFileExists(t, second)
		})
	}
}
