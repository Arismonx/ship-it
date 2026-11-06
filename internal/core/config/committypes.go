// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
)

// CommitTypes are the changes ship.commits.types makes to the built-in commit
// types, by type name. A layer changes the types it names and keeps the
// others: a user file may set an emoji, the repository a label.
type CommitTypes map[string]CommitTypeSetting

// CommitTypeSetting is one type of ship.commits.types: removed (false in the
// file), or the fields to change; nil keeps the built-in value.
type CommitTypeSetting struct {
	Removed      bool
	Emoji        *string
	BranchPrefix *string
	Label        *string
	// Release is the version increment of the type: major, minor, patch or
	// none (convention.ReleaseNames).
	Release *string
}

// commitTypeFields are the keys of a type's object, in schema order.
var commitTypeFields = []string{"emoji", "branchPrefix", "label", "release"}

// MarshalJSON writes false for a removed type, else the fields set.
func (s CommitTypeSetting) MarshalJSON() ([]byte, error) {
	if s.Removed {
		return []byte("false"), nil
	}
	return json.Marshal(struct {
		Emoji        *string `json:"emoji,omitempty"`
		BranchPrefix *string `json:"branchPrefix,omitempty"`
		Label        *string `json:"label,omitempty"`
		Release      *string `json:"release,omitempty"`
	}{s.Emoji, s.BranchPrefix, s.Label, s.Release})
}

// Allowed values of ship.commits.types, as error messages give them.
const (
	commitTypesExpected = "an object: type → {emoji, branchPrefix, label, release} or false"
	typeNameAllowed     = "lowercase letters, such as feat"
	branchPrefixAllowed = `lowercase letters and digits separated by single "-" or "." (Conventional Branch)`
	labelAllowed        = `"" (no label), or lowercase letters and digits separated by single "-" or "."`
)

// merged returns types with the value of a layer applied: a type set to
// false is removed, an object changes the fields it holds (a type removed by
// a weaker layer starts again from the built-in one). Every value is checked
// here, with the key that holds it; checkCommitTypes checks the result.
func (types CommitTypes) merged(value any, key string, layer Layer) (CommitTypes, error) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, wrongTypeAt(layer, key, commitTypesExpected, value)
	}
	out := maps.Clone(types)
	if out == nil {
		out = CommitTypes{}
	}
	invalid := func(key, value, allowed string) error {
		return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", layer.Path, "key", key, "value", value, "allowed", allowed)
	}
	for _, name := range sortedKeys(raw) {
		typeKey := key + "." + name
		if !convention.ValidTypeName(name) {
			return nil, invalid(typeKey, name, typeNameAllowed)
		}
		switch v := raw[name].(type) {
		case bool:
			if v {
				return nil, wrongTypeAt(layer, typeKey, "an object, or false to remove the type", v)
			}
			if convention.IsBuiltInType(convention.CommitType(name)) {
				out[name] = CommitTypeSetting{Removed: true}
			} else {
				delete(out, name) // a type a weaker layer added
			}
		case map[string]any:
			setting := out[name]
			if setting.Removed {
				setting = CommitTypeSetting{}
			}
			for _, field := range sortedKeys(v) {
				fieldKey := typeKey + "." + field
				if !slices.Contains(commitTypeFields, field) {
					return nil, fault.New(fault.ConfigUnknownKey, "config.unknown_key",
						"file", layer.Path, "key", fieldKey, "known", strings.Join(commitTypeFields, ", "))
				}
				s, isString := v[field].(string)
				if !isString {
					return nil, wrongTypeAt(layer, fieldKey, "string", v[field])
				}
				switch field {
				case "emoji":
					if strings.TrimSpace(s) == "" || strings.ContainsAny(s, " \t\r\n") {
						return nil, invalid(fieldKey, s, "a gitmoji, without spaces")
					}
					setting.Emoji = &s
				case "branchPrefix":
					if !convention.ValidBranchPrefix(s) {
						return nil, invalid(fieldKey, s, branchPrefixAllowed)
					}
					setting.BranchPrefix = &s
				case "label":
					if !convention.ValidLabel(s) {
						return nil, invalid(fieldKey, s, labelAllowed)
					}
					setting.Label = &s
				case "release":
					if _, ok := convention.ParseRelease(s); !ok {
						return nil, invalid(fieldKey, s, strings.Join(convention.ReleaseNames(), ", "))
					}
					setting.Release = &s
				}
			}
			out[name] = setting
		default:
			return nil, wrongTypeAt(layer, typeKey, "an object, or false to remove the type", v)
		}
	}
	return out, nil
}

// checkCommitTypes checks what only the merged table can tell: an added type
// has an emoji and a branch prefix (two layers may give one each), and a
// type is left. source names the layer that last set the key.
func checkCommitTypes(types CommitTypes, source string) error {
	const key = "ship.commits.types"
	for _, name := range slices.Sorted(maps.Keys(types)) {
		setting := types[name]
		if !setting.Removed && !convention.IsBuiltInType(convention.CommitType(name)) &&
			(setting.Emoji == nil || setting.BranchPrefix == nil) {
			value, _ := json.Marshal(setting)
			return fault.New(fault.ConfigInvalidValue, "config.invalid_value", "file", source, "key", key+"."+name,
				"value", string(value), "allowed", "a new type needs an emoji and a branchPrefix")
		}
	}
	if len(convention.WithTypes(types.Changes())) == 0 {
		return fault.New(fault.ConfigInvalidValue, "config.invalid_value", "file", source, "key", key,
			"value", stringOf(types), "allowed", "at least one commit type left")
	}
	return nil
}

// Changes returns the types as changes to the built-in table, by name.
func (types CommitTypes) Changes() []convention.TypeChange {
	changes := make([]convention.TypeChange, 0, len(types))
	for _, name := range slices.Sorted(maps.Keys(types)) {
		s := types[name]
		var release *convention.ReleaseImpact
		if s.Release != nil {
			impact, _ := convention.ParseRelease(*s.Release) // checked when merged
			release = &impact
		}
		changes = append(changes, convention.TypeChange{Type: convention.CommitType(name), Remove: s.Removed,
			Gitmoji: s.Emoji, BranchPrefix: s.BranchPrefix, Label: s.Label, Release: release})
	}
	return changes
}

// commitTypesSchema is the JSON Schema of ship.commits.types.
func commitTypesSchema() map[string]any {
	str := func(description string, extra ...any) map[string]any {
		p := map[string]any{"type": "string", "description": description}
		for i := 0; i+1 < len(extra); i += 2 {
			p[extra[i].(string)] = extra[i+1]
		}
		return p
	}
	return map[string]any{
		"type":          "object",
		"propertyNames": map[string]any{"pattern": "^[a-z]+$"},
		"additionalProperties": map[string]any{"oneOf": []any{
			map[string]any{"const": false, "description": "Remove this built-in type."},
			map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"emoji":        str("Gitmoji of the type, written in titles.", "minLength", 1, "pattern", `^\S+$`),
					"branchPrefix": str("Conventional Branch prefix of its branches, such as feat or chore.", "pattern", `^[a-z0-9]+(?:[.-][a-z0-9]+)*$`),
					"label":        str(`Kind of its pull requests, after "kind/"; "" for none.`, "pattern", `^([a-z0-9]+(?:[.-][a-z0-9]+)*)?$`),
					"release": str("Version increment of its commits: major, minor, patch or none (built-in: feat minor; fix, perf, security and revert patch; the others none). A breaking change is always major.",
						"enum", convention.ReleaseNames()),
				},
			},
		}},
	}
}

// Conventions returns the commit and branch conventions the settings give
// (ship.commits.*, ship.branches.*), for the convention package.
func (s Settings) Conventions() convention.Rules {
	c, b := s.Ship.Commits, s.Ship.Branches
	rules := convention.Rules{
		SubjectMaxLength: c.SubjectMaxLength, HeaderMaxLength: c.HeaderMaxLength, BodyWidth: c.BodyWidth,
		Scopes: slices.Clone(c.Scopes), ScopeRequired: c.ScopeRequired,
		BranchMaxWords: b.MaxWords, BranchMaxLength: b.MaxLength, BranchWithoutIssue: b.Issue == BranchIssueNone,
	}
	if len(c.Types) > 0 {
		rules.Types = convention.WithTypes(c.Types.Changes())
	}
	return rules
}
