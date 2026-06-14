package eval

import (
	"fmt"
	"sort"
)

// validateArgs checks args against the subset of JSON Schema the corpus uses:
// `required` field presence and declared scalar/array/object `type`s. It returns
// a sorted list of human-readable problems (empty when args satisfy the schema).
// Unknown extra fields are allowed (additionalProperties defaults to true), so a
// removed/renamed field is detected by comparing two schemas, not by this check.
func validateArgs(schema, args map[string]any) []string {
	if schema == nil {
		return nil
	}
	var problems []string

	for _, req := range schemaRequired(schema) {
		if _, ok := args[req]; !ok {
			problems = append(problems, fmt.Sprintf("missing required field %q", req))
		}
	}

	props := schemaProperties(schema)
	for name, raw := range args {
		spec, ok := props[name].(map[string]any)
		if !ok {
			continue // not declared; additionalProperties allows it
		}
		declared, _ := spec["type"].(string)
		if declared != "" && !jsonTypeMatches(declared, raw) {
			problems = append(problems, fmt.Sprintf("field %q must be %s", name, declared))
		}
	}

	sort.Strings(problems)
	return problems
}

// schemaRequired returns the sorted `required` field names of a JSON-schema
// object, tolerating the []any shape JSON decoding produces.
func schemaRequired(schema map[string]any) []string {
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// schemaProperties returns the `properties` object, or nil.
func schemaProperties(schema map[string]any) map[string]any {
	props, _ := schema["properties"].(map[string]any)
	return props
}

// jsonTypeMatches reports whether val (as produced by encoding/json) conforms to
// a declared JSON Schema scalar/compound type. Integers arrive as float64.
func jsonTypeMatches(declared string, val any) bool {
	switch declared {
	case "string":
		_, ok := val.(string)
		return ok
	case "integer", "number":
		switch val.(type) {
		case float64, int, int64:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "array":
		_, ok := val.([]any)
		return ok
	case "object":
		_, ok := val.(map[string]any)
		return ok
	default:
		return true // unknown/unconstrained type: accept
	}
}
