// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package skill

import (
	"regexp"
	"strings"
	"testing"
)

// TestTheSkillLoads: the frontmatter every client requires (#50).
func TestTheSkillLoads(t *testing.T) {
	text := string(Content())
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("SKILL.md must open with --- on its first line (Claude Code reads the frontmatter only then)")
	}
	front, _, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	if !ok {
		t.Fatal("no closing --- for the frontmatter")
	}
	name := regexp.MustCompile(`(?m)^name: (.+)$`).FindStringSubmatch(front)
	description := regexp.MustCompile(`(?m)^description: (.+)$`).FindStringSubmatch(front)
	if name == nil || name[1] != Name || !regexp.MustCompile(`^[a-z0-9-]{1,64}$`).MatchString(name[1]) {
		t.Errorf("name = %v: lowercase letters, digits and hyphens, at most 64, and %q like its folder", name, Name)
	}
	if description == nil || len(description[1]) > 1024 {
		t.Errorf("description missing or longer than 1024 characters (VS Code's limit): %v", description)
	}
	if !Ours(Content()) {
		t.Error("the installed SKILL.md must carry the marker")
	}
	if lines := strings.Count(text, "\n"); lines > 500 {
		t.Errorf("SKILL.md has %d lines; Claude Code advises under 500", lines)
	}
}

func TestFor(t *testing.T) {
	for client, want := range map[string]string{"claude": ".claude/skills/ship-it/SKILL.md", "codex": ".agents/skills/ship-it/SKILL.md",
		"cursor": ".agents/skills/ship-it/SKILL.md", "vscode": ".agents/skills/ship-it/SKILL.md"} {
		if got, ok := For(client); !ok || got.Path() != want {
			t.Errorf("For(%s) = %s, want %s", client, got.Path(), want)
		}
	}
	if _, ok := For("emacs"); ok {
		t.Error("emacs has no skills folder")
	}
}
