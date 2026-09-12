package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/internal/config"
	"github.com/moepig/dd-conf-gen/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Discovers mocked resources and saves the distributed template, requiring distinct unresolved secret references for each team's endpoint.
func TestApplicationSecretReferences(t *testing.T) {
	t.Parallel()
	app, provider := newTestApplication(t)
	provider.On("Prepare", mock.Anything).Return(nil).Once()
	provider.On("Discover", mock.Anything, mock.Anything).Return([]providers.Resource{
		{Host: "a.example", Port: 3306, Tags: map[string]string{"team": "team-a"}},
		{Host: "b.example", Port: 3306, Tags: map[string]string{"team": "team-b"}},
	}, nil).Once()
	cfg, err := config.LoadGenConfig("../../examples/gen-config-secrets.yaml")
	require.NoError(t, err)
	cfg.Resources[0].Type = provider.Type()
	dir := t.TempDir()
	cfg.Outputs[0].OutputFile = filepath.Join(dir, "mysql.yaml")
	cfg.Outputs[0].Template, err = filepath.Abs("../../examples/templates/mysql-secrets.yaml.tmpl")
	require.NoError(t, err)
	path := writeRunConfig(t, dir, *cfg)
	require.NoError(t, app.run(context.Background(), path))
	content, err := os.ReadFile(cfg.Outputs[0].OutputFile)
	require.NoError(t, err)
	var parsed struct {
		Instances []struct {
			Host, Username, Password string
			DBM                      bool `yaml:"dbm"`
		}
	}
	require.NoError(t, yaml.Unmarshal(content, &parsed))
	require.Len(t, parsed.Instances, 2)
	for i, team := range []string{"team-a", "team-b"} {
		instance := parsed.Instances[i]
		assert.Equal(t, []string{"a.example", "b.example"}[i], instance.Host)
		assert.True(t, instance.DBM)
		assert.Equal(t, "ENC[production/"+team+"/mysql/monitoring;username]", instance.Username)
		assert.Equal(t, "ENC[production/"+team+"/mysql/monitoring;password]", instance.Password)
	}
}
