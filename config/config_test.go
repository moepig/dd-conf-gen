package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Loads misspelled fields and trailing documents from files and requires rejection without a partial configuration.
func TestLoadGenConfigStrictYAML(t *testing.T) {
	t.Parallel()
	valid := "resources:\n  - name: redis\n    type: elasticache_redis\n    region: us-east-1\n    filters:\n      tags:\n        env: prod\noutputs:\n  - template: redis.tmpl\n    output_file: out.yaml\n    data:\n      resource_name: redis\n"
	for name, content := range map[string]string{
		"top level":                 valid + "unknown: true\n",
		"resource":                  strings.Replace(valid, "filters:", "filter:", 1),
		"output":                    strings.Replace(valid, "    data:", "    unknown: true\n    data:", 1),
		"output data":               valid + "      unknown: true\n",
		"second document":           valid + "---\nresources: []\noutputs: []\n",
		"empty second document":     valid + "---\n",
		"malformed second document": valid + "---\n[",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, err := LoadGenConfig(createTempFile(t, content))
			require.Error(t, err)
			assert.Nil(t, cfg)
		})
	}
	cfg, err := LoadGenConfig(createTempFile(t, valid+"...\n# trailing comment\n"))
	require.NoError(t, err)
	assert.Len(t, cfg.Resources, 1)
}

func TestLoadGenConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := `resources:
  - name: production_redis
    type: elasticache_redis
    region: ap-northeast-1
    filters:
      tags:
        env: Production

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /tmp/redisdb.yaml
    data:
      resource_name: production_redis
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		cfg, err := LoadGenConfig(tmpfile)
		require.NoError(t, err)
		assert.Len(t, cfg.Resources, 1)
		assert.Len(t, cfg.Outputs, 1)
		assert.Equal(t, "production_redis", cfg.Resources[0].Name)
		assert.Equal(t, "elasticache_redis", cfg.Resources[0].Type)
		assert.Equal(t, "ap-northeast-1", cfg.Resources[0].Region)
		assert.Equal(t, "production_redis", cfg.Outputs[0].Data.ResourceName)
	})

	t.Run("missing resources", func(t *testing.T) {
		content := `outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: test
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		_, err := LoadGenConfig(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one resource must be defined")
	})

	t.Run("missing outputs", func(t *testing.T) {
		content := `resources:
  - name: test
    type: test_type
    region: us-east-1
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		_, err := LoadGenConfig(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one output must be defined")
	})

	t.Run("duplicate resource name", func(t *testing.T) {
		content := `resources:
  - name: duplicate
    type: type1
    region: us-east-1
  - name: duplicate
    type: type2
    region: us-west-2
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: duplicate
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		_, err := LoadGenConfig(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate resource name")
	})

	t.Run("invalid resource reference", func(t *testing.T) {
		content := `resources:
  - name: existing_resource
    type: test_type
    region: us-east-1
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: nonexistent_resource
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		_, err := LoadGenConfig(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "resource_name 'nonexistent_resource' not found")
	})

	t.Run("missing required fields", func(t *testing.T) {
		testCases := []struct {
			name        string
			content     string
			expectedErr string
		}{
			{
				name: "missing resource name",
				content: `resources:
  - type: test_type
    region: us-east-1
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: test
`,
				expectedErr: "name is required",
			},
			{
				name: "missing resource type",
				content: `resources:
  - name: test
    region: us-east-1
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: test
`,
				expectedErr: "type is required",
			},
			{
				name: "missing resource region",
				content: `resources:
  - name: test
    type: test_type
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: test
`,
				expectedErr: "region is required",
			},
			{
				name: "missing output template",
				content: `resources:
  - name: test
    type: test_type
    region: us-east-1
outputs:
  - output_file: /tmp/test.yaml
    data:
      resource_name: test
`,
				expectedErr: "template is required",
			},
			{
				name: "missing output file",
				content: `resources:
  - name: test
    type: test_type
    region: us-east-1
outputs:
  - template: test.tmpl
    data:
      resource_name: test
`,
				expectedErr: "output_file is required",
			},
			{
				name: "missing output resource_name",
				content: `resources:
  - name: test
    type: test_type
    region: us-east-1
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data: {}
`,
				expectedErr: "resource_name is required",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tmpfile := createTempFile(t, tc.content)
				defer os.Remove(tmpfile)

				_, err := LoadGenConfig(tmpfile)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			})
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadGenConfig("/nonexistent/path/to/config.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read generation config file")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		content := `invalid: yaml: content: [[[`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		_, err := LoadGenConfig(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse generation config")
	})
}

// Duplicate output destinations must be rejected, including relative paths and existing symlink ancestors.
func TestDuplicateOutputPaths(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "alias")
	require.NoError(t, os.Symlink(dir, link))
	abs := filepath.Join(dir, "nested", "out.yaml")
	cwd, err := os.Getwd()
	require.NoError(t, err)
	rel, err := filepath.Rel(cwd, abs)
	require.NoError(t, err)
	for _, second := range []string{abs, dir + "/nested/../nested/out.yaml", rel, filepath.Join(link, "nested", "out.yaml")} {
		t.Run(second, func(t *testing.T) {
			cfg := &GenConfig{
				Resources: []ResourceConfig{{Name: "redis", Type: "elasticache_redis", Region: "us-east-1"}},
				Outputs: []OutputConfig{
					{Template: "redis.tmpl", OutputFile: abs, Data: OutputData{ResourceName: "redis"}},
					{Template: "redis.tmpl", OutputFile: second, Data: OutputData{ResourceName: "redis"}},
				},
			}
			require.ErrorContains(t, validateGenConfig(cfg), "duplicate output_file")
		})
	}
}

func createTempFile(t *testing.T, content string) string {
	tmpfile, err := os.CreateTemp("", "meta-config-*.yaml")
	require.NoError(t, err)
	_, err = tmpfile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())
	return tmpfile.Name()
}

func TestValidateGenConfig(t *testing.T) {
	t.Run("valid config with multiple resources and outputs", func(t *testing.T) {
		cfg := &GenConfig{
			Resources: []ResourceConfig{
				{
					Name:   "redis1",
					Type:   "elasticache_redis",
					Region: "us-east-1",
				},
				{
					Name:   "redis2",
					Type:   "elasticache_redis",
					Region: "us-west-2",
				},
			},
			Outputs: []OutputConfig{
				{
					Template:   "template1.tmpl",
					OutputFile: "/tmp/out1.yaml",
					Data: OutputData{
						ResourceName: "redis1",
					},
				},
				{
					Template:   "template2.tmpl",
					OutputFile: "/tmp/out2.yaml",
					Data: OutputData{
						ResourceName: "redis2",
					},
				},
			},
		}

		err := validateGenConfig(cfg)
		assert.NoError(t, err)
	})

	t.Run("complex filters and tag mapping", func(t *testing.T) {
		content := `resources:
  - name: complex_resource
    type: elasticache_redis
    region: ap-northeast-1
    filters:
      tags:
        Environment: Production
        Team: Backend
      other_filter:
        nested:
          key: value

outputs:
  - template: templates/redis.yaml.tmpl
    output_file: /tmp/redisdb.yaml
    data:
      resource_name: complex_resource
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		cfg, err := LoadGenConfig(tmpfile)
		require.NoError(t, err)
		assert.NotNil(t, cfg.Resources[0].Filters)
	})
}
