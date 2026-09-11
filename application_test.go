package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockOutputWriter struct{ mock.Mock }

func (w *mockOutputWriter) Write(path string, content []byte) error {
	return w.Called(path, content).Error(0)
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
			provider.On("Discover", mock.Anything, mock.Anything).Return(nil, nil).Once()
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
				writer.On("Write", first, []byte("generated")).Return(assert.AnError).Once()
			}
			err := app.run(context.Background(), path)
			if renderFailure {
				require.ErrorContains(t, err, "failed to render template")
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
