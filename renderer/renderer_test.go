package renderer

import (
	"context"
	"os"
	"testing"
	"text/template"
	"time"

	"github.com/moepig/dd-conf-gen/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cancels before and during template execution and requires the context error with no partial output.
func TestTemplateCancellation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"prefix{{cancel}}suffix", "{{$_ := cancel}}"} {
		t.Run(source, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tmpl := template.Must(template.New("test").Funcs(template.FuncMap{"cancel": func() string { cancel(); return "" }}).Parse(source))
			compiled := &CompiledTemplate{template: tmpl}
			data, err := compiled.RenderContext(ctx, TemplateData{})
			require.ErrorIs(t, err, context.Canceled)
			assert.Nil(t, data)
		})
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	compiled := &CompiledTemplate{template: template.Must(template.New("test").Parse("output"))}
	data, err := compiled.RenderContext(ctx, TemplateData{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, data)
	result, err := NewRenderer().CompileContext(ctx, "missing-template")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, result)
}

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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{
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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{
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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{
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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{
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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{
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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{
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
		renderer := NewRenderer()
		data := TemplateData{}

		_, err := renderer.Render("/nonexistent/template.yaml", data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read template file")
	})

	t.Run("invalid template syntax", func(t *testing.T) {
		templateContent := `{{ .InvalidSyntax {{ }}`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer()
		data := TemplateData{}

		_, err := renderer.Render(tmpfile, data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse template")
	})

	t.Run("template execution error", func(t *testing.T) {
		templateContent := `{{ .NonexistentField.SubField }}`
		tmpfile := createTempFile(t, templateContent)
		defer os.Remove(tmpfile)

		renderer := NewRenderer()
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

		renderer := NewRenderer()
		data := TemplateData{
			Resources: []providers.Resource{},
		}

		result, err := renderer.Render(tmpfile, data)
		require.NoError(t, err)

		expected := `init_config:

instances:
`
		assert.Equal(t, expected, string(result))
	})
}

// Missing map keys accessed with dot notation must fail without returning partially rendered configuration.
func TestRendererMissingMapKey(t *testing.T) {
	for _, field := range []string{"Tags.missing", "Metadata.ClustrName"} {
		t.Run(field, func(t *testing.T) {
			path := createTempFile(t, "prefix{{range .Resources}}{{."+field+"}}{{end}}")
			t.Cleanup(func() { os.Remove(path) })
			result, err := NewRenderer().Render(path, TemplateData{Resources: []providers.Resource{{Tags: map[string]string{}, Metadata: map[string]interface{}{"ClusterName": "redis"}}}})
			require.ErrorContains(t, err, "map has no entry for key")
			assert.Nil(t, result)
		})
	}
}

// Optional tag lookup through index must remain usable in conditional template branches.
func TestRendererOptionalTag(t *testing.T) {
	path := createTempFile(t, `{{range .Resources}}{{if index .Tags "optional"}}present{{else}}absent{{end}}{{end}}`)
	t.Cleanup(func() { os.Remove(path) })
	result, err := NewRenderer().Render(path, TemplateData{Resources: []providers.Resource{{Tags: map[string]string{}}}})
	require.NoError(t, err)
	assert.Equal(t, "absent", string(result))
}

// Compiled templates must use the validated source even if the template file changes before rendering.
func TestCompiledTemplateUsesValidatedSource(t *testing.T) {
	path := createTempFile(t, "validated")
	t.Cleanup(func() { os.Remove(path) })
	compiled, err := NewRenderer().CompileContext(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("{{"), 0600))
	content, err := compiled.RenderContext(context.Background(), TemplateData{})
	require.NoError(t, err)
	assert.Equal(t, "validated", string(content))
}

func createTempFile(t *testing.T, content string) string {
	tmpfile, err := os.CreateTemp("", "template-*.yaml")
	require.NoError(t, err)
	_, err = tmpfile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpfile.Close())
	return tmpfile.Name()
}
