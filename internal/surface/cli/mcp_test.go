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
	if code != ExitOK || len(calls) != 1 || !strings.HasPrefix(calls[0], "write .cursor/mcp.json {") ||
		!strings.Contains(stdout, `"ship-it"`) || !strings.Contains(stderr, ".cursor/mcp.json") {
		t.Errorf("exit %d, calls %q\n%s%s", code, calls, stdout, stderr)
	}

	calls = nil
	code, _, stderr = runWith(git, "", false, "mcp", "install", "--client", "codex", "--yes")
	if code != ExitOK || len(calls) != 1 || !strings.Contains(stderr, "trust") {
		t.Errorf("codex: exit %d, calls %q\n%s", code, calls, stderr)
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
	if code != ExitOK || len(calls) != 0 || !strings.Contains(stderr, "left as it is") {
		t.Errorf("the user's ship-it: exit %d, calls %q\n%s", code, calls, stderr)
	}
}
