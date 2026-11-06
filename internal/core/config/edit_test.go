// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSetValue(t *testing.T) {
	tests := []struct {
		name, doc, key, value, want string
	}{
		{"replace keeps comments", "{\n  // team choice\n  \"ship\": { \"publish\": \"auto\" } // why\n}\n", "ship.publish", `"never"`,
			"{\n  // team choice\n  \"ship\": { \"publish\": \"never\" } // why\n}\n"},
		{"insert after the last member", "{\n  \"version\": 1\n}\n", "ship.publish", `"never"`,
			"{\n  \"version\": 1,\n  \"ship\": {\n    \"publish\": \"never\"\n  }\n}\n"},
		{"insert keeps a trailing comma", "{\n  \"version\": 1,\n}\n", "ship.publish", `"never"`,
			"{\n  \"version\": 1,\n  \"ship\": {\n    \"publish\": \"never\"\n  },\n}\n"},
		{"insert after a line comment", "{\n  \"version\": 1 // format\n}\n", "github.managedBy", `"opentofu"`,
			"{\n  \"version\": 1, // format\n  \"github\": {\n    \"managedBy\": \"opentofu\"\n  }\n}\n"},
		{"tabs and CRLF", "{\r\n\t\"ship\": {\r\n\t\t\"publish\": \"auto\"\r\n\t}\r\n}\r\n", "ship.pullRequest.maxLines", "600",
			"{\r\n\t\"ship\": {\r\n\t\t\"publish\": \"auto\",\r\n\t\t\"pullRequest\": {\r\n\t\t\t\"maxLines\": 600\r\n\t\t}\r\n\t}\r\n}\r\n"},
		{"four spaces", "{\n    \"ship\": {\n        \"publish\": \"auto\"\n    }\n}\n", "ship.gates.enabled", "false",
			"{\n    \"ship\": {\n        \"publish\": \"auto\",\n        \"gates\": {\n            \"enabled\": false\n        }\n    }\n}\n"},
		{"inline object stays inline", `{"ship": {"publish": "auto"}}`, "ship.gates.enabled", "false",
			`{"ship": {"publish": "auto", "gates": { "enabled": false }}}`},
		{"empty object", "{}", "ship.publish", `"never"`, "{\n  \"ship\": {\n    \"publish\": \"never\"\n  }\n}"},
		{"empty nested object", "{\n  \"ship\": {}\n}\n", "ship.publish", `"never"`, "{\n  \"ship\": {\n    \"publish\": \"never\"\n  }\n}\n"},
		{"strings with braces and comment markers", "{\n  \"a\": \"}/* // \\\"\",\n  \"ship\": {\"publish\": \"auto\"}\n}", "ship.publish", `"never"`,
			"{\n  \"a\": \"}/* // \\\"\",\n  \"ship\": {\"publish\": \"never\"}\n}"},
		{"duplicate key: the last one", `{"ship": {"publish": "auto"}, "ship": {"publish": "auto"}}`, "ship.publish", `"never"`,
			`{"ship": {"publish": "auto"}, "ship": {"publish": "never"}}`},
		{"array value", "{\n  \"github\": {\n    \"labels\": {\n      \"areas\": [\"a\"]\n    }\n  }\n}", "github.labels.areas", `[ "a", "b" ]`,
			"{\n  \"github\": {\n    \"labels\": {\n      \"areas\": [\"a\",\"b\"]\n    }\n  }\n}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SetValue([]byte(tt.doc), strings.Split(tt.key, "."), tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("SetValue =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestSetValueRefuses(t *testing.T) {
	for name, tt := range map[string]struct{ doc, value string }{
		"not an object":        {`[1]`, `1`},
		"invalid document":     {`{"a": }`, `1`},
		"invalid value":        {`{}`, `nope`},
		"through a non-object": {`{"ship": 3}`, `1`},
	} {
		if _, err := SetValue([]byte(tt.doc), []string{"ship", "publish"}, tt.value); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestUnset(t *testing.T) {
	tests := []struct {
		name, doc, key, want string
	}{
		{"last member drops the previous comma", "{\n  \"version\": 1,\n  \"ship\": {\n    \"publish\": \"never\"\n  }\n}\n", "ship.publish",
			"{\n  \"version\": 1\n}\n"},
		{"first member, comment of its line", "{\n  \"ship\": {\n    \"publish\": \"never\", // CI\n    \"gates\": {\"enabled\": false}\n  }\n}\n", "ship.publish",
			"{\n  \"ship\": {\n    \"gates\": {\"enabled\": false}\n  }\n}\n"},
		{"other comments stay", "{\n  // keep me\n  \"ship\": {\n    // about publish\n    \"publish\": \"never\",\n    \"issues\": {\"keyword\": \"Refs\"}\n  }\n}\n", "ship.publish",
			"{\n  // keep me\n  \"ship\": {\n    // about publish\n    \"issues\": {\"keyword\": \"Refs\"}\n  }\n}\n"},
		{"a parent with a comment stays", "{\n  \"ship\": {\n    // later\n    \"publish\": \"never\"\n  }\n}\n", "ship.publish",
			"{\n  \"ship\": {\n    // later\n  }\n}\n"},
		{"inline", `{"ship": {"publish": "never", "gates": {"enabled": false}}}`, "ship.gates.enabled",
			`{"ship": {"publish": "never"}}`},
		{"inline first", `{"ship": {"publish": "never", "gates": {"enabled": false}}}`, "ship.publish",
			`{"ship": {"gates": {"enabled": false}}}`},
		{"CRLF", "{\r\n  \"version\": 1,\r\n  \"ship\": {\r\n    \"publish\": \"never\"\r\n  }\r\n}\r\n", "ship.publish",
			"{\r\n  \"version\": 1\r\n}\r\n"},
		{"duplicates: the hidden one stays hidden", `{"ship": {"publish": "a"}, "ship": {"publish": "b", "x": 1}}`, "ship.publish",
			`{"ship": {"publish": "a"}, "ship": {"x": 1}}`},
		{"duplicates: every visible one goes", `{"ship": {"publish": "a", "publish": "b"}}`, "ship.publish", `{}`},
		{"missing key", `{"ship": {}}`, "github.managedBy", `{"ship": {}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Unset([]byte(tt.doc), strings.Split(tt.key, "."))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Unset =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// FuzzEdit checks that SetValue and Unset never break a valid file: the
// result parses as JSONC and differs from the input only at the path.
func FuzzEdit(f *testing.F) {
	f.Add("{}", "ship", "publish", `"never"`)
	f.Add("{\n  // c\n  \"ship\": {\"publish\": \"auto\",}, /* x */\n}", "ship", "publish", `"never"`)
	f.Add("{\"a\": [1, {\"b\": 2}], \"c\": {}}", "c", "d", "[1,2]")
	f.Add("{\r\n\t\"x\": 1 // y\r\n}", "z", "", "true")
	f.Add("{\"a\":1,\"b\":2}", "a", "", "{\"q\":null}")
	f.Add("{ \"a\": { /* only a comment */ } }", "a", "b", "3")
	f.Fuzz(func(t *testing.T, doc, k1, k2, value string) {
		var original map[string]any
		if unmarshal(StripJSONC([]byte(doc)), &original) != nil || original == nil || hasDuplicates(t, doc) {
			return
		}
		if !utf8.ValidString(k1) || !utf8.ValidString(k2) {
			return
		}
		path := []string{k1}
		if k2 != "" {
			path = append(path, k2)
		}
		if !json.Valid([]byte(value)) {
			data, _ := json.Marshal(value)
			value = string(data)
		}
		var decoded any
		_ = unmarshal([]byte(value), &decoded)

		set, err := SetValue([]byte(doc), path, value)
		if err != nil {
			if !blocked(original, path) {
				t.Fatalf("SetValue(%q, %q): %v", doc, path, err)
			}
			return
		}
		got := decodeJSONC(t, set)
		want := decodeJSONC(t, []byte(doc))
		put(want, path, decoded)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("SetValue(%q, %q, %s) =\n%s\ndecodes to %v, want %v", doc, path, value, set, got, want)
		}

		unset, err := Unset(set, path)
		if err != nil {
			t.Fatalf("Unset(%q): %v", set, err)
		}
		after := decodeJSONC(t, unset)
		if _, present := lookup(after, path); present {
			t.Fatalf("Unset(%q, %q) = %s: still there", set, path, unset)
		}
		prune(got, path, true)
		prune(after, path, false)
		if !reflect.DeepEqual(after, got) {
			t.Fatalf("Unset(%q, %q) =\n%s\ndecodes to %v, want %v", set, path, unset, after, got)
		}
	})
}

func decodeJSONC(t *testing.T, doc []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := unmarshal(StripJSONC(doc), &m); err != nil {
		t.Fatalf("invalid JSONC %q: %v", doc, err)
	}
	return m
}

// unmarshal keeps numbers as written: 1E1000 is valid JSON, too big for a
// float64.
func unmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("more than one value")
	}
	return nil
}

func hasDuplicates(t *testing.T, doc string) bool {
	root, err := parseJSONC([]byte(doc))
	if err != nil {
		t.Fatalf("parseJSONC(%q): %v", doc, err)
	}
	var walk func(n *node) bool
	walk = func(n *node) bool {
		seen := map[string]bool{}
		for _, m := range n.members {
			if seen[m.key] || walk(m.value) {
				return true
			}
			seen[m.key] = true
		}
		return false
	}
	return walk(root)
}

// blocked reports whether a value on the path is there and not an object.
func blocked(m map[string]any, path []string) bool {
	for _, segment := range path[:len(path)-1] {
		v, ok := m[segment]
		if !ok {
			return false
		}
		if m, ok = v.(map[string]any); !ok {
			return true
		}
	}
	return false
}

func put(m map[string]any, path []string, value any) {
	for _, segment := range path[:len(path)-1] {
		next, ok := m[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[segment] = next
		}
		m = next
	}
	m[path[len(path)-1]] = value
}

func lookup(m map[string]any, path []string) (any, bool) {
	var v any = m
	for _, segment := range path {
		object, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = object[segment]; !ok {
			return nil, false
		}
	}
	return v, true
}

// prune removes the key at path (when remove) and the parent objects on the
// path that are left empty, as Unset does.
func prune(m map[string]any, path []string, remove bool) {
	if len(path) == 1 {
		if remove {
			delete(m, path[0])
		}
		return
	}
	child, ok := m[path[0]].(map[string]any)
	if !ok {
		return
	}
	prune(child, path[1:], remove)
	if len(child) == 0 {
		delete(m, path[0])
	}
}
