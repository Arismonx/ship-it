// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/app"
)

const agentGuide = "docs/how-to/use-with-ai-agents.md"

func readDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// TestTheGuideListsEveryTool checks the table of the guide against the tool
// definitions: same tools, same order, same arguments.
func TestTheGuideListsEveryTool(t *testing.T) {
	row := regexp.MustCompile("(?m)^\\| `(ship_[a-z_]+)` \\| ([^|]*) \\|")
	argument := regexp.MustCompile("`([a-zA-Z]+)`")
	rows := row.FindAllStringSubmatch(readDoc(t, agentGuide), -1)
	if len(rows) != len(tools) {
		t.Fatalf("the guide lists %d tools, the server has %d", len(rows), len(tools))
	}
	for i, tl := range tools {
		if rows[i][1] != tl.name {
			t.Errorf("row %d is %s, want %s", i+1, rows[i][1], tl.name)
			continue
		}
		var documented []string
		for _, m := range argument.FindAllStringSubmatch(rows[i][2], -1) {
			documented = append(documented, m[1])
		}
		var defined []string
		for name := range tl.input["properties"].(map[string]any) {
			defined = append(defined, name)
		}
		slices.Sort(documented)
		slices.Sort(defined)
		if !slices.Equal(documented, defined) {
			t.Errorf("%s: the guide gives the arguments %v, the server %v", tl.name, documented, defined)
		}
	}
}

// TestTheToolCountIsTheSameEverywhere checks every "N tools" of the docs.
func TestTheToolCountIsTheSameEverywhere(t *testing.T) {
	count := regexp.MustCompile(`\b(\d+) (?:MCP )?tools\b`)
	for _, path := range []string{"README.md", "ROADMAP.md", agentGuide, "docs/reference/cli.md", "docs/explanation/compared-to-github.md"} {
		matches := count.FindAllStringSubmatch(readDoc(t, path), -1)
		for _, m := range matches {
			if m[1] != fmt.Sprint(len(tools)) {
				t.Errorf("%s says %q, the server has %d tools", path, m[0], len(tools))
			}
		}
	}
	if matches := count.FindAllString(readDoc(t, agentGuide), -1); len(matches) == 0 {
		t.Errorf("%s must give the number of tools", agentGuide)
	}
}

// TestTheGuideShowsTheRealErrors renders a secret and a red gate as the
// model receives them, and finds both in the guide.
func TestTheGuideShowsTheRealErrors(t *testing.T) {
	s := &Server{}
	secret := s.explain(&app.SecretsFoundError{Findings: []app.SecretView{{
		Path: "config/deploy.env", Line: 3, Rule: "aws_key", Preview: "AKIA…LE",
		Fingerprint: "4f1c2a9e0b7d3c6f8a5e2d1b0c9f7e6a5d4c3b2a1f0e9d8c7b6a5f4e3d2c1b0a",
	}}})
	gate := s.explain(&app.GatesFailedError{Results: []app.GateResult{
		{Name: "lint", Command: "pnpm run lint", Status: app.GatePassed},
		{Name: "test", Command: "pnpm run test", Status: app.GateFailed, Output: "FAIL src/cart.test.ts\n  ✕ handles an empty cart (4 ms)"},
	}})
	guide := readDoc(t, agentGuide)
	for _, text := range []string{secret, gate} {
		if !strings.Contains(guide, "```text\n"+text+"\n```") {
			t.Errorf("%s must show, in a text block:\n%s", agentGuide, text)
		}
	}
}
