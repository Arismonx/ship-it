// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// Render writes a .github/ship.json holding $schema, version and only the
// settings that differ from the defaults (ADR-0009: `ship init` writes no
// noise). Keys follow the declaration order of Settings.
func Render(s Settings) []byte {
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt := func(depth int, key, value string, last bool) {
		b.WriteString(strings.Repeat("  ", depth) + strconv.Quote(key) + ": " + value)
		if !last {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	sections := differences(reflect.ValueOf(s), reflect.ValueOf(Defaults()))
	fmt(1, "$schema", strconv.Quote(SchemaURL), false)
	fmt(1, "version", strconv.Itoa(CurrentVersion), len(sections) == 0)
	writeEntries(&b, sections, 1)
	b.WriteString("}\n")
	return b.Bytes()
}

// Entry is one leaf setting: its dotted key and JSON value.
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Flatten lists every leaf setting in declaration order.
func Flatten(s Settings) []Entry {
	var entries []Entry
	var walk func(v reflect.Value, prefix string)
	walk = func(v reflect.Value, prefix string) {
		for i := range v.NumField() {
			name := jsonName(v.Type().Field(i))
			if name == "" {
				continue
			}
			if v.Field(i).Kind() == reflect.Struct {
				walk(v.Field(i), prefix+name+".")
				continue
			}
			value, _ := json.Marshal(v.Field(i).Interface())
			entries = append(entries, Entry{Key: prefix + name, Value: string(value)})
		}
	}
	walk(reflect.ValueOf(s), "")
	return entries
}

// entry is one key of the rendered file: a leaf (JSON value) or an object.
type entry struct {
	key      string
	value    string
	children []entry
}

// differences returns the fields of v that differ from def, recursively.
func differences(v, def reflect.Value) []entry {
	var entries []entry
	for i := range v.NumField() {
		name := jsonName(v.Type().Field(i))
		if name == "" {
			continue
		}
		field, fieldDefault := v.Field(i), def.Field(i)
		if field.Kind() == reflect.Struct {
			if children := differences(field, fieldDefault); len(children) > 0 {
				entries = append(entries, entry{key: name, children: children})
			}
			continue
		}
		if reflect.DeepEqual(field.Interface(), fieldDefault.Interface()) {
			continue
		}
		value, _ := json.Marshal(field.Interface())
		entries = append(entries, entry{key: name, value: string(value)})
	}
	return entries
}

func writeEntries(b *bytes.Buffer, entries []entry, depth int) {
	indent := strings.Repeat("  ", depth)
	for i, e := range entries {
		b.WriteString(indent + strconv.Quote(e.key) + ": ")
		if e.children != nil {
			b.WriteString("{\n")
			writeEntries(b, e.children, depth+1)
			b.WriteString(indent + "}")
		} else {
			b.WriteString(e.value)
		}
		if i < len(entries)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
}
