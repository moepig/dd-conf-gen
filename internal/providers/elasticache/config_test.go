package elasticache

import (
	"testing"

	"github.com/moepig/dd-conf-gen/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Parses absent, empty, valid, and invalid raw filters; requires preserved region and string tags on success, or an error and zero settings on failure.
func TestParseConfig(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		filters map[string]interface{}
		tags    map[string]string
		err     string
	}{
		{name: "absent filters", tags: map[string]string{}},
		{name: "empty tags", filters: map[string]interface{}{"tags": map[string]interface{}{}}, tags: map[string]string{}},
		{name: "string tags", filters: map[string]interface{}{"tags": map[string]interface{}{"env": "prod", "empty": "", "numeric": "123"}}, tags: map[string]string{"env": "prod", "empty": "", "numeric": "123"}},
		{name: "unknown filter", filters: map[string]interface{}{"tag": map[string]interface{}{"env": "prod"}}, err: "unsupported filter: tag"},
		{name: "non-map tags", filters: map[string]interface{}{"tags": "prod"}, err: "filters.tags must be a map"},
		{name: "numeric tag", filters: map[string]interface{}{"tags": map[string]interface{}{"env": 123}}, err: "filters.tags.env must be a string"},
		{name: "boolean tag", filters: map[string]interface{}{"tags": map[string]interface{}{"env": true}}, err: "filters.tags.env must be a string"},
		{name: "null tag", filters: map[string]interface{}{"tags": map[string]interface{}{"env": nil}}, err: "filters.tags.env must be a string"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			settings, err := parseConfig(providers.ProviderConfig{Region: "us-east-1", Filters: tt.filters})
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				assert.Equal(t, discoveryConfig{}, settings)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "us-east-1", settings.region)
			assert.Equal(t, tt.tags, settings.tags)
		})
	}
}

// Mutates raw and parsed tag maps separately and verifies that each retains its own values.
func TestParseConfigOwnsTags(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{"env": "prod"}
	settings, err := parseConfig(providers.ProviderConfig{Region: "us-east-1", Filters: map[string]interface{}{"tags": raw}})
	require.NoError(t, err)
	raw["env"] = "test"
	assert.Equal(t, "prod", settings.tags["env"])
	settings.tags["env"] = "staging"
	assert.Equal(t, "test", raw["env"])
}
