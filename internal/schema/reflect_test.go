// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package schema

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type inner struct {
	N int `json:"n"`
}

type sample struct {
	Name    string            `json:"name" doc:"The name."`
	Count   int               `json:"count,omitempty"`
	When    time.Time         `json:"when"`
	Tags    []string          `json:"tags"`
	Extra   map[string]string `json:"extra,omitempty"`
	Child   *inner            `json:"child,omitempty"`
	Skipped string            `json:"-"`
	//lint:ignore U1000 present to check that unexported fields are skipped
	hidden string
	Self   *sample `json:"self,omitempty"`
}

func TestFor(t *testing.T) {
	got := For(sample{})
	data, _ := json.Marshal(got)
	var round map[string]any
	_ = json.Unmarshal(data, &round)
	props := round["properties"].(map[string]any)
	if _, ok := props["Skipped"]; ok {
		t.Error("json:\"-\" must be skipped")
	}
	if _, ok := props["hidden"]; ok {
		t.Error("unexported fields must be skipped")
	}
	if props["name"].(map[string]any)["description"] != "The name." {
		t.Error("doc tag lost")
	}
	if props["when"].(map[string]any)["format"] != "date-time" {
		t.Error("time.Time is a date-time string")
	}
	if !reflect.DeepEqual(round["required"], []any{"name", "when", "tags"}) {
		t.Errorf("required = %v", round["required"])
	}
	self := props["self"].(map[string]any)["anyOf"].([]any)
	if self[0].(map[string]any)["type"] != "object" {
		t.Error("recursion must stop")
	}
}

func TestValidate(t *testing.T) {
	s := For(sample{})
	valid := map[string]any{}
	data, _ := json.Marshal(sample{Name: "a", Tags: nil, Child: &inner{N: 1}})
	_ = json.Unmarshal(data, &valid)
	if err := Validate(s, valid); err != nil {
		t.Fatalf("valid value rejected: %v", err)
	}
	for name, bad := range map[string]map[string]any{
		"missing required": {"when": "x", "tags": nil},
		"wrong type":       {"name": 3, "when": "x", "tags": nil},
		"bad item":         {"name": "a", "when": "x", "tags": []any{1}},
		"bad child":        {"name": "a", "when": "x", "tags": nil, "child": map[string]any{"n": "one"}},
	} {
		if Validate(s, bad) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
