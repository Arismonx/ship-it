// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package schema

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

// For returns the JSON Schema (2020-12) of the JSON encoding of v's type: the
// single source for MCP outputSchema and --json outputs (P3). Fields
// follow encoding/json: json tags, omitempty fields are optional, "-" is
// skipped. A "doc" tag becomes the description.
func For(v any) map[string]any {
	return typeSchema(reflect.TypeOf(v), map[reflect.Type]bool{})
}

var (
	timeType    = reflect.TypeOf(time.Time{})
	rawJSONType = reflect.TypeOf(json.RawMessage{})
)

func typeSchema(t reflect.Type, visiting map[reflect.Type]bool) map[string]any {
	if t.Kind() == reflect.Pointer {
		// A nil pointer encodes as null.
		return map[string]any{"anyOf": []any{typeSchema(t.Elem(), visiting), map[string]any{"type": "null"}}}
	}
	if t == timeType {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t == rawJSONType {
		return map[string]any{} // any JSON value: the empty schema accepts all
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		// A nil slice encodes as null.
		return map[string]any{"type": []string{"array", "null"}, "items": typeSchema(t.Elem(), visiting)}
	case reflect.Map:
		// A nil map encodes as null.
		return map[string]any{"type": []string{"object", "null"}, "additionalProperties": typeSchema(t.Elem(), visiting)}
	case reflect.Struct:
		if visiting[t] {
			return map[string]any{"type": "object"} // recursive type: no deeper detail
		}
		visiting[t] = true
		defer delete(visiting, t)
		properties := map[string]any{}
		var required []string
		addFields(t, properties, &required, visiting)
		s := map[string]any{"type": "object", "properties": properties}
		if len(required) > 0 {
			s["required"] = required
		}
		return s
	case reflect.Interface:
		return map[string]any{}
	}
	panic("schema: unsupported kind " + t.Kind().String())
}

func addFields(t reflect.Type, properties map[string]any, required *[]string, visiting map[reflect.Type]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			addFields(f.Type, properties, required, visiting) // embedded: fields promoted
			continue
		}
		if name == "" {
			name = f.Name
		}
		p := typeSchema(f.Type, visiting)
		if doc := f.Tag.Get("doc"); doc != "" {
			p["description"] = doc
		}
		properties[name] = p
		if !strings.Contains(opts, "omitempty") && f.Type.Kind() != reflect.Pointer {
			*required = append(*required, name)
		}
	}
}
