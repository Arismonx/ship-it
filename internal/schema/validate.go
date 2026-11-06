// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package schema

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Validate checks a decoded JSON value (from encoding/json into any) against
// the subset of JSON Schema that For generates: type, properties, required,
// items, additionalProperties and anyOf. It proves in tests that results
// conform to the outputSchema MCP clients validate against (MCP: "Servers
// MUST provide structured results that conform to this schema").
func Validate(schema map[string]any, value any) error { return validate(schema, value, "$") }

func validate(s map[string]any, v any, at string) error {
	if alternatives, ok := s["anyOf"].([]any); ok {
		for _, a := range alternatives {
			if validate(a.(map[string]any), v, at) == nil {
				return nil
			}
		}
		return fmt.Errorf("%s: matches no alternative", at)
	}
	if types := typesOf(s["type"]); len(types) > 0 && !slices.Contains(types, jsonType(v)) &&
		!(jsonType(v) == "integer" && slices.Contains(types, "number")) {
		return fmt.Errorf("%s: %s, want %s", at, jsonType(v), strings.Join(types, " or "))
	}
	switch value := v.(type) {
	case map[string]any:
		properties, _ := s["properties"].(map[string]any)
		for _, r := range typesOf(s["required"]) {
			if _, ok := value[r]; !ok {
				return fmt.Errorf("%s: missing %q", at, r)
			}
		}
		for k, child := range value {
			sub, known := properties[k].(map[string]any)
			if !known {
				if extra, ok := s["additionalProperties"].(map[string]any); ok {
					sub = extra
				} else if s["additionalProperties"] == false {
					return fmt.Errorf("%s: unexpected %q", at, k)
				} else {
					continue
				}
			}
			if err := validate(sub, child, at+"."+k); err != nil {
				return err
			}
		}
	case []any:
		if items, ok := s["items"].(map[string]any); ok {
			for i, item := range value {
				if err := validate(items, item, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// typesOf reads "type" or "required", whether a string or a list.
func typesOf(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return nil
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}
