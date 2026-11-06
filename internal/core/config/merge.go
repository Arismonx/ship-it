// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// Merge applies layers, weakest first, over the defaults, for no particular
// repository: contexts and overrides do not apply (see MergeFor).
func Merge(layers []Layer) (Effective, error) {
	return MergeFor(layers, Target{})
}

// MergeFor applies layers, weakest first, over the defaults, resolving the
// contexts and overrides that match target. Settings is the single source of
// truth: keys, types and allowed values come from its json, enum and doc
// tags, which also generate the JSON Schema. The organisation policy applies
// last, wherever it is in layers.
func MergeFor(layers []Layer, target Target) (Effective, error) {
	eff, err := mergeLayers(layers, target)
	if err != nil {
		return eff, err
	}
	// What only the merged settings tell: a commit type that two layers
	// define together, for instance.
	return eff, checkCommitTypes(eff.Settings.Ship.Commits.Types, sourcePath(layers, eff.Source("ship.commits.types")))
}

// sourcePath returns the path of the layer a source names ("repository",
// "repository overrides[0]", "policy"), for messages; the source itself when
// no layer has that name.
func sourcePath(layers []Layer, source string) string {
	for _, l := range slices.Backward(layers) {
		if source == l.Name || strings.HasPrefix(source, l.Name+" ") || (source == PolicyLayerName && l.kind() == KindPolicy) {
			return l.Path
		}
	}
	return source
}

// mergeLayers is MergeFor without the checks of the merged result, for
// Explain, which merges each layer alone.
func mergeLayers(layers []Layer, target Target) (Effective, error) {
	eff := Effective{Settings: Defaults(), Sources: map[string]string{}}
	var policy *Layer
	for _, layer := range layers {
		if len(bytes.TrimSpace(layer.Data)) == 0 {
			continue
		}
		if layer.kind() == KindPolicy {
			policy = &layer // applied last, whatever its position
			continue
		}
		raw, err := decode(layer)
		if err != nil {
			return eff, err
		}
		if err := checkVersion(layer, raw); err != nil {
			return eff, err
		}
		if err := applyDocument(&eff, layer, raw, target); err != nil {
			return eff, err
		}
	}
	if policy != nil {
		if err := applyPolicy(&eff, *policy); err != nil {
			return eff, err
		}
	}
	return eff, nil
}

func decode(layer Layer) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(StripJSONC(layer.Data)))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		line, column := 1, 1
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			line, column = position(layer.Data, syntax.Offset)
		}
		return nil, fault.New(fault.ConfigInvalidJSON, "config.invalid_json",
			"file", layer.Path, "line", strconv.Itoa(line), "column", strconv.Itoa(column), "detail", err.Error())
	}
	if dec.More() {
		return nil, fault.New(fault.ConfigInvalidJSON, "config.invalid_json",
			"file", layer.Path, "line", "1", "column", "1", "detail", "more than one JSON value")
	}
	return raw, nil
}

// position converts a byte offset into a 1-based line and column.
func position(data []byte, offset int64) (line, column int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	before := data[:offset]
	line = bytes.Count(before, []byte("\n")) + 1
	column = int(offset) - bytes.LastIndexByte(before, '\n')
	return line, column
}

func checkVersion(layer Layer, raw map[string]any) error {
	value, ok := raw["version"]
	if !ok {
		return nil // absent means the current version
	}
	n, isNumber := value.(json.Number)
	version, err := strconv.Atoi(string(n))
	switch {
	case !isNumber || err != nil || version < 1:
		return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", layer.Path, "key", "version", "value", stringOf(value), "allowed", strconv.Itoa(CurrentVersion))
	case version > CurrentVersion:
		return fault.New(fault.ConfigTooNew, "config.too_new",
			"file", layer.Path, "version", strconv.Itoa(version), "supported", strconv.Itoa(CurrentVersion))
	}
	return nil
}

// overlay copies the values of raw onto the struct v, recording sources.
func overlay(v reflect.Value, raw map[string]any, prefix string, layer Layer, sources map[string]string) error {
	fields := fieldsByName(v.Type())
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, name := range keys {
		value := raw[name]
		key := prefix + name
		index, ok := fields[name]
		if !ok {
			return fault.New(fault.ConfigUnknownKey, "config.unknown_key",
				"file", layer.Path, "key", key, "known", strings.Join(knownKeys(v.Type(), prefix), ", "))
		}
		field := v.Field(index)
		spec := v.Type().Field(index)
		wrongType := func(expected string) error {
			return fault.New(fault.ConfigWrongType, "config.wrong_type",
				"file", layer.Path, "key", key, "expected", expected, "value", stringOf(value))
		}
		switch field.Kind() {
		case reflect.Struct:
			nested, isObject := value.(map[string]any)
			if !isObject {
				return wrongType("object")
			}
			if err := overlay(field, nested, key+".", layer, sources); err != nil {
				return err
			}
			continue
		case reflect.Bool:
			b, isBool := value.(bool)
			if !isBool {
				return wrongType("true or false")
			}
			field.SetBool(b)
		case reflect.String:
			s, isString := value.(string)
			if !isString {
				return wrongType("string")
			}
			if allowed := enumOf(spec); len(allowed) > 0 && !slices.Contains(allowed, s) {
				return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
					"file", layer.Path, "key", key, "value", s, "allowed", strings.Join(allowed, ", "))
			}
			if pattern := spec.Tag.Get("pattern"); pattern != "" && !regexp.MustCompile(pattern).MatchString(s) {
				allowed := spec.Tag.Get("patternHint")
				if allowed == "" {
					allowed = "values matching " + pattern
				}
				return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
					"file", layer.Path, "key", key, "value", s, "allowed", allowed)
			}
			field.SetString(s)
		case reflect.Int:
			n, isNumber := value.(json.Number)
			i, err := strconv.Atoi(string(n))
			if !isNumber || err != nil {
				return wrongType("integer")
			}
			if minimum := spec.Tag.Get("minimum"); minimum != "" {
				if low, _ := strconv.Atoi(minimum); i < low {
					return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
						"file", layer.Path, "key", key, "value", string(n), "allowed", "≥ "+minimum)
				}
			}
			field.SetInt(int64(i))
		case reflect.Slice:
			if field.Type().Elem().Kind() == reflect.Struct {
				list, err := overlayObjects(field.Type(), spec, value, key, layer)
				if err != nil {
					return err
				}
				field.Set(list)
				break
			}
			items, isArray := value.([]any)
			if !isArray {
				return wrongType("array of strings")
			}
			list := reflect.MakeSlice(field.Type(), 0, len(items))
			pattern := spec.Tag.Get("itemPattern")
			for _, item := range items {
				s, isString := item.(string)
				if !isString {
					return wrongType("array of strings")
				}
				if pattern != "" && !regexp.MustCompile(pattern).MatchString(s) {
					allowed := spec.Tag.Get("patternHint")
					if allowed == "" {
						allowed = "values matching " + pattern
					}
					return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
						"file", layer.Path, "key", key, "value", s, "allowed", allowed)
				}
				if allowed := itemEnumOf(spec); len(allowed) > 0 && !slices.Contains(allowed, s) {
					return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
						"file", layer.Path, "key", key, "value", s, "allowed", strings.Join(allowed, ", "))
				}
				list = reflect.Append(list, reflect.ValueOf(s).Convert(field.Type().Elem()))
			}
			field.Set(list)
		case reflect.Map:
			setting, isSetting := field.Interface().(mapSetting)
			if !isSetting {
				panic("config: unsupported map field " + key)
			}
			merged, err := setting.mergedWith(value, key, layer)
			if err != nil {
				return err
			}
			field.Set(reflect.ValueOf(merged))
		default:
			panic("config: unsupported field kind " + field.Kind().String())
		}
		sources[key] = layer.Name
	}
	return nil
}

// itemChecker is a list item that validates itself once decoded: it names
// the field at fault, its value and what is allowed.
type itemChecker interface {
	check() (field, value, allowed string, ok bool)
}

// overlayObjects decodes a list of objects (ship.secrets.rules): each item
// over a zero value, then checked by itself and, with a uniqueBy tag, for a
// field no two items share. The list replaces the weaker one as a whole.
func overlayObjects(t reflect.Type, spec reflect.StructField, value any, key string, layer Layer) (reflect.Value, error) {
	items, isArray := value.([]any)
	if !isArray {
		return reflect.Value{}, wrongTypeAt(layer, key, "array of objects", value)
	}
	list := reflect.MakeSlice(t, 0, len(items))
	unique := spec.Tag.Get("uniqueBy")
	seen := map[string]bool{}
	for i, item := range items {
		itemKey := key + "[" + strconv.Itoa(i) + "]"
		object, isObject := item.(map[string]any)
		if !isObject {
			return reflect.Value{}, wrongTypeAt(layer, itemKey, "object", item)
		}
		elem := reflect.New(t.Elem()).Elem()
		if err := overlay(elem, object, itemKey+".", layer, map[string]string{}); err != nil {
			return reflect.Value{}, err
		}
		if c, ok := elem.Interface().(itemChecker); ok {
			if field, v, allowed, valid := c.check(); !valid {
				return reflect.Value{}, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
					"file", layer.Path, "key", itemKey+"."+field, "value", v, "allowed", allowed)
			}
		}
		if unique != "" {
			id := elem.Field(fieldsByName(t.Elem())[unique]).String()
			if seen[id] {
				return reflect.Value{}, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
					"file", layer.Path, "key", itemKey+"."+unique, "value", id, "allowed", "a "+unique+" no other item has")
			}
			seen[id] = true
		}
		list = reflect.Append(list, elem)
	}
	return list, nil
}

// itemEnumOf returns the values allowed in a list of strings.
func itemEnumOf(f reflect.StructField) []string {
	if tag := f.Tag.Get("itemEnum"); tag != "" {
		return strings.Split(tag, ",")
	}
	return nil
}

// fieldsByName maps json names to field indexes.
func fieldsByName(t reflect.Type) map[string]int {
	fields := map[string]int{}
	for i := range t.NumField() {
		if name := jsonName(t.Field(i)); name != "" {
			fields[name] = i
		}
	}
	return fields
}

func knownKeys(t reflect.Type, prefix string) []string {
	var keys []string
	for i := range t.NumField() {
		if name := jsonName(t.Field(i)); name != "" {
			keys = append(keys, prefix+name)
		}
	}
	return keys
}

func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "-" {
		return ""
	}
	return name
}

func enumOf(f reflect.StructField) []string {
	if tag := f.Tag.Get("enum"); tag != "" {
		return strings.Split(tag, ",")
	}
	return nil
}

func stringOf(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	return string(data)
}
