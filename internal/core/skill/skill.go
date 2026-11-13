// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package skill is ship-it's Agent Skill (#50): a SKILL.md that tells a
// coding agent how to deliver work with ship-it, and where each agent reads
// it. The locations come from each client's documentation, read on
// 2026-11-13:
//
//   - Claude Code: .claude/skills/<name>/SKILL.md
//     (https://code.claude.com/docs/en/skills);
//   - Codex: .agents/skills/ (https://learn.chatgpt.com/docs/build-skills);
//   - Cursor: .agents/skills/ and .cursor/skills/
//     (https://cursor.com/docs/skills);
//   - VS Code: .github/skills/, .claude/skills/ and .agents/skills/
//     (https://code.visualstudio.com/docs/copilot/customization/agent-skills).
//
// Two copies cover all four: .agents/skills (the open standard) and
// .claude/skills. `name` must be lowercase letters, digits and hyphens and
// match its folder; `description` holds at most 1024 characters (VS Code).
//
// Allowed imports: standard library without I/O, other core packages.
package skill

import (
	_ "embed"
	"strings"
)

// Name is the skill's name and the folder it lives in.
const Name = "ship-it"

// Marker is in every SKILL.md ship-it writes: a file without it is the
// user's, and is never replaced.
const Marker = "<!-- ship-it skill, written by `ship skill install`"

//go:embed SKILL.md
var content string

// Content is the SKILL.md ship-it installs.
func Content() []byte { return []byte(content) }

// Target is one place a skill is read from, relative to the repository root.
type Target struct {
	// Dir is the skills directory: .agents/skills or .claude/skills.
	Dir string
	// Clients read it.
	Clients []string
}

// Path is the SKILL.md of ship-it in the target.
func (t Target) Path() string { return t.Dir + "/" + Name + "/SKILL.md" }

// Targets are where ship skill install writes, by default all of them.
var Targets = []Target{
	{Dir: ".agents/skills", Clients: []string{"codex", "cursor", "vscode"}},
	{Dir: ".claude/skills", Clients: []string{"claude", "vscode"}},
}

// For returns the target of one client: claude reads .claude/skills, the
// others .agents/skills.
func For(client string) (Target, bool) {
	for _, t := range Targets {
		for _, c := range t.Clients {
			if c == client {
				if client == "vscode" {
					return Targets[0], true
				}
				return t, true
			}
		}
	}
	return Target{}, false
}

// Ours reports whether existing is a SKILL.md ship-it wrote, any version.
func Ours(existing []byte) bool { return strings.Contains(string(existing), Marker) }
