// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package footprint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/ports"
)

type locator struct{ top, gitDir string }

func (l locator) TopLevel(context.Context) (string, error) { return l.top, nil }
func (l locator) GitDir(context.Context) (string, error)   { return l.gitDir, nil }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestFindAndRemove: #55, on a temporary home and repository.
func TestFindAndRemove(t *testing.T) {
	home, top := t.TempDir(), t.TempDir()
	gitDir := filepath.Join(top, ".git")
	write(t, filepath.Join(gitDir, "ship", "oplog", "1.json"), "{}")
	write(t, filepath.Join(gitDir, "config"), "[core]")
	write(t, filepath.Join(home, ".config", "ship-it", "install-salt"), "salt")
	write(t, filepath.Join(home, ".local", "state", "ship-it", "logs", "run.jsonl"), "{}")
	write(t, filepath.Join(top, ".mcp.json"), "{}")
	env := platform.Environment{GOOS: "linux", Home: home, Getenv: func(string) string { return "" },
		PolicyPath: filepath.Join(home, "policy.json")}
	f := New(locator{top: top, gitDir: gitDir}, env)
	ctx := context.Background()

	repo, err := f.Repository(ctx)
	if err != nil || len(repo) != 1 || repo[0].Files != 1 || repo[0].Bytes != 2 {
		t.Fatalf("Repository = %+v, %v", repo, err)
	}
	user, err := f.User(ctx)
	if err != nil || len(user) != 2 || user[0].Holds[0] != "config" || user[1].Path != filepath.Join(home, ".local", "state", "ship-it") {
		t.Fatalf("User = %+v, %v", user, err)
	}

	// Only what it found, and agent files of ship mcp install.
	for _, l := range []ports.Leftover{{Kind: ports.LeftoverRepository, Path: gitDir}, {Kind: ports.LeftoverUser, Path: home},
		{Kind: ports.LeftoverAgentFile, Path: "go.mod"}} {
		if err := f.Remove(ctx, l); err == nil {
			t.Errorf("removed %+v", l)
		}
	}
	for _, l := range append(append(repo, user...), ports.Leftover{Kind: ports.LeftoverAgentFile, Path: ".mcp.json"}) {
		if err := f.Remove(ctx, l); err != nil {
			t.Fatalf("Remove(%+v): %v", l, err)
		}
	}
	if exists(filepath.Join(gitDir, "ship")) || !exists(filepath.Join(gitDir, "config")) ||
		exists(filepath.Join(home, ".config", "ship-it")) || !exists(filepath.Join(home, ".local", "state")) ||
		exists(filepath.Join(top, ".mcp.json")) {
		t.Fatal("removed too little or too much")
	}
	if again, err := f.Repository(ctx); err != nil || len(again) != 0 {
		t.Fatalf("after removal: %+v, %v", again, err)
	}
}

// TestSharedWindowsDirectoryIsListedOnce: the cache and the logs share
// %LocalAppData%\ship-it on Windows.
func TestSharedWindowsDirectoryIsListedOnce(t *testing.T) {
	local := t.TempDir()
	write(t, filepath.Join(local, "ship-it", "cache", "a"), "x")
	write(t, filepath.Join(local, "ship-it", "logs", "b"), "y")
	vars := map[string]string{"LocalAppData": local, "AppData": filepath.Join(local, "roaming")}
	env := platform.Environment{GOOS: "windows", Home: local, Getenv: func(k string) string { return vars[k] }}
	user, err := New(locator{}, env).User(context.Background())
	if err != nil || len(user) != 1 || len(user[0].Holds) != 2 || user[0].Files != 2 {
		t.Fatalf("User = %+v, %v", user, err)
	}
}
