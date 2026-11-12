// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcpconfig

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const shipEntry = `"ship-it": {
      "type": "stdio",
      "command": "ship",
      "args": [
        "mcp",
        "--client",
        "claude"
      ]
    }`

// golden compares got with testdata/golden/mcpinstall/name.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "golden", "mcpinstall", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (go test ./internal/core/mcpconfig -run %s -update)", err, t.Name())
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); w != string(got) {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", path, got, w)
	}
}

func target(t *testing.T, c Client) Target {
	t.Helper()
	tg, ok := TargetOf(string(c))
	if !ok {
		t.Fatalf("no target for %s", c)
	}
	return tg
}

// existing holds a server of the user's for each client, in its own format.
var existing = map[Client]string{
	Claude: `{
  "zFirst": true,
  "mcpServers": {
    "github": {"type": "http", "url": "https://api.githubcopilot.com/mcp/"}
  }
}`,
	VSCode: `{
  "inputs": [],
  "servers": {
    "playwright": {"type": "stdio", "command": "npx", "args": ["-y", "@playwright/mcp@latest"]}
  }
}`,
	Cursor: `{"mcpServers": {"docs": {"type": "stdio", "command": "python", "args": ["mcp-server.py"]}}}`,
	Codex: `# Codex settings of the team
model = "gpt-5"

[mcp_servers.context7]
command = "npx" # keep this comment
args = ["-y", "@upstash/context7-mcp"]

[mcp_servers.context7.env]
MY_ENV_VAR = "MY_ENV_VALUE"`,
}

func TestEveryClientHasItsGoldenFiles(t *testing.T) {
	for _, tg := range Targets {
		t.Run(string(tg.Client), func(t *testing.T) {
			fresh, err := tg.Add(nil)
			if err != nil || !fresh.Changed {
				t.Fatalf("Add(nil) = %+v, %v", fresh, err)
			}
			golden(t, string(tg.Client)+".new.golden", fresh.Content)
			if string(tg.Snippet()) != string(fresh.Content) {
				t.Errorf("the snippet of a new file must be the file:\n%s", tg.Snippet())
			}
			merged, err := tg.Add([]byte(existing[tg.Client]))
			if err != nil || !merged.Changed {
				t.Fatalf("Add(existing) = %+v, %v", merged, err)
			}
			golden(t, string(tg.Client)+".merged.golden", merged.Content)
			if again, err := tg.Add(merged.Content); err != nil || again.Changed || again.Kept || string(again.Content) != string(merged.Content) {
				t.Errorf("a second install must change nothing: %+v, %v", again, err)
			}
			if strings.Contains(string(merged.Content), `:\`) || strings.Contains(string(merged.Content), `"/`) {
				t.Errorf("a project file must not hold an absolute path:\n%s", merged.Content)
			}
		})
	}
}

func TestJSONKeepsTheRestInOrderAndValid(t *testing.T) {
	for _, c := range []Client{Claude, VSCode, Cursor} {
		tg := target(t, c)
		got, err := tg.Add([]byte(existing[c]))
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(got.Content, &parsed); err != nil {
			t.Fatalf("%s: invalid JSON: %v\n%s", c, err, got.Content)
		}
		if servers, _ := parsed[tg.ServersKey].(map[string]any); len(servers) != 2 || servers[ServerName] == nil {
			t.Errorf("%s: servers = %v", c, parsed[tg.ServersKey])
		}
	}
	got, _, _ := AddServer([]byte(existing[Claude]))
	s := string(got)
	if strings.Index(s, "zFirst") > strings.Index(s, "mcpServers") || strings.Index(s, `"github"`) > strings.Index(s, `"ship-it"`) ||
		!strings.Contains(s, shipEntry) {
		t.Fatalf("order not kept:\n%s", s)
	}
}

func TestTheUsersShipItIsNeverReplaced(t *testing.T) {
	mine := `{"mcpServers": {"ship-it": {"command": "/opt/ship", "args": ["mcp", "--attended"]}}}`
	got, err := target(t, Claude).Add([]byte(mine))
	if err != nil || got.Changed || !got.Kept || string(got.Content) != mine {
		t.Errorf("Add = %+v, %v", got, err)
	}
	toml := "[mcp_servers.\"ship-it\"] # mine\ncommand = \"/opt/ship\"\nargs = [\"mcp\"]\n"
	got, err = target(t, Codex).Add([]byte(toml))
	if err != nil || got.Changed || !got.Kept || string(got.Content) != toml {
		t.Errorf("Add = %+v, %v", got, err)
	}
}

func TestJSONRefusesBrokenFiles(t *testing.T) {
	for _, bad := range []string{`[1]`, `{"mcpServers": []}`, `{"a": 1`, `{} {}`, "{\n  // a comment\n}"} {
		if _, _, err := AddServer([]byte(bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if _, err := target(t, VSCode).Add([]byte(`{"servers": 3}`)); err == nil {
		t.Error("servers must be an object")
	}
}

func TestTOMLOnlyAppends(t *testing.T) {
	codex := target(t, Codex)
	crlf := "model = \"gpt-5\"\r\n# last line, no newline"
	got, err := codex.Add([]byte(crlf))
	if err != nil || !strings.HasPrefix(string(got.Content), crlf+"\r\n\r\n[mcp_servers.ship-it]\r\n") || strings.Count(string(got.Content), "\n") != strings.Count(string(got.Content), "\r\n") {
		t.Errorf("CRLF file: %q, %v", got.Content, err)
	}
	if again, _ := codex.Add(got.Content); again.Changed {
		t.Error("a second install must change nothing")
	}
}

func TestTOMLRefusesWhatItCannotTell(t *testing.T) {
	for _, file := range []string{
		"mcp_servers = { other = { command = \"x\" } }\n",
		"[mcp_servers]\nship-it = { command = \"ship\", args = [\"mcp\"] }\n",
		"mcp_servers.ship-it.command = \"ship\"\n",
		"# ship-it was here\n",
		"[[mcp_servers]]\ncommand = \"x\"\n",
	} {
		got, err := target(t, Codex).Add([]byte(file))
		if !errors.Is(err, ErrManualEdit) || got.Changed {
			t.Errorf("%q: %+v, %v", file, got, err)
		}
	}
}

func TestUnknownClient(t *testing.T) {
	if _, ok := TargetOf("emacs"); ok {
		t.Error("emacs is not a client")
	}
	if tg, ok := TargetOf(""); !ok || tg.Client != Claude {
		t.Errorf("the default client is %v", tg.Client)
	}
}
