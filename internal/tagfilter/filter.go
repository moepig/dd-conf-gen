package tagfilter

import (
	"fmt"
	"slices"
)

// Holds a tag key, comparison operator, and independently owned candidate values.
type Condition struct {
	Key      string
	Operator string
	Values   []string
}

// Holds tag conditions combined with AND semantics.
type Conditions []Condition

// Reports whether all conditions match tags. not_in also matches missing keys; exists and not_exists distinguish missing keys from empty values.
func (conditions Conditions) Matches(tags map[string]string) bool {
	for _, c := range conditions {
		value, exists := tags[c.Key]
		switch c.Operator {
		case "in":
			if !exists || !slices.Contains(c.Values, value) {
				return false
			}
		case "not_in":
			if exists && slices.Contains(c.Values, value) {
				return false
			}
		case "exists":
			if !exists {
				return false
			}
		case "not_exists":
			if exists {
				return false
			}
		}
	}
	return true
}

// Parses exact tags and conditions from raw filters, returning independent values or an error for unsupported fields or invalid types and operators.
func Parse(filters map[string]interface{}) (map[string]string, Conditions, error) {
	tags := make(map[string]string)
	var conditions Conditions
	for key, value := range filters {
		switch key {
		case "tags":
			raw, ok := value.(map[string]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("filters.tags must be a map")
			}
			for key, value := range raw {
				s, ok := value.(string)
				if !ok {
					return nil, nil, fmt.Errorf("filters.tags.%s must be a string", key)
				}
				tags[key] = s
			}
		case "tag_conditions":
			raw, ok := value.([]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("filters.tag_conditions must be a list")
			}
			for i, value := range raw {
				c, err := parseCondition(value)
				if err != nil {
					return nil, nil, fmt.Errorf("filters.tag_conditions[%d]: %w", i, err)
				}
				conditions = append(conditions, c)
			}
		default:
			return nil, nil, fmt.Errorf("unsupported filter: %s", key)
		}
	}
	return tags, conditions, nil
}

// Parses a condition map and returns a typed copy or an error for malformed input.
func parseCondition(value interface{}) (Condition, error) {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return Condition{}, fmt.Errorf("condition must be a map")
	}
	var c Condition
	for key := range raw {
		if key != "key" && key != "operator" && key != "values" {
			return c, fmt.Errorf("unsupported condition field: %s", key)
		}
	}
	c.Key, ok = raw["key"].(string)
	if !ok || c.Key == "" {
		return c, fmt.Errorf("key must be a nonempty string")
	}
	c.Operator, ok = raw["operator"].(string)
	if !ok {
		return c, fmt.Errorf("operator must be a string")
	}
	switch c.Operator {
	case "exists", "not_exists":
		if _, exists := raw["values"]; exists {
			return c, fmt.Errorf("values must be omitted for %s", c.Operator)
		}
	case "in", "not_in":
		values, ok := raw["values"].([]interface{})
		if !ok || len(values) == 0 {
			return c, fmt.Errorf("values must be a nonempty string list")
		}
		for _, v := range values {
			s, ok := v.(string)
			if !ok {
				return c, fmt.Errorf("values must contain only strings")
			}
			c.Values = append(c.Values, s)
		}
	default:
		return c, fmt.Errorf("unsupported operator: %s", c.Operator)
	}
	return c, nil
}
