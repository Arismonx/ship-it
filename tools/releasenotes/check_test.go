// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"strings"
	"testing"
)

func TestCheckFragment(t *testing.T) {
	fragment := []string{"internal/x.go", "changelog.d/feat-x.md"}
	code := []string{"internal/x.go"}
	for _, tt := range []struct {
		name    string
		title   string
		labels  []string
		changed []string
		ok      bool
	}{
		{"a feature with its fragment", "✨ feat(cli): add ship bug", nil, fragment, true},
		{"a feature without", "✨ feat(cli): add ship bug", nil, code, false},
		{"a breaking fix without", "🐛 fix!: drop v1", nil, code, false},
		{"security without", "🔒️ security(secrets): derive with PBKDF2", nil, code, false},
		{"skipped by the label", "🐛 fix(ci): name the jobs", []string{"kind/bug", SkipLabel}, code, true},
		{"ci needs none", "👷 ci: run each job where needed", nil, code, true},
		{"docs need none", "📝 docs(changelog): record 0.1.2", nil, code, true},
		{"the README is no fragment", "✨ feat: x", nil, []string{"changelog.d/README.md"}, false},
		{"no gitmoji", "feat: x", nil, fragment, true},
	} {
		var out strings.Builder
		if got := checkFragment(&out, tt.title, tt.labels, tt.changed); got != tt.ok {
			t.Errorf("%s: %v, want %v\n%s", tt.name, got, tt.ok, out.String())
		}
	}
}
