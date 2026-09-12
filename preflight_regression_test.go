package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Supplies ancestor output paths in both orders, including symlink aliases, and requires rejection before discovery or filesystem changes.
func TestApplicationRejectsAncestorOutputs(t *testing.T) {
	t.Parallel()
	for _, alias := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			name := map[bool]string{false: "direct", true: "symlink"}[alias] + "/" + map[bool]string{false: "parent first", true: "child first"}[reverse]
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				app, provider := newTestApplication(t)
				provider.On("Prepare", mock.Anything).Return(nil).Once()
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "test.tmpl"), []byte("generated"), 0600))
				parent := filepath.Join(dir, "conf")
				child := filepath.Join(parent, "nested", "redis.yaml")
				if alias {
					link := filepath.Join(dir, "alias")
					require.NoError(t, os.Symlink(dir, link))
					child = filepath.Join(link, "conf", "nested", "redis.yaml")
				}
				paths := []string{parent, child}
				if reverse {
					paths[0], paths[1] = paths[1], paths[0]
				}
				cfg := config.GenConfig{Resources: []config.ResourceConfig{{Name: "redis", Type: provider.Type(), Region: "east"}}}
				for _, path := range paths {
					cfg.Outputs = append(cfg.Outputs, config.OutputConfig{Template: "test.tmpl", OutputFile: path, Data: config.OutputData{ResourceName: "redis"}})
				}
				err := app.run(context.Background(), writeRunConfig(t, dir, cfg))
				require.ErrorContains(t, err, "output_file is inside output[")
				provider.AssertNotCalled(t, "Discover", mock.Anything, mock.Anything)
				_, err = os.Stat(parent)
				assert.True(t, os.IsNotExist(err), "output paths must remain absent")
			})
		}
	}
}

// Generates outputs whose paths share a textual prefix without an ancestor relationship, requiring both files to be saved.
func TestApplicationAllowsOutputPathPrefixes(t *testing.T) {
	t.Parallel()
	app, provider := newTestApplication(t)
	provider.On("Prepare", mock.Anything).Return(nil).Once()
	provider.On("Discover", mock.Anything, mock.Anything).Return(nil, nil).Once()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.tmpl"), []byte("generated"), 0600))
	cfg := config.GenConfig{Resources: []config.ResourceConfig{{Name: "redis", Type: provider.Type(), Region: "east"}}}
	for _, path := range []string{filepath.Join(dir, "conf"), filepath.Join(dir, "conf.d", "redis.yaml")} {
		cfg.Outputs = append(cfg.Outputs, config.OutputConfig{Template: "test.tmpl", OutputFile: path, Data: config.OutputData{ResourceName: "redis"}})
	}
	require.NoError(t, app.run(context.Background(), writeRunConfig(t, dir, cfg)))
	for _, out := range cfg.Outputs {
		data, err := os.ReadFile(out.OutputFile)
		require.NoError(t, err)
		assert.Equal(t, "generated", string(data))
	}
}

// Passes positional arguments around CLI options and requires exit code 2 without version output, provider preparation, discovery, or saving.
func TestCLIRejectsPositionalArguments(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"leading", "trailing", "before timeout", "after terminator", "with version"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			app, provider := newTestApplication(t)
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "test.tmpl"), []byte("generated"), 0600))
			dest := filepath.Join(dir, "out.yaml")
			path := writeRunConfig(t, dir, config.GenConfig{
				Resources: []config.ResourceConfig{{Name: "redis", Type: provider.Type(), Region: "east"}},
				Outputs:   []config.OutputConfig{{Template: "test.tmpl", OutputFile: dest, Data: config.OutputData{ResourceName: "redis"}}},
			})
			args := []string{"-config", path}
			switch scenario {
			case "leading":
				args = append([]string{"stray"}, args...)
			case "trailing":
				args = append(args, "stray")
			case "before timeout":
				args = append(args, "stray", "-timeout=0")
			case "after terminator":
				args = append(args, "--", "-timeout=0")
			case "with version":
				args = append(args, "-version", "stray")
			}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, 2, app.runCLI(context.Background(), args, &stdout, &stderr))
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "positional arguments are not supported")
			provider.AssertNotCalled(t, "Prepare", mock.Anything)
			provider.AssertNotCalled(t, "Discover", mock.Anything, mock.Anything)
			assert.NoFileExists(t, dest)
		})
	}
}
