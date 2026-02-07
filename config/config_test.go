package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGenConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := `version: "1.0"
resources:
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
		assert.Equal(t, "1.0", cfg.Version)
		assert.Len(t, cfg.Resources, 1)
		assert.Len(t, cfg.Outputs, 1)
		assert.Equal(t, "production_redis", cfg.Resources[0].Name)
		assert.Equal(t, "elasticache_redis", cfg.Resources[0].Type)
		assert.Equal(t, "ap-northeast-1", cfg.Resources[0].Region)
		assert.Equal(t, "production_redis", cfg.Outputs[0].Data.ResourceName)
	})

	t.Run("missing version", func(t *testing.T) {
		content := `resources:
  - name: test
    type: test_type
    region: us-east-1
outputs:
  - template: test.tmpl
    output_file: /tmp/test.yaml
    data:
      resource_name: test
`
		tmpfile := createTempFile(t, content)
		defer os.Remove(tmpfile)

		_, err := LoadGenConfig(tmpfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version is required")
	})

	t.Run("missing resources", func(t *testing.T) {
		content := `version: "1.0"
outputs:
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
		content := `version: "1.0"
resources:
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
		content := `version: "1.0"
resources:
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
		content := `version: "1.0"
resources:
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
				content: `version: "1.0"
resources:
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
				content: `version: "1.0"
resources:
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
				content: `version: "1.0"
resources:
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
				content: `version: "1.0"
resources:
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
				content: `version: "1.0"
resources:
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
				content: `version: "1.0"
resources:
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
			Version: "1.0",
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
		content := `version: "1.0"
resources:
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
