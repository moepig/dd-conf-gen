package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type mockProvider struct {
	mock.Mock
}

func (p *mockProvider) Type() string { return "elasticache_redis" }

func (p *mockProvider) ValidateConfig(cfg providers.ProviderConfig) error {
	return p.Called(cfg).Error(0)
}

func (p *mockProvider) Discover(ctx context.Context, cfg providers.ProviderConfig) ([]providers.Resource, error) {
	args := p.Called(ctx, cfg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]providers.Resource), args.Error(1)
}

// Installs a mock provider for one test and restores the registered provider afterward.
func installMockProvider(t *testing.T) *mockProvider {
	t.Helper()
	p := new(mockProvider)
	p.Test(t)
	original, err := providers.Get(p.Type())
	require.NoError(t, err)
	providers.Register(p)
	t.Cleanup(func() {
		providers.Register(original)
		p.AssertExpectations(t)
	})
	return p
}

// Writes a generation configuration in the test directory and returns its path.
func writeRunConfig(t *testing.T, dir string, cfg config.GenConfig) string {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(dir, "gen-config.yaml")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}

// Uses mocked discovery to verify region/filter forwarding, resource selection, template paths, and output creation.
func TestRunGeneratesOutputs(t *testing.T) {
	p := installMockProvider(t)
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "templates"), 0755))
	templatePath := filepath.Join(dir, "templates", "redis.tmpl")
	require.NoError(t, os.WriteFile(templatePath, []byte(`{{range .Resources}}{{.Host}}:{{.Port}} {{index .Tags "env"}} {{index .Metadata "ClusterName"}}
{{end}}`), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	filters := map[string]interface{}{"tags": map[string]interface{}{"env": "prod"}}
	p.On("Discover", ctx, providers.ProviderConfig{Region: "us-east-1", Filters: filters}).Return([]providers.Resource{{
		Host: "east.example.com", Port: 6379, Tags: map[string]string{"env": "prod"}, Metadata: map[string]interface{}{"ClusterName": "east"},
	}}, nil).Once()
	p.On("Discover", ctx, providers.ProviderConfig{Region: "ap-northeast-1", Filters: map[string]interface{}{}}).Return([]providers.Resource{{
		Host: "tokyo.example.com", Port: 6380, Tags: map[string]string{"env": "test"}, Metadata: map[string]interface{}{"ClusterName": "tokyo"},
	}}, nil).Once()
	firstOutput := filepath.Join(dir, "out", "east.yaml")
	secondOutput := filepath.Join(dir, "out", "tokyo.yaml")
	path := writeRunConfig(t, dir, config.GenConfig{
		Resources: []config.ResourceConfig{
			{Name: "east", Type: p.Type(), Region: "us-east-1", Filters: filters},
			{Name: "tokyo", Type: p.Type(), Region: "ap-northeast-1"},
		},
		Outputs: []config.OutputConfig{
			{Template: "templates/redis.tmpl", OutputFile: firstOutput, Data: config.OutputData{ResourceName: "east"}},
			{Template: templatePath, OutputFile: secondOutput, Data: config.OutputData{ResourceName: "tokyo"}},
		},
	})
	require.NoError(t, run(ctx, path))
	for output, expected := range map[string]string{
		firstOutput: "east.example.com:6379 prod east\n", secondOutput: "tokyo.example.com:6380 test tokyo\n",
	} {
		content, err := os.ReadFile(output)
		require.NoError(t, err)
		assert.Equal(t, expected, string(content))
	}
}

// Discovery and rendering errors must leave an existing output intact; filesystem errors must be returned.
func TestRunFailures(t *testing.T) {
	for _, name := range []string{"invalid config", "unknown provider", "discovery error", "missing template", "invalid template", "execution error", "output directory error", "output file error"} {
		t.Run(name, func(t *testing.T) {
			p := installMockProvider(t)
			dir := t.TempDir()
			output := filepath.Join(dir, "output.yaml")
			require.NoError(t, os.WriteFile(output, []byte("existing configuration"), 0600))
			templatePath := filepath.Join(dir, "redis.tmpl")
			require.NoError(t, os.WriteFile(templatePath, []byte("{{range .Resources}}{{.Host}}{{end}}"), 0600))
			cfg := config.GenConfig{
				Resources: []config.ResourceConfig{{Name: "redis", Type: p.Type(), Region: "us-east-1"}},
				Outputs:   []config.OutputConfig{{Template: templatePath, OutputFile: output, Data: config.OutputData{ResourceName: "redis"}}},
			}
			var discoverErr error
			var expectedError string
			switch name {
			case "invalid config":
				cfg.Resources = nil
				expectedError = "failed to load generation config"
			case "unknown provider":
				cfg.Resources[0].Type = "unknown-test-provider"
				expectedError = "failed to get provider for resource 'redis'"
			case "discovery error":
				discoverErr = assert.AnError
				expectedError = "failed to discover resources for 'redis'"
			case "missing template":
				cfg.Outputs[0].Template = filepath.Join(dir, "missing.tmpl")
				expectedError = "failed to read template file"
			case "invalid template":
				require.NoError(t, os.WriteFile(templatePath, []byte("{{"), 0600))
				expectedError = "failed to parse template"
			case "execution error":
				require.NoError(t, os.WriteFile(templatePath, []byte("{{.NonexistentField}}"), 0600))
				expectedError = "failed to execute template"
			case "output directory error":
				cfg.Outputs[0].OutputFile = filepath.Join(output, "child.yaml")
				expectedError = "failed to create output directory"
			case "output file error":
				cfg.Outputs[0].OutputFile = dir
				expectedError = "failed to write output file"
			}
			if name != "invalid config" && name != "unknown provider" {
				p.On("Discover", mock.Anything, providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{}}).Return([]providers.Resource{{Host: "redis.example.com"}}, discoverErr).Once()
			}
			err := run(context.Background(), writeRunConfig(t, dir, cfg))
			require.ErrorContains(t, err, expectedError)
			if discoverErr != nil {
				assert.ErrorIs(t, err, discoverErr)
			}
			content, err := os.ReadFile(output)
			require.NoError(t, err)
			assert.Equal(t, "existing configuration", string(content))
		})
	}
}

// An empty discovery result must still render the template and replace obsolete output.
func TestRunEmptyResources(t *testing.T) {
	p := installMockProvider(t)
	p.On("Discover", mock.Anything, providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{}}).Return(nil, nil).Once()
	dir := t.TempDir()
	output := filepath.Join(dir, "output.yaml")
	require.NoError(t, os.WriteFile(output, []byte("obsolete configuration"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "redis.tmpl"), []byte("instances: [{{range .Resources}}{{.Host}}{{end}}]\n"), 0600))
	path := writeRunConfig(t, dir, config.GenConfig{
		Resources: []config.ResourceConfig{{Name: "redis", Type: p.Type(), Region: "us-east-1"}},
		Outputs:   []config.OutputConfig{{Template: "redis.tmpl", OutputFile: output, Data: config.OutputData{ResourceName: "redis"}}},
	})
	require.NoError(t, run(context.Background(), path))
	content, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Equal(t, "instances: []\n", string(content))
}

// Runs the CLI in a subprocess so exit codes and separate output streams can be checked.
func TestCLI(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	missingConfig := filepath.Join(t.TempDir(), "missing.yaml")
	type cliTest struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}
	tests := []cliTest{
		{name: "version", args: []string{"-version"}, stdout: version + "\n"},
		{name: "help", args: []string{"-help"}, stderr: "Usage of dd-conf-gen:"},
		{name: "missing config", code: 1, stderr: "-config option is required"},
		{name: "invalid log level", args: []string{"-log-level=invalid"}, code: 1, stderr: "invalid log level 'invalid'"},
		{name: "unknown flag", args: []string{"-unknown"}, code: 2, stderr: "flag provided but not defined"},
	}
	for _, level := range []string{"debug", "info", "warn", "error"} {
		tests = append(tests, cliTest{name: level, args: []string{"-log-level=" + level, "-config=" + missingConfig}, code: 1, stderr: "failed to read generation config file"})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(executable, append([]string{"-test.run=^TestCLIProcess$", "--"}, tt.args...)...)
			cmd.Env = append(os.Environ(), "DD_CONF_GEN_CLI_TEST=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if tt.code == 0 {
				require.NoError(t, err)
			} else {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
			}
			assert.Equal(t, tt.code, cmd.ProcessState.ExitCode())
			assert.Equal(t, tt.stdout, stdout.String())
			if tt.stderr == "" {
				assert.Empty(t, stderr.String())
			} else {
				assert.Contains(t, stderr.String(), tt.stderr)
			}
		})
	}
}

// Enters the real CLI only in the subprocess created by TestCLI.
func TestCLIProcess(t *testing.T) {
	if os.Getenv("DD_CONF_GEN_CLI_TEST") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"dd-conf-gen"}, os.Args[i+1:]...)
			flag.CommandLine = flag.NewFlagSet("dd-conf-gen", flag.ExitOnError)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing CLI argument separator")
}
