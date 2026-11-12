// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	template, err := os.ReadFile("../../.github/pull_request_template.md")
	if err != nil {
		t.Fatal(err)
	}
	empty := string(template)
	tick := func(body, box string) string { return strings.Replace(body, "- [ ] "+box, "- [x] "+box, 1) }
	for _, tt := range []struct {
		name string
		body string
		code int
	}{
		{"the template as it is", empty, 1},
		{"a human", tick(empty, Boxes[0]), 0},
		{"an agent on its own, upper-case X", strings.Replace(empty, "- [ ] "+Boxes[2], "- [X] "+Boxes[2], 1), 0},
		{"two boxes", tick(tick(empty, Boxes[0]), Boxes[1]), 1},
		{"an empty body", "", 1},
	} {
		var out strings.Builder
		if got := check(&out, tt.body); got != tt.code {
			t.Errorf("%s: exit %d, want %d\n%s", tt.name, got, tt.code, out.String())
		}
	}
}

// TestTheTemplateHasEveryBox: the boxes this tool knows are the template's.
func TestTheTemplateHasEveryBox(t *testing.T) {
	template, err := os.ReadFile("../../.github/pull_request_template.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, box := range Boxes {
		if !strings.Contains(string(template), "- [ ] "+box) {
			t.Errorf("the template has no box %q", box)
		}
	}
}
