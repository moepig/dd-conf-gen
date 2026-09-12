package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/output"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockOutputWriter struct{ mock.Mock }

func (w *mockOutputWriter) Prepare(path string) (output.Destination, error) {
	if err := w.Called(path).Error(0); err != nil {
		return output.Destination{}, err
	}
	return (output.FileWriter{}).Prepare(path)
}

func (w *mockOutputWriter) Write(destination output.Destination, content []byte) error {
	return w.Called(destination.Path(), content).Error(0)
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
			provider.On("Prepare", providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{}}).Return(nil).Once()
			secondType := provider.Type()
			if unknown {
				secondType = "unknown"
			} else {
				provider.On("Prepare", providers.ProviderConfig{Region: "us-west-2", Filters: map[string]interface{}{}}).Return(assert.AnError).Once()
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
			provider.On("Prepare", mock.Anything).Return(nil).Once()
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
				writer.On("Prepare", first).Return(nil).Once()
				writer.On("Prepare", second).Return(nil).Once()
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

// Exercises lexical and symlink aliases and requires duplicate rejection before discovery or saving.
func TestApplicationRejectsOutputAliases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "b", "child"), 0700))
	require.NoError(t, os.Symlink("b/child", filepath.Join(dir, "link")))
	require.NoError(t, os.Symlink(dir, filepath.Join(dir, "alias")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redis.tmpl"), []byte("generated"), 0600))
	abs := filepath.Join(dir, "b", "out.yaml")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	rel, err := filepath.Rel(cwd, abs)
	require.NoError(t, err)
	for _, second := range []string{abs, dir + "/b/../b/out.yaml", rel, dir + "/alias/b/out.yaml", dir + "/link/../out.yaml", dir + "/missing/../link/../out.yaml"} {
		t.Run(second, func(t *testing.T) {
			app, provider := newTestApplication(t)
			provider.On("Prepare", mock.Anything).Return(nil).Once()
			cfg := config.GenConfig{
				Resources: []config.ResourceConfig{{Name: "redis", Type: provider.Type(), Region: "us-east-1"}},
				Outputs: []config.OutputConfig{
					{Template: "redis.tmpl", OutputFile: abs, Data: config.OutputData{ResourceName: "redis"}},
					{Template: "redis.tmpl", OutputFile: second, Data: config.OutputData{ResourceName: "redis"}},
				},
			}
			require.ErrorContains(t, app.run(context.Background(), writeRunConfig(t, dir, cfg)), "duplicate output_file")
			provider.AssertNotCalled(t, "Discover", mock.Anything, mock.Anything)
			assert.NoFileExists(t, abs)
		})
	}
}
