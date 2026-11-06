// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// UserSchemaURL is the JSON Schema of the user configuration file.
const UserSchemaURL = "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/user.schema.json"

// Schema returns the JSON Schema (draft 2020-12) of .github/ship.json (and of
// the presets it extends), derived from Settings and its tags like Merge, so
// that the two never disagree. tools/gen writes it to schemas/ship.schema.json.
func Schema() []byte {
	properties := documentProperties()
	delete(properties, personalSection) // the user file's only
	properties["overrides"] = map[string]any{
		"type":        "array",
		"description": "Settings for a part of the repository, in order (a later match wins). An override applies to a delivery when every one of its files matches one of the patterns.",
		"items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"paths"},
			"properties": map[string]any{
				"paths": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1},
					"description": `Glob patterns relative to the repository root: "docs/**", "*.md" (a name matches at any depth), "apps/*/src/**".`},
				"ship": settingsSchema()["properties"].(map[string]any)["ship"],
			},
		},
	}
	return marshalSchema(SchemaURL, ".github/ship.json",
		"Settings of ship-it for this repository. Every key is optional: a missing key keeps its default. Comments and trailing commas are allowed (JSONC).", properties)
}

// UserSchema returns the JSON Schema of the user configuration file, which
// may hold contexts: settings for the repositories of an owner, a host or a
// directory (an agency's clients). tools/gen writes schemas/user.schema.json.
func UserSchema() []byte {
	properties := documentProperties()
	settings := settingsSchema()["properties"].(map[string]any)
	properties["contexts"] = map[string]any{
		"type":        "array",
		"description": "Settings that apply to some repositories only, in order (a later match wins). Every criterion given in match must hold.",
		"items": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"match"},
			"properties": map[string]any{
				"match": map[string]any{
					"type": "object", "additionalProperties": false, "minProperties": 1,
					"properties": map[string]any{
						"owner": map[string]any{"type": "string", "minLength": 1, "description": "GitHub owner of origin (user or organisation), without regard to case."},
						"host":  map[string]any{"type": "string", "minLength": 1, "description": "Host of origin: github.com, or a GitHub Enterprise server."},
						"path":  map[string]any{"type": "string", "minLength": 1, "description": `Glob pattern of the repository root; "~/" is the home directory: "~/clients/acme/**".`},
					},
				},
				"ship":   settings["ship"],
				"github": settings["github"],
				"ui":     settings["ui"],
			},
		},
	}
	return marshalSchema(UserSchemaURL, "ship-it user configuration",
		"Settings of ship-it for every repository of this user. Comments and trailing commas are allowed (JSONC).", properties)
}

// documentProperties are the keys every configuration file shares.
func documentProperties() map[string]any {
	properties := map[string]any{
		"$schema": map[string]any{"type": "string", "description": "JSON Schema of this file, for editor completion."},
		"version": map[string]any{"type": "integer", "enum": []int{CurrentVersion}, "default": CurrentVersion,
			"description": "Version of the file format."},
		"extends": map[string]any{
			"type": "array", "uniqueItems": true,
			"description": "Presets applied before this file, in order: built-in (ship-it:recommended), a .json file relative to this file's root, or a file of a GitHub repository pinned to a full commit SHA.",
			"items": map[string]any{"type": "string", "anyOf": []any{
				map[string]any{"pattern": `^ship-it:[a-z][a-z0-9-]*$`},
				map[string]any{"pattern": `^github:[A-Za-z0-9-]+/[A-Za-z0-9._-]+/.+\.jsonc?@[0-9a-f]{40}$`},
				map[string]any{"pattern": `^(\./)?[^/\\:][^\\:]*\.jsonc?$`},
			}},
		},
	}
	for name, property := range settingsSchema()["properties"].(map[string]any) {
		properties[name] = property
	}
	return properties
}

func settingsSchema() map[string]any {
	return objectSchema(reflect.TypeOf(Settings{}), reflect.ValueOf(Defaults()))
}

func marshalSchema(id, title, description string, properties map[string]any) []byte {
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  id,
		"title":                title,
		"description":          description,
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err) // only maps, strings and numbers: cannot fail
	}
	return append(data, '\n')
}

func objectSchema(t reflect.Type, defaults reflect.Value) map[string]any {
	properties := map[string]any{}
	for i := range t.NumField() {
		field := t.Field(i)
		name := jsonName(field)
		if name == "" {
			continue
		}
		var property map[string]any
		switch field.Type.Kind() {
		case reflect.Struct:
			property = objectSchema(field.Type, defaults.Field(i))
		case reflect.Bool:
			property = map[string]any{"type": "boolean", "default": defaults.Field(i).Bool()}
		case reflect.String:
			property = map[string]any{"type": "string", "default": defaults.Field(i).String()}
			if allowed := enumOf(field); len(allowed) > 0 {
				property["enum"] = allowed
			}
			if pattern := field.Tag.Get("pattern"); pattern != "" {
				property["pattern"] = pattern
			}
		case reflect.Int:
			property = map[string]any{"type": "integer", "default": defaults.Field(i).Int()}
			if minimum := field.Tag.Get("minimum"); minimum != "" {
				low, _ := strconv.Atoi(minimum)
				property["minimum"] = low
			}
		case reflect.Slice:
			items := map[string]any{"type": "string"}
			if field.Type.Elem().Kind() == reflect.Struct {
				items = itemSchema(field.Type.Elem())
			}
			if pattern := field.Tag.Get("itemPattern"); pattern != "" {
				items["pattern"] = pattern
			}
			if allowed := itemEnumOf(field); len(allowed) > 0 {
				items["enum"] = allowed
			}
			property = map[string]any{"type": "array", "items": items, "uniqueItems": true,
				"default": defaults.Field(i).Interface()}
		case reflect.Map:
			property = reflect.Zero(field.Type).Interface().(mapSetting).jsonSchema()
			property["default"] = defaults.Field(i).Interface()
		default:
			panic("config: unsupported field kind " + field.Type.Kind().String())
		}
		if doc := strings.TrimSpace(field.Tag.Get("doc")); doc != "" {
			property["description"] = doc
		}
		properties[name] = property
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
}

// itemSchema is the schema of an object in a list: no defaults (an item has
// none), and every field tagged required:"true" is required.
func itemSchema(t reflect.Type) map[string]any {
	schema := objectSchema(t, reflect.New(t).Elem())
	for _, property := range schema["properties"].(map[string]any) {
		delete(property.(map[string]any), "default")
	}
	required := []string{}
	for i := range t.NumField() {
		if t.Field(i).Tag.Get("required") == "true" {
			required = append(required, jsonName(t.Field(i)))
		}
	}
	slices.Sort(required)
	schema["required"] = required
	return schema
}
