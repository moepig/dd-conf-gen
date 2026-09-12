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

// Holds mocked destination preparation and write calls and errors.
type mockOutputWriter struct{ mock.Mock }

// Records path and returns the configured error, or validates path and returns its destination without writing.
func (w *mockOutputWriter) Prepare(path string) (output.Destination, error) {
	if err := w.Called(path).Error(0); err != nil {
		return output.Destination{}, err
	}
	return (output.FileWriter{}).Prepare(path)
}

// Records destination.Path() and content and returns the configured error.
func (w *mockOutputWriter) Write(destination output.Destination, content []byte) error {
	return w.Called(destination.Path(), content).Error(0)
}

// Supplies a later invalid or unregistered resource with mocked providers and output writing; requires an error before discovery or saving.
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

// Supplies a later malformed template or a mocked save error; requires no writes for the template error and no subsequent writes for the save error.
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

// Aggregates mocked searches with overlapping endpoints and verifies deduplication, first-definition tags, and deterministic ordering in the saved output.
func TestApplicationAggregatesResources(t *testing.T) {
	t.Parallel()
	app, p := newTestApplication(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.tmpl"), []byte(`{{range .Resources}}{{.Host}}:{{.Port}}/{{index .Tags "env"}};{{end}}`), 0600))
	p.On("Prepare", mock.Anything).Return(nil).Twice()
	p.On("Discover", mock.Anything, mock.MatchedBy(func(c providers.ProviderConfig) bool { return c.Region == "east" })).Return([]providers.Resource{
		{Host: "z", Port: 2, Tags: map[string]string{"env": "first"}},
		{Host: "a", Port: 2},
	}, nil).Once()
	p.On("Discover", mock.Anything, mock.MatchedBy(func(c providers.ProviderConfig) bool { return c.Region == "west" })).Return([]providers.Resource{
		{Host: "z", Port: 2, Tags: map[string]string{"env": "second"}},
		{Host: "a", Port: 1},
	}, nil).Once()
	dest := filepath.Join(dir, "out.yaml")
	path := writeRunConfig(t, dir, config.GenConfig{
		Resources: []config.ResourceConfig{{Name: "a", Type: p.Type(), Region: "east"}, {Name: "b", Type: p.Type(), Region: "west"}},
		Outputs:   []config.OutputConfig{{Template: "test.tmpl", OutputFile: dest, Data: config.OutputData{ResourceNames: []string{"a", "b"}}}},
	})
	require.NoError(t, app.run(context.Background(), path))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "a:1/;a:2/;z:2/first;", string(data))
}

// Uses empty mocked searches with existing and absent files to verify every empty-result policy and that a later error prevents earlier saves.
func TestApplicationEmptyResults(t *testing.T) {
	t.Parallel()
	for _, policy := range []string{"", "render", "keep", "error", "invalid"} {
		t.Run(policy, func(t *testing.T) {
			t.Parallel()
			for _, existing := range []bool{false, true} {
				app, p := newTestApplication(t)
				dir := t.TempDir()
				first, dest := filepath.Join(dir, "first.yaml"), filepath.Join(dir, "out.yaml")
				require.NoError(t, os.WriteFile(first, []byte("original"), 0600))
				if existing {
					require.NoError(t, os.WriteFile(dest, []byte("original"), 0600))
				}
				require.NoError(t, os.WriteFile(filepath.Join(dir, "test.tmpl"), []byte("instances: []\n"), 0600))
				if policy != "invalid" {
					p.On("Prepare", mock.Anything).Return(nil).Once()
					p.On("Discover", mock.Anything, mock.Anything).Return(nil, nil).Once()
				}
				path := writeRunConfig(t, dir, config.GenConfig{
					Resources: []config.ResourceConfig{{Name: "r", Type: p.Type(), Region: "east"}},
					Outputs: []config.OutputConfig{
						{Template: "test.tmpl", OutputFile: first, Data: config.OutputData{ResourceName: "r"}},
						{Template: "test.tmpl", OutputFile: dest, OnEmpty: policy, Data: config.OutputData{ResourceName: "r"}},
					},
				})
				err := app.run(context.Background(), path)
				failed := policy == "error" || policy == "invalid"
				if failed {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				firstData, err := os.ReadFile(first)
				require.NoError(t, err)
				if failed {
					assert.Equal(t, "original", string(firstData))
				} else {
					assert.Equal(t, "instances: []\n", string(firstData))
				}
				if policy == "keep" || failed {
					if existing {
						data, err := os.ReadFile(dest)
						require.NoError(t, err)
						assert.Equal(t, "original", string(data))
					} else {
						assert.NoFileExists(t, dest)
					}
				} else {
					data, err := os.ReadFile(dest)
					require.NoError(t, err)
					assert.Equal(t, "instances: []\n", string(data))
				}
			}
		})
	}
}
