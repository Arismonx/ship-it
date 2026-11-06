// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"errors"
	"path"
	"regexp"
	"strings"
)

// ExtendsKind is where a preset named in "extends" comes from.
type ExtendsKind int

// Kinds of presets.
const (
	// BuiltinPreset is shipped with ship-it: "ship-it:recommended".
	BuiltinPreset ExtendsKind = iota + 1
	// FilePreset is a file of the same repository (or next to the user
	// file): "ship/base.json".
	FilePreset
	// GitHubPreset is a file of another GitHub repository, pinned to a full
	// commit SHA so that it never changes under the developer's feet:
	// "github:acme/ship-config/base.json@<40 hex digits>".
	GitHubPreset
)

// Extends is one parsed entry of "extends".
type Extends struct {
	Kind ExtendsKind
	// Name of a built-in preset.
	Name string
	// Path of the file, relative to the repository root (or the user
	// configuration directory, or the root of the GitHub repository).
	Path string
	// Owner, Repository and Commit of a GitHub preset.
	Owner, Repository, Commit string
}

// extendsForms is shown when an entry is malformed.
const extendsForms = `"ship-it:<preset>", a relative .json file, or "github:<owner>/<repo>/<path>.json@<commit sha>"`

var (
	gitHubPreset = regexp.MustCompile(`^github:([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)/(.+)@([0-9a-f]{40})$`)
	presetName   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// ParseExtends reads one entry of "extends".
func ParseExtends(s string) (Extends, error) {
	switch {
	case strings.HasPrefix(s, "ship-it:"):
		name := strings.TrimPrefix(s, "ship-it:")
		if _, ok := builtinPresets[name]; !ok || !presetName.MatchString(name) {
			return Extends{}, errors.New("config: unknown built-in preset " + s)
		}
		return Extends{Kind: BuiltinPreset, Name: name}, nil
	case strings.HasPrefix(s, "github:"):
		m := gitHubPreset.FindStringSubmatch(s)
		if m == nil || !safeRelativeFile(m[3]) {
			return Extends{}, errors.New("config: malformed GitHub preset " + s)
		}
		return Extends{Kind: GitHubPreset, Owner: m[1], Repository: m[2], Path: m[3], Commit: m[4]}, nil
	case safeRelativeFile(s):
		return Extends{Kind: FilePreset, Path: path.Clean(strings.TrimPrefix(s, "./"))}, nil
	}
	return Extends{}, errors.New("config: malformed preset " + s)
}

// String writes the entry back as a source name for messages.
func (e Extends) String() string {
	switch e.Kind {
	case BuiltinPreset:
		return "ship-it:" + e.Name
	case GitHubPreset:
		return "github:" + e.Owner + "/" + e.Repository + "/" + e.Path + "@" + e.Commit[:12]
	}
	return e.Path
}

// safeRelativeFile accepts a relative .json or .jsonc path that stays inside
// its root: no absolute path, no "..", forward slashes.
func safeRelativeFile(p string) bool {
	if p == "" || strings.ContainsAny(p, `\:`) || strings.HasPrefix(p, "/") {
		return false
	}
	clean := path.Clean(strings.TrimPrefix(p, "./"))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	return strings.HasSuffix(clean, ".json") || strings.HasSuffix(clean, ".jsonc")
}

// builtinPresets are the presets shipped with ship-it, as JSONC documents
// (contents decided by the user on 2026-10-24).
var builtinPresets = map[string]string{
	// The defaults, for files that want to say so.
	"recommended": `{}`,
	// Small, always-checked pull requests.
	"strict": `{
	  "ship": {
	    "pullRequest": {"maxLines": 250, "draft": "unattended", "labels": true},
	    "gates": {"enabled": true, "allowSkip": false}
	  }
	}`,
	// A client's repository: deliver locally and review before publishing,
	// never close the client's issues, leave the client's labels alone.
	"agency": `{
	  "ship": {"publish": {"mode": "never"}, "issues": {"keyword": "Refs"}},
	  "github": {"labels": {"taxonomy": "none"}}
	}`,
}

// BuiltinPresetLayer returns the layer of a built-in preset.
func BuiltinPresetLayer(name string) (Layer, bool) {
	data, ok := builtinPresets[name]
	if !ok {
		return Layer{}, false
	}
	return Layer{Name: "ship-it:" + name, Path: "ship-it:" + name, Kind: KindPreset, Data: []byte(data)}, true
}

// ExtendsOf returns the parsed "extends" of a file, in order; a malformed
// file returns nothing (Merge reports the error with its position).
func ExtendsOf(layer Layer) []Extends {
	raw, err := decode(layer)
	if err != nil {
		return nil
	}
	list, _ := raw["extends"].([]any)
	var out []Extends
	for _, item := range list {
		if s, ok := item.(string); ok {
			if e, err := ParseExtends(s); err == nil {
				out = append(out, e)
			}
		}
	}
	return out
}
