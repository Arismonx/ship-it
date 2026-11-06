// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// KeyInfo describes one setting key, from the tags of Settings.
type KeyInfo struct {
	Key string `json:"key"`
	// Doc is the doc tag, in English like the JSON Schema.
	Doc string `json:"doc"`
	// Type is "boolean", "integer", "string", "array" (of strings) or
	// "object" (ship.commits.types) or "objects" (an array of objects, such as
	// ship.secrets.rules).
	Type    string   `json:"type"`
	Allowed []string `json:"allowed,omitempty"`
	Minimum *int     `json:"minimum,omitempty"`
	// Default is the built-in value, as JSON.
	Default string `json:"default"`
	// Env is the environment variable that sets the key (EnvName).
	Env string `json:"env"`
}

// Describe returns what Settings says about a leaf key.
func Describe(key string) (KeyInfo, bool) {
	field, ok := leafField(key)
	if !ok {
		return KeyInfo{}, false
	}
	info := KeyInfo{Key: key, Doc: strings.TrimSpace(field.Tag.Get("doc")), Allowed: enumOf(field), Env: EnvName(key)}
	switch field.Type.Kind() {
	case reflect.Bool:
		info.Type = "boolean"
	case reflect.Int:
		info.Type = "integer"
		if minimum := field.Tag.Get("minimum"); minimum != "" {
			low, _ := strconv.Atoi(minimum)
			info.Minimum = &low
		}
	case reflect.Slice:
		info.Type = "array"
		if field.Type.Elem().Kind() == reflect.Struct {
			info.Type = "objects"
		}
		info.Allowed = itemEnumOf(field)
	case reflect.Map:
		info.Type = "object"
	default:
		info.Type = "string"
	}
	for _, e := range Flatten(Defaults()) {
		if e.Key == key {
			info.Default = e.Value
		}
	}
	return info, true
}

// leafField returns the struct field of a leaf setting key.
func leafField(key string) (reflect.StructField, bool) {
	t := reflect.TypeOf(Settings{})
	var field reflect.StructField
	for _, segment := range strings.Split(key, ".") {
		if t.Kind() != reflect.Struct {
			return field, false
		}
		index, ok := fieldsByName(t)[segment]
		if !ok {
			return field, false
		}
		field = t.Field(index)
		t = field.Type
	}
	return field, t.Kind() != reflect.Struct
}

// IsKey reports whether key names a leaf setting ("ship.publish.mode", not "ship").
func IsKey(key string) bool {
	_, ok := leafField(key)
	return ok
}

// UnknownSetting is the fault for a key given on the command line that is no
// setting, with the closest key when one is close enough to be a typo.
func UnknownSetting(key string) *fault.Fault {
	if suggestion := Suggest(key); suggestion != "" {
		return fault.New(fault.ConfigUnknownKey, "config.unknown_setting", "key", key, "suggestion", suggestion)
	}
	return fault.New(fault.ConfigUnknownKey, "config.unknown_setting_none", "key", key)
}

// Suggest returns the setting key closest to key by edit distance, compared
// without case to the whole key and, for a key without a dot, to the last
// segment ("maxlines" → "ship.pullRequest.maxLines"); "" when none is within
// a third of its length (two edits at least).
func Suggest(key string) string {
	typed := strings.ToLower(key)
	best, bestDistance := "", -1
	for _, candidate := range Keys() {
		lower := strings.ToLower(candidate)
		d := editDistance(typed, lower)
		if !strings.Contains(typed, ".") {
			d = min(d, editDistance(typed, lower[strings.LastIndexByte(lower, '.')+1:]))
		}
		if bestDistance < 0 || d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	if bestDistance <= max(2, len([]rune(key))/3) {
		return best
	}
	return ""
}

// editDistance is the Levenshtein distance between a and b, in runes.
func editDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	previous := make([]int, len(y)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(x); i++ {
		current := make([]int, len(y)+1)
		current[0] = i
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(y)]
}

// AssignmentValue returns, as JSON, the value `--set key=text` gives key:
// the same reading of text by the key's type (true/false, an integer, a
// string, a list). Allowed values are checked by Merge, like for --set.
func AssignmentValue(key, text string) (string, error) {
	layer, err := assignmentLayer(key, text, Layer{Name: "ship config set", Path: "ship config set", Kind: KindFlag})
	if err != nil {
		return "", err
	}
	dec := json.NewDecoder(bytes.NewReader(layer.Data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", err
	}
	for _, segment := range strings.Split(key, ".") {
		value = value.(map[string]any)[segment]
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// LayerValue is the value one layer gives a key.
type LayerValue struct {
	// Source is the name the effective settings show: "user",
	// "repository overrides[0]", "SHIP_PUBLISH_MODE", "policy"…
	Source string `json:"source"`
	Path   string `json:"path"`
	Value  string `json:"value"`
}

// Explanation is everything about one key: what it is, what every layer
// says, and which one wins.
type Explanation struct {
	KeyInfo
	// Layers are the layers that set the key, weakest first.
	Layers []LayerValue `json:"layers"`
	// Value is the effective value (JSON) and Source the layer it comes from,
	// "default" when no layer sets it.
	Value      string  `json:"value"`
	Source     string  `json:"source"`
	SourcePath string  `json:"source_path,omitempty"`
	Policy     *Policy `json:"policy,omitempty"`
}

// Explain resolves key over layers for target, as MergeFor does, and reports
// the value each layer gives it on its own (its contexts or overrides that
// match target included).
func Explain(layers []Layer, target Target, key string) (Explanation, error) {
	info, ok := Describe(key)
	if !ok {
		return Explanation{}, UnknownSetting(key)
	}
	x := Explanation{KeyInfo: info, Layers: []LayerValue{}}
	for _, layer := range layers {
		if len(bytes.TrimSpace(layer.Data)) == 0 {
			continue
		}
		alone, err := mergeLayers([]Layer{layer}, target)
		if err != nil {
			return x, err
		}
		if source, set := alone.Sources[key]; set {
			x.Layers = append(x.Layers, LayerValue{Source: source, Path: layer.Path, Value: valuesByKey(alone.Settings)[key]})
		}
	}
	eff, err := MergeFor(layers, target)
	if err != nil {
		return x, err
	}
	x.Value, x.Source, x.Policy = valuesByKey(eff.Settings)[key], eff.Source(key), eff.Policy
	for _, l := range x.Layers {
		if l.Source == x.Source {
			x.SourcePath = l.Path
		}
	}
	return x, nil
}
