// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"strings"
	"testing"
)

// TestNoToolMerges: merging a stack is a human's decision (ADR-0006).
func TestNoToolMerges(t *testing.T) {
	found := map[string]bool{}
	for _, tl := range tools {
		found[tl.name] = true
		if strings.Contains(tl.name, "merge") {
			t.Errorf("%s: no tool may merge", tl.name)
		}
	}
	if !found["ship_stack"] || !found["ship_stack_status"] {
		t.Errorf("tools = %v", found)
	}
	for _, tl := range stackTools {
		if tl.name == "ship_stack" && !strings.Contains(tl.description, "DRAFT") {
			t.Error("ship_stack must say its pull requests are drafts")
		}
	}
}
