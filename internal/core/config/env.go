// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// EnvPrefix starts every environment variable of a setting.
const EnvPrefix = "SHIP_"

// EnvName returns the environment variable of a setting key: the segments in
// upper case joined by "_", "ship." dropped as the prefix already says it
// ("ship.pullRequest.maxLines" → "SHIP_PULLREQUEST_MAXLINES",
// "github.labels.areas" → "SHIP_GITHUB_LABELS_AREAS").
func EnvName(key string) string {
	key = strings.TrimPrefix(key, "ship.")
	return EnvPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// Keys returns every setting key, in the order of the Settings struct.
func Keys() []string {
	var keys []string
	for _, e := range Flatten(Defaults()) {
		keys = append(keys, e.Key)
	}
	return keys
}

// EnvironmentLayers turns the SHIP_* variables that name a setting into
// layers, one per variable so that an error names it. Other SHIP_ variables
// (SHIP_TOKEN…) are not settings and are left alone; UnknownEnvironment lists
// the ones that look like a mistyped setting.
func EnvironmentLayers(environ []string) ([]Layer, error) {
	byName := map[string]string{}
	for _, key := range Keys() {
		byName[EnvName(key)] = key
	}
	var layers []Layer
	for _, entry := range sortedEnviron(environ) {
		name, value, _ := strings.Cut(entry, "=")
		key, ok := byName[name]
		if !ok {
			continue
		}
		layer, err := assignmentLayer(key, value, Layer{Name: name, Path: "$" + name, Kind: KindEnvironment})
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return layers, nil
}

// notSettings are SHIP_ variables of ship-it that are not settings.
var notSettings = []string{"SHIP_TOKEN", "SHIP_BIN", "SHIP_LANG"}

// UnknownEnvironment returns the SHIP_ variables that are neither a setting
// nor a known variable: probably a typo, which `ship doctor` reports.
func UnknownEnvironment(environ []string) []string {
	known := map[string]bool{}
	for _, key := range Keys() {
		known[EnvName(key)] = true
	}
	var unknown []string
	for _, entry := range sortedEnviron(environ) {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, EnvPrefix) && !known[name] && !slices.Contains(notSettings, name) {
			unknown = append(unknown, name)
		}
	}
	return unknown
}

// FlagLayers turns --set key=value assignments into layers, in order: the
// last assignment of a key wins.
func FlagLayers(assignments []string) ([]Layer, error) {
	var layers []Layer
	for _, a := range assignments {
		key, value, ok := strings.Cut(a, "=")
		key = strings.TrimSpace(key)
		layer := Layer{Name: "--set " + key, Path: "--set " + a, Kind: KindFlag}
		if !ok || key == "" {
			return nil, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
				"file", "--set", "key", a, "value", a, "allowed", "key=value, for example ship.pullRequest.maxLines=600")
		}
		layer, err := assignmentLayer(key, value, layer)
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return layers, nil
}

// assignmentLayer builds the JSON document that sets key to the text value,
// read according to the key's type: true/false (1/0, yes/no), an integer, a
// string, or a list as a JSON array or comma-separated items.
func assignmentLayer(key, text string, layer Layer) (Layer, error) {
	field, ok := fieldOfKey(key)
	if !ok {
		return layer, fault.New(fault.ConfigUnknownKey, "config.unknown_key",
			"file", layer.Path, "key", key, "known", strings.Join(Keys(), ", "))
	}
	var value any
	text = strings.TrimSpace(text)
	switch field.Kind() {
	case reflect.Bool:
		switch strings.ToLower(text) {
		case "true", "1", "yes", "on":
			value = true
		case "false", "0", "no", "off":
			value = false
		default:
			return layer, wrongTypeAt(layer, key, "true or false", text)
		}
	case reflect.Int:
		n, err := strconv.Atoi(text)
		if err != nil {
			return layer, wrongTypeAt(layer, key, "integer", text)
		}
		value = json.Number(strconv.Itoa(n))
	case reflect.Slice:
		var items []any
		if field.Elem().Kind() == reflect.Struct {
			// A list of objects only reads as JSON: [{"id": …}].
			dec := json.NewDecoder(strings.NewReader(text))
			dec.UseNumber()
			if err := dec.Decode(&items); err != nil || items == nil {
				return layer, wrongTypeAt(layer, key, "JSON array of objects", text)
			}
		} else if strings.HasPrefix(text, "[") {
			if err := json.Unmarshal([]byte(text), &items); err != nil {
				return layer, wrongTypeAt(layer, key, "array of strings", text)
			}
		} else {
			items = []any{}
			for _, item := range strings.Split(text, ",") {
				if item = strings.TrimSpace(item); item != "" {
					items = append(items, item)
				}
			}
		}
		value = items
	case reflect.Map:
		dec := json.NewDecoder(strings.NewReader(text))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			return layer, wrongTypeAt(layer, key, reflect.Zero(field).Interface().(mapSetting).expected(), text)
		}
	default:
		value = text
	}
	// Nest the value under the key's segments.
	segments := strings.Split(key, ".")
	doc := map[string]any{segments[len(segments)-1]: value}
	for i := len(segments) - 2; i >= 0; i-- {
		doc = map[string]any{segments[i]: doc}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return layer, err
	}
	layer.Data = data
	return layer, nil
}

// fieldOfKey returns the type of the setting key, if it exists.
func fieldOfKey(key string) (reflect.Type, bool) {
	t := reflect.TypeOf(Settings{})
	for _, segment := range strings.Split(key, ".") {
		if t.Kind() != reflect.Struct {
			return nil, false
		}
		index, ok := fieldsByName(t)[segment]
		if !ok {
			return nil, false
		}
		t = t.Field(index).Type
	}
	return t, t.Kind() != reflect.Struct
}

func sortedEnviron(environ []string) []string {
	sorted := slices.Clone(environ)
	slices.Sort(sorted)
	return sorted
}
