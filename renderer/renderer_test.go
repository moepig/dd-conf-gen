package renderer

import (
	"os"
	"testing"

	"github.com/moepig/dd-conf-gen/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderer_Render(t *testing.T) {
	t.Run("simple template", func(t *testing.T) {
		templateContent := `init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{
				{
					Host: "example.com",
					Port: 6379,
				},
			},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		expected := `init_config:

instances:
  - host: example.com
    port: 6379
`
		assert.Equal(t, expected, string(result))
	})

	t.Run("template with hardcoded values", func(t *testing.T) {
		templateContent := `init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{
				{
					Host: "redis1.example.com",
					Port: 6379,
				},
			},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		expected := `init_config:

instances:
  - host: redis1.example.com
    port: 6379
    username: "%%env_REDIS_USERNAME%%"
    password: "%%env_REDIS_PASSWORD%%"
`
		assert.Equal(t, expected, string(result))
	})

	t.Run("template with tags", func(t *testing.T) {
		templateContent := `instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    tags:
    {{- range $key, $value := .Tags }}
      - {{ $key }}:{{ $value }}
    {{- end }}
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{
				{
					Host: "redis1.example.com",
					Port: 6379,
					Tags: map[string]string{
						"env":  "production",
						"team": "backend",
					},
				},
			},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		// Note: map iteration order is not guaranteed, so we check both possible orders
		resultStr := string(result)
		assert.Contains(t, resultStr, "- host: redis1.example.com")
		assert.Contains(t, resultStr, "port: 6379")
		assert.Contains(t, resultStr, "env:production")
		assert.Contains(t, resultStr, "team:backend")
	})

	t.Run("template with hardcoded tags and dynamic tags", func(t *testing.T) {
		templateContent := `instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    tags:
      - "instancetag:bar"
      - "custom:tag"
    {{- range $key, $value := .Tags }}
      - {{ $key }}:{{ $value }}
    {{- end }}
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{
				{
					Host: "redis1.example.com",
					Port: 6379,
					Tags: map[string]string{
						"env": "production",
					},
				},
			},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		resultStr := string(result)
		assert.Contains(t, resultStr, "- host: redis1.example.com")
		assert.Contains(t, resultStr, "instancetag:bar")
		assert.Contains(t, resultStr, "custom:tag")
		assert.Contains(t, resultStr, "env:production")
	})

	t.Run("multiple resources", func(t *testing.T) {
		templateContent := `instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{
				{Host: "redis1.example.com", Port: 6379},
				{Host: "redis2.example.com", Port: 6379},
				{Host: "redis3.example.com", Port: 6379},
			},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		resultStr := string(result)
		assert.Contains(t, resultStr, "redis1.example.com")
		assert.Contains(t, resultStr, "redis2.example.com")
		assert.Contains(t, resultStr, "redis3.example.com")
	})

	t.Run("template with metadata", func(t *testing.T) {
		templateContent := `instances:
{{- range .Resources }}
  - host: {{ .Host }}
    port: {{ .Port }}
    cluster_name: {{ index .Metadata "ClusterName" }}
    is_primary: {{ index .Metadata "IsPrimary" }}
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{
				{
					Host: "redis1.example.com",
					Port: 6379,
					Metadata: map[string]interface{}{
						"ClusterName": "my-cluster",
						"IsPrimary":   true,
					},
				},
			},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		expected := `instances:
  - host: redis1.example.com
    port: 6379
    cluster_name: my-cluster
    is_primary: true
`
		assert.Equal(t, expected, string(result))
	})

	t.Run("file not found", func(t *testing.T) {
		renderer := NewRenderer("")
		data := TemplateData{}

		_, err := renderer.Render("/nonexistent/template.yaml", data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read template file")
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		templateContent := `{{ .InvalidSyntax {{ }}`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{}

		_, err := renderer.Render(tmpfile, data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse template")
	})

	t.Run("template execution error", func(t *testing.T) {
		templateContent := `{{ .NonexistentField.SubField }}`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{}

		_, err := renderer.Render(tmpfile, data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to execute template")
	})

	t.Run("empty resources", func(t *testing.T) {
		templateContent := `init_config:

instances:
{{- range .Resources }}
  - host: {{ .Host }}
{{- end }}
`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer("")
		data := TemplateData{
			Resources: []resources.Resource{},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		expected := `init_config:

instances:
`
		assert.Equal(t, expected, string(result))
	})
}

func createTempFile(t *testing.T, content string) string {
	tmpfile, err := os.CreateTemp("", "template-*.yaml")
	require.NoError(t, err)
	_, err = tmpfile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())
	return tmpfile.Name()
}
