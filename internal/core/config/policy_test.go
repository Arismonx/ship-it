// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

const acmePolicy = `{
  "version": 1,
  "organization": "Acme",
  "contact": "dsi@acme.example",
  "enforce": {"ship": {"pullRequest": {"draft": "always"}, "issues": {"keyword": "Refs"}}}
}`

func policyLayer(data string) Layer {
	return Layer{Name: PolicyLayerName, Path: "/etc/ship-it/policy.json", Data: []byte(data)}
}

func TestPolicyLocksItsKeys(t *testing.T) {
	repo := Layer{Name: "repository", Path: ".github/ship.json", Data: []byte(`{"ship": {"publish": {"mode": "never"}, "issues": {"keyword": "Refs"}}}`)}
	// The policy is applied last even when listed first.
	eff, err := Merge([]Layer{policyLayer(acmePolicy), repo})
	if err != nil {
		t.Fatal(err)
	}
	if eff.Settings.Ship.Issues.Keyword != "Refs" || eff.Settings.Ship.Publish.Mode != PublishNever {
		t.Fatalf("settings = %+v", eff.Settings.Ship)
	}
	if eff.Source("ship.issues.keyword") != PolicyLayerName || eff.Source("ship.publish.mode") != "repository" {
		t.Errorf("sources = %v", eff.Sources)
	}
	want := &Policy{Organization: "Acme", Contact: "dsi@acme.example", Path: "/etc/ship-it/policy.json",
		Locked: []string{"ship.issues.keyword", "ship.pullRequest.draft"}}
	if !reflect.DeepEqual(eff.Policy, want) {
		t.Errorf("policy = %+v", eff.Policy)
	}
}

func TestPolicyRefusesAContradiction(t *testing.T) {
	user := Layer{Name: "user", Path: "~/.config/ship-it/config.json", Data: []byte(`{"ship": {"issues": {"keyword": "Closes"}}}`)}
	_, err := Merge([]Layer{user, policyLayer(acmePolicy)})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != fault.PolicyConflict {
		t.Fatalf("error = %v", err)
	}
	for k, want := range map[string]string{"key": "ship.issues.keyword", "value": `"Closes"`, "imposed": `"Refs"`,
		"source": "user", "organization": "Acme", "contact": "dsi@acme.example"} {
		if f.Params[k] != want {
			t.Errorf("%s = %q, want %q", k, f.Params[k], want)
		}
	}
}

func TestPolicyFileErrors(t *testing.T) {
	for name, data := range map[string]string{
		"no organization": `{"enforce": {}}`,
		"unknown key":     `{"organization": "Acme", "allow": {}}`,
		"bad enforce":     `{"organization": "Acme", "enforce": []}`,
		"bad setting":     `{"organization": "Acme", "enforce": {"ship": {"reviewers": 2}}}`,
	} {
		var f *fault.Fault
		if _, err := Merge([]Layer{policyLayer(data)}); !errors.As(err, &f) || f.Params["file"] != "/etc/ship-it/policy.json" {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

func TestPolicySchemaIsValidJSON(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(PolicySchema(), &schema); err != nil || schema["$id"] != PolicySchemaURL {
		t.Fatalf("schema: %v", err)
	}
}
