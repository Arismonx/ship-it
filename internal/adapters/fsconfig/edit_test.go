// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fsconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func TestReplaceConfigFileIsACompareAndSwap(t *testing.T) {
	repo := t.TempDir()
	files, userDir := newFiles(t, repo)
	ctx := context.Background()

	file, err := files.ReadConfigFile(ctx, ports.ScopeUser)
	if err != nil || file.Exists || file.Path != filepath.Join(userDir, "config.json") {
		t.Fatalf("ReadConfigFile(user) = %+v, %v", file, err)
	}
	// Created, with its directory.
	path, err := files.ReplaceConfigFile(ctx, ports.ConfigFileChange{Scope: ports.ScopeUser, New: []byte("{}\n"), NewExists: true})
	if err != nil || path != file.Path {
		t.Fatalf("create: %s, %v", path, err)
	}
	if file, _ = files.ReadConfigFile(ctx, ports.ScopeUser); !file.Exists || string(file.Data) != "{}\n" {
		t.Fatalf("after create: %+v", file)
	}
	// Refused when the file is not what the caller read.
	stale := ports.ConfigFileChange{Scope: ports.ScopeUser, Old: []byte("other"), OldExists: true, New: []byte("x"), NewExists: true}
	if _, err := files.ReplaceConfigFile(ctx, stale); !errors.Is(err, ports.ErrConfigChanged) {
		t.Fatalf("stale replace: %v", err)
	}
	if _, err := files.ReplaceConfigFile(ctx, ports.ConfigFileChange{Scope: ports.ScopeUser, New: []byte("x"), NewExists: true}); !errors.Is(err, ports.ErrConfigChanged) {
		t.Fatalf("create over an existing file: %v", err)
	}
	// Replaced, then removed.
	if _, err := files.ReplaceConfigFile(ctx, ports.ConfigFileChange{Scope: ports.ScopeUser, Old: []byte("{}\n"), OldExists: true, New: []byte("{\r\n}"), NewExists: true}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{\r\n}" {
		t.Fatalf("content = %q", data)
	}
	if _, err := files.ReplaceConfigFile(ctx, ports.ConfigFileChange{Scope: ports.ScopeUser, Old: []byte("{\r\n}"), OldExists: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file not removed: %v", err)
	}
	entries, _ := os.ReadDir(userDir)
	if len(entries) != 0 {
		t.Fatalf("temporary files left: %v", entries)
	}

	// The repository scope is .github/ship.json.
	path, err = files.ReplaceConfigFile(ctx, ports.ConfigFileChange{Scope: ports.ScopeRepository, New: []byte("{}"), NewExists: true})
	if err != nil || path != filepath.Join(repo, ".github", "ship.json") {
		t.Fatalf("repository: %s, %v", path, err)
	}
}

func TestNoRepositoryScopeOutsideARepository(t *testing.T) {
	files, _ := newFiles(t, "")
	if _, err := files.ReadConfigFile(context.Background(), ports.ScopeRepository); !errors.Is(err, ports.ErrNoConfigScope) {
		t.Fatalf("ReadConfigFile = %v", err)
	}
	if _, err := files.ReplaceConfigFile(context.Background(), ports.ConfigFileChange{Scope: ports.ScopeRepository, NewExists: true}); !errors.Is(err, ports.ErrNoConfigScope) {
		t.Fatalf("ReplaceConfigFile = %v", err)
	}
}
