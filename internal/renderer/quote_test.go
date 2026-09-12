package renderer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/moepig/dd-conf-gen/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Renders strings through quote and decodes the YAML, requiring exact string values without additional fields or scalar type conversion.
func TestQuoteRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "quote.tmpl")
	require.NoError(t, os.WriteFile(path, []byte("value: {{ (index .Resources 0).Host | quote }}\n"), 0600))
	compiled, err := Compile(context.Background(), path)
	require.NoError(t, err)
	for name, value := range map[string]string{
		"empty":                  "",
		"boolean":                "true",
		"null":                   "null",
		"number":                 "00123",
		"secret reference":       "ENC[production/team/redis;password]",
		"quotes and backslashes": "\"quoted\" \\ path",
		"newlines":               "first\nsecond\r\nthird\r",
		"control characters":     "\x00\x01\t\x1b\x7f",
		"unicode":                "日本語 😀\u0085\u2028\u2029",
		"YAML structure":         "value: # comment\nother: [injected]\n---\n- item",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := compiled.Render(context.Background(), TemplateData{Resources: []providers.Resource{{Host: value}}})
			require.NoError(t, err)
			var decoded map[string]interface{}
			require.NoError(t, yaml.Unmarshal(data, &decoded))
			assert.Equal(t, map[string]interface{}{"value": value}, decoded)
		})
	}
}
