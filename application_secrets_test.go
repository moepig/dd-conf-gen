package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Records secret requests for application tests.
type mockSecretClient struct{ mock.Mock }

// Returns the configured secret response or error.
func (m *mockSecretClient) GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*secretsmanager.GetSecretValueOutput), args.Error(1)
}

// Generates YAML with mocked secrets and checks safe quoting, restricted file permissions, and log redaction on successful and failing runs.
func TestApplicationSecrets(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"new", "existing", "api error", "template error", "invalid reference", "keep"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			app, p := newTestApplication(t)
			dir := t.TempDir()
			dest := filepath.Join(dir, "out.yaml")
			first := filepath.Join(dir, "first.yaml")
			require.NoError(t, os.WriteFile(first, []byte("original"), 0600))
			if scenario == "existing" {
				require.NoError(t, os.WriteFile(dest, []byte("original"), 0644))
				require.NoError(t, os.Chmod(dest, 0644))
			}
			source := "password: {{.Secrets.password | quote}}\n"
			if scenario == "template error" {
				source = `{{call .Secrets.password}}`
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.tmpl"), []byte(source), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.tmpl"), []byte("generated"), 0600))
			secretValue := "SECRET_VALUE: # quoted\n\"password\""
			client := new(mockSecretClient)
			client.Test(t)
			if scenario != "invalid reference" {
				p.On("Prepare", mock.Anything).Return(nil).Once()
				var resources []providers.Resource
				if scenario != "keep" {
					resources = []providers.Resource{{Host: "redis", Port: 6379}}
				}
				p.On("Discover", mock.Anything, mock.Anything).Return(resources, nil).Once()
				if scenario != "keep" {
					if scenario == "api error" {
						client.On("GetSecretValue", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New(secretValue)).Once()
					} else {
						client.On("GetSecretValue", mock.Anything, mock.Anything, mock.Anything).Return(&secretsmanager.GetSecretValueOutput{SecretString: aws.String(secretValue)}, nil).Once()
					}
				}
			}
			app.secretClientFactory = func(context.Context, string) (secrets.API, error) { return client, nil }
			ref := secrets.Reference{Region: "east", SecretID: "password"}
			if scenario == "invalid reference" {
				ref.Region = ""
			}
			out := config.OutputConfig{Template: "secret.tmpl", OutputFile: dest, Data: config.OutputData{ResourceName: "r", Secrets: map[string]secrets.Reference{"password": ref}}}
			if scenario == "keep" {
				out.OnEmpty = "keep"
			}
			path := writeRunConfig(t, dir, config.GenConfig{
				Resources: []config.ResourceConfig{{Name: "r", Type: p.Type(), Region: "east"}},
				Outputs:   []config.OutputConfig{{Template: "plain.tmpl", OutputFile: first, Data: config.OutputData{ResourceName: "r"}}, out},
			})
			var stdout, stderr bytes.Buffer
			status := app.runCLI(context.Background(), []string{"-config", path, "-log-level", "debug"}, &stdout, &stderr)
			assert.NotContains(t, stderr.String(), "SECRET_VALUE")
			assert.Empty(t, stdout.String())
			if scenario == "api error" || scenario == "template error" || scenario == "invalid reference" {
				assert.Equal(t, 1, status)
				assert.NoFileExists(t, dest)
				data, err := os.ReadFile(first)
				require.NoError(t, err)
				assert.Equal(t, "original", string(data))
			} else {
				require.Equal(t, 0, status, stderr.String())
				if scenario == "keep" {
					assert.NoFileExists(t, dest)
				} else {
					data, err := os.ReadFile(dest)
					require.NoError(t, err)
					var parsed map[string]string
					require.NoError(t, yaml.Unmarshal(data, &parsed))
					assert.Equal(t, secretValue, parsed["password"])
					info, err := os.Stat(dest)
					require.NoError(t, err)
					assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
				}
			}
			client.AssertExpectations(t)
		})
	}
}
