// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestInstallMCPForEveryClientThenUndo(t *testing.T) {
	paths := map[string]string{"claude": ".mcp.json", "vscode": ".vscode/mcp.json", "cursor": ".cursor/mcp.json", "codex": ".codex/config.toml"}
	for _, client := range MCPClients() {
		t.Run(client, func(t *testing.T) {
			var calls []string
			git := testkit.HealthyRepository("main")
			git.Calls = &calls
			git.WorkingFiles = map[string]string{}
			ws, _, log := workspaceOf(git)

			plan, err := InstallMCP(context.Background(), ws, client, false)
			if err != nil || !plan.Changed || plan.Applied || plan.Path != paths[client] || len(calls) != 0 {
				t.Fatalf("plan = %+v, %v, calls %q", plan, err, calls)
			}
			done, err := InstallMCP(context.Background(), ws, client, true)
			if err != nil || !done.Applied || done.Operation == "" || len(log.Operations) != 1 {
				t.Fatalf("install = %+v, %v", done, err)
			}
			if !strings.Contains(done.After, `"ship"`) || strings.Contains(done.After, `:\`) {
				t.Errorf("the file must launch ship from the PATH:\n%s", done.After)
			}
			if again, _ := InstallMCP(context.Background(), ws, client, true); again.Changed || len(log.Operations) != 1 {
				t.Errorf("a second install must change nothing: %+v", again)
			}
			calls = nil
			if _, err := Undo(context.Background(), ws, UndoRequest{}); err != nil {
				t.Fatal(err)
			}
			if want := []string{"remove " + paths[client]}; !reflect.DeepEqual(calls, want) {
				t.Errorf("undo calls %q, want %q", calls, want)
			}
		})
	}
}

func TestInstallMCPKeepsTheUsersServersAndUndoRestoresThem(t *testing.T) {
	var calls []string
	mine := "{\"servers\": {\"playwright\": {\"type\": \"stdio\", \"command\": \"npx\"}}}\n"
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	git.WorkingFiles = map[string]string{".vscode/mcp.json": mine}
	ws, _, _ := workspaceOf(git)
	done, err := InstallMCP(context.Background(), ws, "vscode", true)
	if err != nil || !strings.Contains(done.After, `"playwright"`) || !strings.Contains(done.After, `"ship-it"`) || done.Before != mine {
		t.Fatalf("install = %+v, %v", done, err)
	}
	calls = nil
	if _, err := Undo(context.Background(), ws, UndoRequest{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"write .vscode/mcp.json " + strings.ReplaceAll(mine, "\n", `\n`)}; !reflect.DeepEqual(calls, want) {
		t.Errorf("undo calls %q, want %q", calls, want)
	}
}

func TestInstallMCPLeavesAFileItCannotChange(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	git.WorkingFiles = map[string]string{".codex/config.toml": "mcp_servers = { docs = { command = \"x\" } }\n"}
	ws, _, log := workspaceOf(git)
	_, err := InstallMCP(context.Background(), ws, "codex", true)
	if !errors.Is(err, &fault.Fault{Code: fault.MCPManualEdit}) || !NeedsHuman(err) || len(calls) != 0 || len(log.Operations) != 0 {
		t.Errorf("err %v, calls %q", err, calls)
	}
	path, snippet, err := MCPSnippet("codex")
	if err != nil || path != ".codex/config.toml" || !strings.HasPrefix(snippet, "[mcp_servers.ship-it]\n") {
		t.Errorf("snippet %s %q, %v", path, snippet, err)
	}
	if _, err := InstallMCP(context.Background(), ws, "emacs", true); err == nil {
		t.Error("an unknown client must be refused")
	}
}

func TestDryRunOfMCPInstallForEveryClient(t *testing.T) {
	for _, client := range MCPClients() {
		var calls []string
		git := testkit.HealthyRepository("main")
		git.Calls = &calls
		real, _, log := workspaceOf(git)
		view, err := InstallMCP(context.Background(), rehearsedWorkspace(t, real), client, true)
		if err != nil {
			t.Fatal(err)
		}
		wantPlan(t, view.Rehearsal, "write_file", "log")
		if !view.Changed || view.After == "" || len(calls) != 0 || len(log.Operations) != 0 {
			t.Errorf("%s: view = %+v, calls %q", client, view, calls)
		}
	}
}
