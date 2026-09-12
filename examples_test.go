package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/config"
	"github.com/moepig/dd-conf-gen/providers"
	"github.com/moepig/dd-conf-gen/providers/aurora"
	"github.com/moepig/dd-conf-gen/providers/elasticache"
	"github.com/moepig/dd-conf-gen/renderer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Loads every distributed configuration, validates providers without API access, and renders every template with empty and representative resources to verify valid YAML, role selection, and quoting.
func TestDistributedExamples(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("examples/gen-config*.yaml")
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
								metadata := map[string]interface{}{"ClusterName": "cluster", "DBInstanceID": "instance", "IsWriter": primary, "IsPrimary": primary, "RoleKnown": true}
								if scenario == "unknown" {
									delete(metadata, "IsPrimary")
									metadata["RoleKnown"] = false
								}
								resources = append(resources, providers.Resource{Host: "node.example", Port: 6379, Tags: map[string]string{"awsenv": tagValue, "env": tagValue, "service": "team: ops"}, Metadata: metadata})
							}
						}
						secretValues := map[string]string{}
						for name := range out.Data.Secrets {
							secretValues[name] = "value: # special\n\"quoted\""
						}
						data, err := template.Render(context.Background(), renderer.TemplateData{Resources: resources, Secrets: secretValues})
						require.NoError(t, err)
						var parsed struct {
							Instances []struct {
								Host, Username, Password string
								Port                     int
								Tags                     []string
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
						for _, instance := range parsed.Instances {
							assert.Equal(t, "node.example", instance.Host)
							assert.Equal(t, 6379, instance.Port)
							switch filepath.Base(out.Template) {
							case "redis.yaml.tmpl":
								assert.Contains(t, instance.Tags, "env:"+tagValue)
								assert.Contains(t, instance.Tags, "team:team: ops")
							case "mysql.yaml.tmpl":
								assert.Contains(t, instance.Tags, "env:"+tagValue)
							case "mysql-secrets.yaml.tmpl":
								assert.Equal(t, secretValues["username"], instance.Username)
								assert.Equal(t, secretValues["password"], instance.Password)
							}
						}
					})
				}
			}
		})
	}
}
