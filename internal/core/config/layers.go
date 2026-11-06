// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/glob"
)

// LayerKind says what a layer is, hence which top-level keys it may hold and
// where it stands in the precedence (decision of 2026-10-23):
//
//	defaults < presets < user < user contexts < repository < repository
//	overrides < environment (SHIP_*) < --set < organisation policy
//
// A preset (extends) sits right before the layer that extends it. Inside a
// layer, its settings apply first, then its matching contexts or overrides in
// file order: an explicit setting of a later layer beats the overrides of an
// earlier one, as in ESLint's flat configuration.
type LayerKind string

// Layer kinds.
const (
	KindUser        LayerKind = "user"
	KindRepository  LayerKind = "repository"
	KindPreset      LayerKind = "preset"
	KindEnvironment LayerKind = "environment"
	KindFlag        LayerKind = "flag"
	KindPolicy      LayerKind = "policy"
)

// kind returns the layer's kind; layers built before kinds existed are
// recognised by name.
func (l Layer) kind() LayerKind {
	if l.Kind != "" {
		return l.Kind
	}
	switch l.Name {
	case "user":
		return KindUser
	case PolicyLayerName:
		return KindPolicy
	}
	return KindRepository
}

// documentKeys are the top-level keys each kind of file may hold. The
// interface settings (ui) are personal: the user file, its contexts, SHIP_*
// and --set hold them, never a file shared with a team (decision of
// 2026-11-05).
var documentKeys = map[LayerKind][]string{
	KindUser:        {"$schema", "version", "extends", "ship", "github", "ui", "contexts"},
	KindRepository:  {"$schema", "version", "extends", "ship", "github", "overrides"},
	KindPreset:      {"$schema", "version", "extends", "ship", "github", "overrides"},
	KindEnvironment: {"ship", "github", "ui"},
	KindFlag:        {"ship", "github", "ui"},
}

// personalSection holds the settings of one person, refused in shared files.
const personalSection = "ui"

// IsPersonalKey reports whether key is a personal setting, which only the
// user file, its contexts, SHIP_* and --set may hold.
func IsPersonalKey(key string) bool {
	return key == personalSection || strings.HasPrefix(key, personalSection+".")
}

// personalOnly is the error of a personal section in a shared file or an
// override.
func personalOnly(layer Layer, key string) error {
	return fault.New(fault.ConfigUnknownKey, "config.personal_only", "file", layer.Path, "key", key)
}

// settingsSections are the top-level keys that hold settings.
var settingsSections = []string{"ship", "github", "ui"}

// Target is the repository a configuration is resolved for. Its zero value
// matches no context and applies no override.
type Target struct {
	// Owner and Host are those of the GitHub repository of origin.
	Owner, Host string
	// Dir is the root of the working tree, and Home the user's home directory
	// (for "~/" in context paths); FoldCase compares them without case
	// (Windows, macOS default volumes).
	Dir, Home string
	FoldCase  bool
	// Paths are the files of the delivery: an override applies when every
	// one of them matches its patterns. None: overrides do not apply.
	Paths []string
}

// applyDocument overlays one decoded file: its settings, then its contexts or
// overrides that match target.
func applyDocument(eff *Effective, layer Layer, raw map[string]any, target Target) error {
	allowed := documentKeys[layer.kind()]
	for _, k := range sortedKeys(raw) {
		if k == personalSection && !slices.Contains(allowed, k) {
			return personalOnly(layer, k)
		}
		if !slices.Contains(allowed, k) {
			return fault.New(fault.ConfigUnknownKey, "config.unknown_key",
				"file", layer.Path, "key", k, "known", strings.Join(allowed, ", "))
		}
	}
	if err := checkExtends(layer, raw["extends"]); err != nil {
		return err
	}
	settings := map[string]any{}
	for _, k := range settingsSections {
		if v, ok := raw[k]; ok {
			settings[k] = v
		}
	}
	if err := overlay(reflect.ValueOf(&eff.Settings).Elem(), settings, "", layer, eff.Sources); err != nil {
		return err
	}
	if blocks, ok := raw["contexts"]; ok {
		if err := applyBlocks(eff, layer, "contexts", blocks, target, contextMatches); err != nil {
			return err
		}
	}
	if blocks, ok := raw["overrides"]; ok {
		if err := applyBlocks(eff, layer, "overrides", blocks, target, overrideMatches); err != nil {
			return err
		}
	}
	return nil
}

// matcher validates the selector of a block and reports whether it matches.
type matcher func(layer Layer, key string, block map[string]any, target Target) (bool, error)

// applyBlocks validates every block of a contexts or overrides list (an
// error in a block that does not match is still an error) and overlays the
// matching ones, in order, under the source "<layer> <key>[i]".
func applyBlocks(eff *Effective, layer Layer, name string, value any, target Target, matches matcher) error {
	list, ok := value.([]any)
	if !ok {
		return wrongTypeAt(layer, name, "array of objects", value)
	}
	selector := map[string]string{"contexts": "match", "overrides": "paths"}[name]
	for i, item := range list {
		key := name + "[" + strconv.Itoa(i) + "]"
		block, ok := item.(map[string]any)
		if !ok {
			return wrongTypeAt(layer, key, "object", item)
		}
		// Overrides are per path: only ship settings make sense there.
		known := []string{selector, "ship"}
		if name == "contexts" {
			known = append(known, "github", "ui")
		}
		for _, k := range sortedKeys(block) {
			if k == personalSection && !slices.Contains(known, k) {
				return personalOnly(layer, key+"."+k)
			}
			if !slices.Contains(known, k) {
				return fault.New(fault.ConfigUnknownKey, "config.unknown_key",
					"file", layer.Path, "key", key+"."+k, "known", strings.Join(known, ", "))
			}
		}
		matched, err := matches(layer, key, block, target)
		if err != nil {
			return err
		}
		settings := map[string]any{}
		for _, k := range settingsSections {
			if v, ok := block[k]; ok {
				settings[k] = v
			}
		}
		// Type-check against a scratch copy, then apply only if it matches.
		scratch := Defaults()
		if err := overlay(reflect.ValueOf(&scratch).Elem(), settings, "", layer, map[string]string{}); err != nil {
			return err
		}
		if matched {
			source := layer
			source.Name = layer.Name + " " + key
			if err := overlay(reflect.ValueOf(&eff.Settings).Elem(), settings, "", source, eff.Sources); err != nil {
				return err
			}
		}
	}
	return nil
}

// contextMatches: every criterion given in "match" holds for the target.
func contextMatches(layer Layer, key string, block map[string]any, target Target) (bool, error) {
	raw, ok := block["match"].(map[string]any)
	if !ok || len(raw) == 0 {
		return false, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", layer.Path, "key", key+".match", "value", stringOf(block["match"]), "allowed", "an object with owner, host or path")
	}
	matched := true
	for _, k := range sortedKeys(raw) {
		s, isString := raw[k].(string)
		if !isString || strings.TrimSpace(s) == "" {
			return false, wrongTypeAt(layer, key+".match."+k, "non-empty string", raw[k])
		}
		switch k {
		case "owner":
			matched = matched && strings.EqualFold(s, target.Owner)
		case "host":
			matched = matched && strings.EqualFold(s, target.Host)
		case "path":
			if !glob.Valid(s) {
				return false, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
					"file", layer.Path, "key", key+".match.path", "value", s, "allowed", "a glob pattern")
			}
			matched = matched && dirMatches(s, target)
		default:
			return false, fault.New(fault.ConfigUnknownKey, "config.unknown_key",
				"file", layer.Path, "key", key+".match."+k, "known", "owner, host, path")
		}
	}
	return matched, nil
}

// dirMatches matches the repository root against an absolute pattern, "~/"
// standing for the home directory.
func dirMatches(pattern string, target Target) bool {
	if target.Dir == "" {
		return false
	}
	clean := func(s string) string {
		s = strings.TrimPrefix(strings.ReplaceAll(s, `\`, "/"), "/")
		if target.FoldCase {
			s = strings.ToLower(s)
		}
		return s
	}
	if rest, ok := strings.CutPrefix(pattern, "~/"); ok && target.Home != "" {
		pattern = strings.TrimSuffix(strings.ReplaceAll(target.Home, `\`, "/"), "/") + "/" + rest
	}
	p, dir := clean(pattern), clean(target.Dir)
	if !strings.Contains(p, "/") {
		return false // a bare name would match any directory of that name
	}
	return glob.Match(p, dir)
}

// overrideMatches: the delivery has files and each matches one of "paths".
func overrideMatches(layer Layer, key string, block map[string]any, target Target) (bool, error) {
	list, ok := block["paths"].([]any)
	if !ok || len(list) == 0 {
		return false, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", layer.Path, "key", key+".paths", "value", stringOf(block["paths"]), "allowed", "a non-empty array of glob patterns")
	}
	patterns := make([]string, len(list))
	for i, item := range list {
		s, isString := item.(string)
		if !isString || !glob.Valid(s) || strings.TrimSpace(s) == "" {
			return false, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
				"file", layer.Path, "key", key+".paths", "value", stringOf(item), "allowed", "a glob pattern")
		}
		patterns[i] = s
	}
	if len(target.Paths) == 0 {
		return false, nil
	}
	for _, p := range target.Paths {
		if !glob.Any(patterns, p) {
			return false, nil
		}
	}
	return true, nil
}

// checkExtends validates "extends"; the presets themselves are resolved by
// the caller (ResolveExtends) and given as layers before this one.
func checkExtends(layer Layer, value any) error {
	if value == nil {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return wrongTypeAt(layer, "extends", "array of strings", value)
	}
	for _, item := range list {
		s, isString := item.(string)
		if !isString {
			return wrongTypeAt(layer, "extends", "array of strings", value)
		}
		if _, err := ParseExtends(s); err != nil {
			return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
				"file", layer.Path, "key", "extends", "value", s, "allowed", extendsForms)
		}
	}
	return nil
}

func wrongTypeAt(layer Layer, key, expected string, value any) error {
	return fault.New(fault.ConfigWrongType, "config.wrong_type",
		"file", layer.Path, "key", key, "expected", expected, "value", stringOf(value))
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
