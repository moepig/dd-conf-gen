package tagfilter

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// Evaluates candidate, exclusion, and existence conditions against absent, empty, matching, and other tags.
func TestMatches(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		op                         string
		absent, empty, prod, other bool
	}{
		{"in", false, false, true, false}, {"not_in", true, true, false, true},
		{"exists", false, true, true, true}, {"not_exists", true, false, false, false},
	} {
		t.Run(tc.op, func(t *testing.T) {
			c := Conditions{{Key: "env", Operator: tc.op, Values: []string{"prod", "staging"}}}
			for i, tags := range []map[string]string{nil, {"env": ""}, {"env": "prod"}, {"env": "other"}} {
				assert.Equal(t, []bool{tc.absent, tc.empty, tc.prod, tc.other}[i], c.Matches(tags))
			}
		})
	}
	c := Conditions{{Key: "env", Operator: "in", Values: []string{"prod", "staging"}}, {Key: "disabled", Operator: "not_exists"}}
	assert.True(t, c.Matches(map[string]string{"env": "staging"}))
	assert.False(t, c.Matches(map[string]string{"env": "prod", "disabled": ""}))
}

// Rejects malformed conditions during parsing and verifies that successful parsing owns nested candidate slices.
func TestParseConditions(t *testing.T) {
	t.Parallel()
	for _, value := range []interface{}{
		"bad", nil, []interface{}{"bad"},
		[]interface{}{map[string]interface{}{"key": "env", "operator": "invalid"}},
		[]interface{}{map[string]interface{}{"key": "", "operator": "exists"}},
		[]interface{}{map[string]interface{}{"key": "env", "operator": "exists", "values": nil}},
		[]interface{}{map[string]interface{}{"key": "env", "operator": "in", "values": []interface{}{}}},
		[]interface{}{map[string]interface{}{"key": "env", "operator": "in", "values": []interface{}{1}}},
		[]interface{}{map[string]interface{}{"key": "env", "operator": "exists", "unknown": true}},
	} {
		_, _, err := Parse(map[string]interface{}{"tag_conditions": value})
		require.Error(t, err)
	}
	values := []interface{}{"prod", "staging"}
	raw := map[string]interface{}{"key": "env", "operator": "in", "values": values}
	_, conditions, err := Parse(map[string]interface{}{"tag_conditions": []interface{}{raw}})
	require.NoError(t, err)
	values[0] = "changed"
	raw["key"] = "changed"
	assert.True(t, conditions.Matches(map[string]string{"env": "prod"}))
	assert.False(t, conditions.Matches(map[string]string{"env": "changed"}))
}
