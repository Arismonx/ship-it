// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedFilesAreUpToDate guards the rule "regenerate, never edit by
// hand" (AGENTS.md).
func TestGeneratedFilesAreUpToDate(t *testing.T) {
	files, err := generated(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range files {
		got, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil || !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
			t.Errorf("%s is out of date: run go run ./tools/gen (%v)", filepath.ToSlash(path), err)
		}
		if filepath.Ext(path) == ".json" && !json.Valid(want) {
			t.Errorf("%s is not valid JSON", path)
		}
	}
}

// TestDependabotWorkflowAllowsEveryOutput keeps the allowlist of
// .github/workflows/dependabot-gen.yml in step with generated(): its commit
// job refuses a patch touching any other file.
func TestDependabotWorkflowAllowsEveryOutput(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "dependabot-gen.yml"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	in := false
	for _, line := range strings.Split(strings.ReplaceAll(string(workflow), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "ALLOWED: |":
			in = true
		case in && strings.HasPrefix(line, "            ") && trimmed != "":
			allowed[trimmed] = true
		case in:
			in = false
		}
	}
	files, err := generated(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for path := range files {
		if !allowed[filepath.ToSlash(path)] {
			t.Errorf("dependabot-gen.yml: ALLOWED misses %s", filepath.ToSlash(path))
		}
		delete(allowed, filepath.ToSlash(path))
	}
	for path := range allowed {
		t.Errorf("dependabot-gen.yml: ALLOWED lists %s, which tools/gen does not generate", path)
	}
}
