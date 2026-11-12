// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import "testing"

func TestMCPServeArgs(t *testing.T) {
	for _, tt := range []struct {
		args     []string
		attended bool
		client   string
		ok       bool
	}{
		{[]string{"mcp"}, false, "", true},
		{[]string{"mcp", "--attended"}, true, "", true},
		{[]string{"mcp", "--client", "Claude"}, false, "claude", true},
		{[]string{"mcp", "--client=codex", "--attended"}, true, "codex", true},
		{[]string{"mcp", "install"}, false, "", false},
		{[]string{"mcp", "--client"}, false, "", false},
		{[]string{"status"}, false, "", false},
	} {
		attended, client, ok := mcpServeArgs(tt.args)
		if attended != tt.attended || client != tt.client || ok != tt.ok {
			t.Errorf("mcpServeArgs(%q) = %v, %q, %v", tt.args, attended, client, ok)
		}
	}
}
