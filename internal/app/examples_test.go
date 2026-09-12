package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moepig/dd-conf-gen/internal/config"
	"github.com/moepig/dd-conf-gen/internal/providers"
	"github.com/moepig/dd-conf-gen/internal/providers/aurora"
	"github.com/moepig/dd-conf-gen/internal/providers/elasticache"
	"github.com/moepig/dd-conf-gen/internal/renderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Loads every distributed configuration, validates providers without API access, and renders empty and representative resources to verify YAML, role selection, DBM settings, and unresolved secret references.
func TestDistributedExamples(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("../../examples/gen-config*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	registry := providers.NewRegistry(map[string]providers.Factory{
		"elasticache_redis": func() providers.Provider { return elasticache.NewProvider() },
		"aurora_mysql":      func() providers.Provider { return aurora.NewProvider() },
	})
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			cfg, err := config.LoadGenConfig(path)
			require.NoError(t, err)
			app := application{registry: registry}
			_, err = app.prepareResources(cfg.Resources)
			require.NoError(t, err)
			for _, out := range cfg.Outputs {
				template, err := renderer.Compile(context.Background(), filepath.Join(filepath.Dir(path), out.Template))
				require.NoError(t, err)
				for _, scenario := range []string{"empty", "roles", "unknown"} {
					t.Run(filepath.Base(out.Template)+"/"+scenario, func(t *testing.T) {
						var resources []providers.Resource
						tagValue := "Production: # special\n\"quoted\""
						if scenario != "empty" {
							for _, primary := range []bool{true, false} {
								metadata := map[string]interface{}{"ClusterName": "cluster", "CacheClusterID": "cache-node", "DBInstanceID": "instance", "IsWriter": primary, "IsPrimary": primary, "RoleKnown": true}
								if scenario == "unknown" {
									delete(metadata, "IsPrimary")
									metadata["RoleKnown"] = false
								}
								team := "team-a"
								if !primary {
									team = "team-b"
								}
								resources = append(resources, providers.Resource{Host: "node.example", Port: 6379, Tags: map[string]string{"awsenv": tagValue, "env": tagValue, "service": "team: ops", "team": team}, Metadata: metadata})
							}
						}
						data, err := template.Render(context.Background(), renderer.TemplateData{Resources: resources})
						require.NoError(t, err)
						var parsed struct {
							Instances []struct {
								Host, Username, Password string
								Port                     int
								Tags                     []string
								DBM                      bool
								AWS                      struct {
									InstanceEndpoint string `yaml:"instance_endpoint"`
									Region           string
								}
							}
						}
						require.NoError(t, yaml.Unmarshal(data, &parsed), string(data))
						require.NotNil(t, parsed.Instances, string(data))
						expected := len(resources)
						switch filepath.Base(out.Template) {
						case "redis-primary.yaml.tmpl":
							if scenario == "roles" {
								expected = 1
							} else {
								expected = 0
							}
						case "mysql-writer.yaml.tmpl":
							if scenario != "empty" {
								expected = 1
							}
						}
						require.Len(t, parsed.Instances, expected)
						for i, instance := range parsed.Instances {
							assert.Equal(t, "node.example", instance.Host)
							assert.Equal(t, 6379, instance.Port)
							engine := "redis"
							if strings.HasPrefix(filepath.Base(out.Template), "mysql") {
								engine = "mysql"
								assert.True(t, instance.DBM)
								assert.Equal(t, instance.Host, instance.AWS.InstanceEndpoint)
								assert.Equal(t, "ap-northeast-1", instance.AWS.Region)
							}
							secretID := "monitoring/" + engine
							if strings.HasSuffix(out.Template, "-secrets.yaml.tmpl") {
								teams := []string{"team-a", "team-b"}
								secretID = "production/" + teams[i] + "/" + engine + "/monitoring"
							}
							assert.Equal(t, "ENC["+secretID+";username]", instance.Username)
							assert.Equal(t, "ENC["+secretID+";password]", instance.Password)
							switch filepath.Base(out.Template) {
							case "redis.yaml.tmpl":
								assert.Contains(t, instance.Tags, "env:"+tagValue)
								assert.Contains(t, instance.Tags, "team:team: ops")
							case "mysql.yaml.tmpl":
								assert.Contains(t, instance.Tags, "env:"+tagValue)
							case "redis-secrets.yaml.tmpl":
								assert.Contains(t, instance.Tags, "cacheclusterid:cache-node")
							}
						}
					})
				}
			}
		})
	}
}
