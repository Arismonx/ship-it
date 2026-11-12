// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		name  string
		paths []string
		want  Needs
	}{
		{"docs only", []string{"README.md", "docs/how-to/install.md", "CHANGELOG.md"}, Needs{}},
		{"core code", []string{"internal/core/risk/risk.go"}, Needs{Go: true}},
		{"an adapter", []string{"internal/adapters/gitexec/gitexec.go"}, Needs{Go: true, System: true}},
		{"the install script", []string{"scripts/install.sh"}, Needs{System: true}},
		{"an adapter's README", []string{"internal/adapters/gitexec/README.md"}, Needs{}},
		{"test data", []string{"internal/core/glob/testdata/fuzz/FuzzMatch/1"}, Needs{Go: true}},
		{"the extension", []string{"extensions/vscode/src/extension.ts"}, Needs{Extension: true}},
		{"the MCP surface", []string{"internal/surface/mcp/tools.go"}, Needs{Go: true, Extension: true}},
		{"the CI itself", []string{"docs/a.md", ".github/workflows/ci.yml"}, everything},
		{"a dependency", []string{"go.sum"}, everything},
		{"another workflow", []string{".github/workflows/stale.yml"}, Needs{}},
	} {
		if got := Classify(tt.paths); got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestWrite(t *testing.T) {
	var out strings.Builder
	if err := write(&out, Needs{Go: true}); err != nil || out.String() != "go=true\nsystem=false\nextension=false\n" {
		t.Errorf("%q, %v", out.String(), err)
	}
}
