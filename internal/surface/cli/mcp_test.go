// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestMCPInstallPrintsTheSnippetOfEveryClient(t *testing.T) {
	for client, want := range map[string]string{
		"claude": "\"mcpServers\": {\n    \"ship-it\": {\n      \"type\": \"stdio\",\n      \"command\": \"ship\",",
		"vscode": "\"servers\": {\n    \"ship-it\": {",
		"cursor": "\"mcpServers\": {",
		"codex":  "[mcp_servers.ship-it]\ncommand = \"ship\"\nargs = [\"mcp\", \"--client\", \"codex\"]\n",
	} {
		var calls []string
		git := testkit.HealthyRepository("main")
		git.Calls = &calls
		code, stdout, stderr := runWith(git, "", false, "mcp", "install", "--print", "--client", client)
		if code != ExitOK || !strings.Contains(stdout, want) || stderr != "" || len(calls) != 0 {
			t.Errorf("%s: exit %d, calls %q\n%s%s", client, code, calls, stdout, stderr)
		}
	}
}

func TestMCPInstallWritesTheClientsFile(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	code, stdout, stderr := runWith(git, "", false, "mcp", "install", "--client", "cursor", "--yes")
	if code != ExitOK || len(calls) != 2 || !strings.HasPrefix(calls[0], "write .cursor/mcp.json {") ||
		!strings.HasPrefix(calls[1], "write .agents/skills/ship-it/SKILL.md ---") ||
		!strings.Contains(stdout, `"ship-it"`) || !strings.Contains(stderr, ".cursor/mcp.json") {
		t.Errorf("exit %d, calls %q\n%s%s", code, calls, stdout, stderr)
	}

	calls = nil
	code, _, stderr = runWith(git, "", false, "mcp", "install", "--client", "codex", "--yes")
	if code != ExitOK || len(calls) != 2 || !strings.Contains(stderr, "trust") {
		t.Errorf("codex: exit %d, calls %q\n%s", code, calls, stderr)
	}

	calls = nil
	if code, _, stderr = runWith(git, "", false, "mcp", "install", "--client", "claude", "--yes", "--no-skill"); code != ExitOK || len(calls) != 1 {
		t.Errorf("--no-skill: exit %d, calls %q\n%s", code, calls, stderr)
	}
}

// TestSkillInstall: #50. Both folders by default, one with --client, the
// user's own SKILL.md never replaced, and --print writes nothing.
func TestSkillInstall(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	code, _, stderr := runWith(git, "", false, "skill", "install", "--yes")
	if code != ExitOK || len(calls) != 2 || !strings.HasPrefix(calls[0], "write .agents/skills/ship-it/SKILL.md") ||
		!strings.HasPrefix(calls[1], "write .claude/skills/ship-it/SKILL.md") {
		t.Fatalf("exit %d, calls %q\n%s", code, calls, stderr)
	}

	calls = nil
	git.WorkingFiles = map[string]string{".claude/skills/ship-it/SKILL.md": "---\nname: ship-it\ndescription: ours\n---\n"}
	code, _, stderr = runWith(git, "", false, "skill", "install", "--client", "claude", "--yes")
	if code != ExitOK || len(calls) != 0 || !strings.Contains(stderr, "not ship-it's") {
		t.Errorf("the user's skill: exit %d, calls %q\n%s", code, calls, stderr)
	}

	calls = nil
	code, stdout, _ := runWith(git, "", false, "skill", "install", "--print")
	if code != ExitOK || len(calls) != 0 || !strings.HasPrefix(stdout, "---\nname: ship-it\n") {
		t.Errorf("--print: exit %d, calls %q", code, calls)
	}
	if code, _, _ := run("skill", "install", "--client", "emacs"); code != ExitUsage {
		t.Errorf("unknown client: exit %d", code)
	}
}

func TestMCPInstallDryRunWritesNothing(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	code, stdout, _ := runWith(git, "", false, "mcp", "install", "--client", "vscode", "--dry-run")
	if code != ExitOK || len(calls) != 0 || !strings.Contains(stdout, "write .vscode/mcp.json") {
		t.Errorf("exit %d, calls %q\n%s", code, calls, stdout)
	}
}

func TestMCPInstallRefusals(t *testing.T) {
	if code, _, stderr := run("mcp", "install", "--client", "emacs"); code != ExitUsage || !strings.Contains(stderr, "claude, vscode, cursor, codex") {
		t.Errorf("unknown client: exit %d\n%s", code, stderr)
	}

	var calls []string
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	git.WorkingFiles = map[string]string{".codex/config.toml": "mcp_servers = { docs = { command = \"x\" } }\n"}
	code, stdout, stderr := runWith(git, "", false, "mcp", "install", "--client", "codex", "--yes")
	if code != ExitNeedsHuman || len(calls) != 0 || !strings.Contains(stderr, "SHIP-E0715") ||
		!strings.Contains(stderr, "Add this to .codex/config.toml") || stdout != "[mcp_servers.ship-it]\ncommand = \"ship\"\nargs = [\"mcp\", \"--client\", \"codex\"]\n" {
		t.Errorf("exit %d, calls %q\n%s%s", code, calls, stdout, stderr)
	}

	git.WorkingFiles = map[string]string{".mcp.json": `{"mcpServers": {"ship-it": {"command": "/opt/ship", "args": ["mcp"]}}}`}
	code, _, stderr = runWith(git, "", false, "mcp", "install", "--yes")
	if code != ExitOK || len(calls) != 1 || !strings.HasPrefix(calls[0], "write .claude/skills/ship-it/SKILL.md") || !strings.Contains(stderr, "left as it is") {
		t.Errorf("the user's ship-it: exit %d, calls %q\n%s", code, calls, stderr)
	}
}
