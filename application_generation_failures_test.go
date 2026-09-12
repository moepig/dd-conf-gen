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

// Fails after an earlier discovery or render succeeds, requiring preservation of existing files and no newly saved outputs.
func TestApplicationDiscardsPartialGeneration(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"discovery", "render"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			app, provider := newTestApplication(t)
			dir := t.TempDir()
			first, second := filepath.Join(dir, "first.yaml"), filepath.Join(dir, "second.yaml")
			require.NoError(t, os.WriteFile(first, []byte("original"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "first.tmpl"), []byte("generated"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "second.tmpl"), []byte("prefix{{range .Resources}}{{.Tags.required}}{{end}}"), 0600))
			writer := new(mockOutputWriter)
			writer.Test(t)
			writer.On("Prepare", first).Return(nil).Once()
			writer.On("Prepare", second).Return(nil).Once()
			app.writer = writer
			provider.On("Prepare", mock.Anything).Return(nil).Twice()
			provider.On("Discover", mock.Anything, mock.MatchedBy(func(cfg providers.ProviderConfig) bool {
				return cfg.Region == "east"
			})).Return([]providers.Resource{{Host: "first.example"}}, nil).Once()
			var discoveryError error
			if stage == "discovery" {
				discoveryError = assert.AnError
			}
			provider.On("Discover", mock.Anything, mock.MatchedBy(func(cfg providers.ProviderConfig) bool {
				return cfg.Region == "west"
			})).Return([]providers.Resource{{Host: "second.example", Tags: map[string]string{}}}, discoveryError).Once()
			path := writeRunConfig(t, dir, config.GenConfig{
				Resources: []config.ResourceConfig{
					{Name: "first", Type: provider.Type(), Region: "east"},
					{Name: "second", Type: provider.Type(), Region: "west"},
				},
				Outputs: []config.OutputConfig{
					{Template: "first.tmpl", OutputFile: first, Data: config.OutputData{ResourceName: "first"}},
					{Template: "second.tmpl", OutputFile: second, Data: config.OutputData{ResourceName: "second"}},
				},
			})
			err := app.run(context.Background(), path)
			if stage == "discovery" {
				require.ErrorIs(t, err, assert.AnError)
				require.ErrorContains(t, err, "failed to discover resources for 'second'")
			} else {
				require.ErrorContains(t, err, "failed to execute template")
				require.ErrorContains(t, err, second)
			}
			writer.AssertNotCalled(t, "Write", mock.Anything, mock.Anything)
			writer.AssertExpectations(t)
			content, err := os.ReadFile(first)
			require.NoError(t, err)
			assert.Equal(t, "original", string(content))
			assert.NoFileExists(t, second)
		})
	}
}
