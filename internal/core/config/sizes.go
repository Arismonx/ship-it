// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
)

// mapSetting is a setting held in a Go map (ship.commits.types,
// ship.pullRequest.sizes): it merges a layer's value itself, with its own
// rules, and describes its JSON Schema.
type mapSetting interface {
	// mergedWith returns the setting with a layer's value applied, of the
	// same type, or the fault that names the key at fault.
	mergedWith(value any, key string, layer Layer) (any, error)
	jsonSchema() map[string]any
	// typeLabel describes the type in the settings reference.
	typeLabel() string
	// expected is what an error message says the value must be.
	expected() string
}

var (
	_ mapSetting = CommitTypes{}
	_ mapSetting = Sizes{}
)

func (types CommitTypes) mergedWith(value any, key string, layer Layer) (any, error) {
	return types.merged(value, key, layer)
}
func (CommitTypes) jsonSchema() map[string]any { return commitTypesSchema() }
func (CommitTypes) typeLabel() string {
	return "object: type → `{emoji, branchPrefix, label, release}` or `false`"
}
func (CommitTypes) expected() string { return commitTypesExpected }

// Sizes are the size labels of ship.pullRequest.sizes: name → the number of
// changed lines under which the size applies, 0 for everything larger.
type Sizes map[string]int

// DefaultSizes returns Kubernetes' scale, the default of
// ship.pullRequest.sizes.
func DefaultSizes() Sizes {
	s := Sizes{}
	for _, size := range pullrequest.DefaultSizes {
		s[size.Name] = size.Below
	}
	return s
}

// List returns the sizes, smallest first, the unbounded one last.
func (s Sizes) List() []pullrequest.Size {
	list := make([]pullrequest.Size, 0, len(s))
	for name, below := range s {
		list = append(list, pullrequest.Size{Name: name, Below: below})
	}
	return pullrequest.NewTaxonomy("", "", list).Sizes
}

// MarshalJSON writes the sizes in threshold order, smallest first.
func (s Sizes) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, size := range s.List() {
		if i > 0 {
			b.WriteByte(',')
		}
		name, _ := json.Marshal(size.Name)
		b.Write(name)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(size.Below))
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// sizeName is a size label's name, after the prefix.
var sizeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

const sizesExpected = "an object: size name → changed lines under which it applies (0: everything larger)"

// mergedWith replaces the table as a whole, as lists are: thresholds only
// make sense together. Each size is checked, then the table: one size at
// least, distinct thresholds (they increase from the smallest size), at
// most one unbounded size.
func (Sizes) mergedWith(value any, key string, layer Layer) (any, error) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, wrongTypeAt(layer, key, sizesExpected, value)
	}
	invalid := func(key, value, allowed string) error {
		return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", layer.Path, "key", key, "value", value, "allowed", allowed)
	}
	if len(raw) == 0 {
		return nil, invalid(key, "{}", "one size at least")
	}
	out := Sizes{}
	byThreshold := map[int]string{}
	for _, name := range sortedKeys(raw) {
		sizeKey := key + "." + name
		if !sizeName.MatchString(name) {
			return nil, invalid(sizeKey, name, "letters, digits, \".\", \"_\" and \"-\", such as XL")
		}
		n, isNumber := raw[name].(json.Number)
		below, err := strconv.Atoi(string(n))
		if !isNumber || err != nil {
			return nil, wrongTypeAt(layer, sizeKey, "integer", raw[name])
		}
		if below < 0 {
			return nil, invalid(sizeKey, string(n), "≥ 0 (0: everything larger than the other sizes)")
		}
		if other, taken := byThreshold[below]; taken {
			allowed := "a threshold no other size has (" + other + " has it): thresholds increase from the smallest size"
			if below == 0 {
				allowed = "one size at most with 0 (" + other + " has it)"
			}
			return nil, invalid(sizeKey, string(n), allowed)
		}
		byThreshold[below] = name
		out[name] = below
	}
	return out, nil
}

func (Sizes) jsonSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"minProperties":        1,
		"propertyNames":        map[string]any{"pattern": sizeName.String()},
		"additionalProperties": map[string]any{"type": "integer", "minimum": 0},
	}
}
func (Sizes) typeLabel() string { return "object: size name → changed lines (0: the largest)" }
func (Sizes) expected() string  { return sizesExpected }

// PullRequestLabels returns the label taxonomy of the settings
// (ship.pullRequest.labelPrefixes and sizes), for the pullrequest package.
func (s Settings) PullRequestLabels() pullrequest.Taxonomy {
	pr := s.Ship.PullRequest
	return pullrequest.NewTaxonomy(pr.LabelPrefixes.Kind, pr.LabelPrefixes.Size, pr.Sizes.List())
}
