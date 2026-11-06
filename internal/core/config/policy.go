// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// PolicyLayerName names the organisation policy layer: the strongest one.
// Its file is deployed by the IT department where developers cannot write
// (docs/reference/cli.md, "Organisation policy").
const PolicyLayerName = "policy"

// PolicySchemaURL is the JSON Schema of policy.json.
const PolicySchemaURL = "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/policy.schema.json"

// Policy says who imposes the locked settings, for every message about them.
type Policy struct {
	Organization string `json:"organization"`
	Contact      string `json:"contact,omitempty"`
	Path         string `json:"path"`
	// Locked are the keys the policy enforces, sorted.
	Locked []string `json:"locked"`
}

// policyKeys are the top-level keys of policy.json.
var policyKeys = []string{"$schema", "version", "organization", "contact", "enforce"}

// applyPolicy overlays the "enforce" block of a policy file and locks its
// keys. A weaker layer that set a locked key to another value is refused, so
// that nobody wonders why their setting has no effect.
func applyPolicy(eff *Effective, layer Layer) error {
	raw, err := decode(layer)
	if err != nil {
		return err
	}
	if err := checkVersion(layer, raw); err != nil {
		return err
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if !slices.Contains(policyKeys, k) {
			return fault.New(fault.ConfigUnknownKey, "config.unknown_key",
				"file", layer.Path, "key", k, "known", strings.Join(policyKeys, ", "))
		}
	}
	policy := &Policy{Path: layer.Path}
	for key, target := range map[string]*string{"organization": &policy.Organization, "contact": &policy.Contact} {
		if value, ok := raw[key]; ok {
			s, isString := value.(string)
			if !isString {
				return fault.New(fault.ConfigWrongType, "config.wrong_type",
					"file", layer.Path, "key", key, "expected", "string", "value", stringOf(value))
			}
			*target = strings.TrimSpace(s)
		}
	}
	if policy.Organization == "" {
		return fault.New(fault.ConfigInvalidValue, "config.invalid_value",
			"file", layer.Path, "key", "organization", "value", "", "allowed", "the name shown to developers")
	}
	if policy.Contact == "" {
		policy.Contact = policy.Organization // "ask Acme for an exception"
	}

	enforce, ok := raw["enforce"].(map[string]any)
	if _, present := raw["enforce"]; present && !ok {
		return fault.New(fault.ConfigWrongType, "config.wrong_type",
			"file", layer.Path, "key", "enforce", "expected", "object", "value", stringOf(raw["enforce"]))
	}
	before := valuesByKey(eff.Settings)
	locked := map[string]string{}
	if err := overlay(reflect.ValueOf(&eff.Settings).Elem(), enforce, "", layer, locked); err != nil {
		return err
	}
	after := valuesByKey(eff.Settings)
	for key := range locked {
		if source, set := eff.Sources[key]; set && before[key] != after[key] {
			return fault.New(fault.PolicyConflict, "config.policy_conflict",
				"key", key, "value", before[key], "imposed", after[key], "source", source,
				"organization", policy.Organization, "contact", policy.Contact)
		}
		eff.Sources[key] = PolicyLayerName
		policy.Locked = append(policy.Locked, key)
	}
	slices.Sort(policy.Locked)
	eff.Policy = policy
	return nil
}

// valuesByKey returns the JSON value of every leaf setting.
func valuesByKey(s Settings) map[string]string {
	values := map[string]string{}
	for _, e := range Flatten(s) {
		values[e.Key] = e.Value
	}
	return values
}

// PolicySchema returns the JSON Schema of policy.json: "enforce" takes the
// same shape as .github/ship.json.
func PolicySchema() []byte {
	settings := objectSchema(reflect.TypeOf(Settings{}), reflect.ValueOf(Defaults()))
	settings["description"] = "Settings imposed on every repository; developers cannot override them."
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  PolicySchemaURL,
		"title":                "ship-it organisation policy",
		"description":          "Deployed by the IT department where developers cannot write (ProgramData, /Library/Application Support, /etc).",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"organization"},
		"properties": map[string]any{
			"$schema":      map[string]any{"type": "string"},
			"version":      map[string]any{"type": "integer", "enum": []int{CurrentVersion}, "default": CurrentVersion},
			"organization": map[string]any{"type": "string", "minLength": 1, "description": "Name shown to developers when a setting is imposed."},
			"contact":      map[string]any{"type": "string", "description": "Who to ask for an exception (e-mail, URL)."},
			"enforce":      settings,
		},
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}
