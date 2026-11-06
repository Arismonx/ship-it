// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fsconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/ports"
)

type topLevel string

func (t topLevel) TopLevel(context.Context) (string, error) {
	if t == "" {
		return "", errors.New("not a git repository")
	}
	return string(t), nil
}

func newFiles(t *testing.T, repo string) (*Files, string) {
	t.Helper()
	home := t.TempDir()
	env := platform.Environment{GOOS: runtime.GOOS, Home: home, Getenv: func(k string) string {
		switch k {
		case "AppData", "XDG_CONFIG_HOME":
			return filepath.Join(home, "config")
		}
		return ""
	}}
	dir, err := env.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	env.PolicyPath = filepath.Join(home, "system", "policy.json")
	return &Files{Git: topLevel(repo), Env: env}, dir
}

func TestLayersInOrder(t *testing.T) {
	repo := t.TempDir()
	files, userDir := newFiles(t, repo)
	ctx := context.Background()

	if layers, err := files.Layers(ctx); err != nil || len(layers) != 0 {
		t.Fatalf("no file: %v, %v", layers, err)
	}
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(`{"ship":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := files.CreateRepositoryFile(ctx, []byte(`{"version":1}`))
	if err != nil || path != filepath.Join(repo, ".github", "ship.json") {
		t.Fatalf("CreateRepositoryFile = %s, %v", path, err)
	}
	layers, err := files.Layers(ctx)
	if err != nil || len(layers) != 2 || layers[0].Name != "user" || layers[1].Name != "repository" || string(layers[1].Data) != `{"version":1}` {
		t.Fatalf("Layers = %+v, %v", layers, err)
	}
}

func TestPolicyComesLast(t *testing.T) {
	files, _ := newFiles(t, t.TempDir())
	if err := os.MkdirAll(filepath.Dir(files.Env.PolicyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.Env.PolicyPath, []byte(`{"organization":"Acme"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := files.CreateRepositoryFile(context.Background(), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	layers, err := files.Layers(context.Background())
	if err != nil || len(layers) != 2 || layers[1].Name != "policy" {
		t.Fatalf("Layers = %+v, %v", layers, err)
	}
}

func TestCreateNeverOverwrites(t *testing.T) {
	files, _ := newFiles(t, t.TempDir())
	ctx := context.Background()
	if _, err := files.CreateRepositoryFile(ctx, []byte("first")); err != nil {
		t.Fatal(err)
	}
	path, err := files.CreateRepositoryFile(ctx, []byte("second"))
	if !errors.Is(err, ports.ErrConfigExists) {
		t.Fatalf("error = %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "first" {
		t.Fatalf("file overwritten: %q", data)
	}
}

func TestOutsideARepositoryOnlyTheUserLayer(t *testing.T) {
	files, _ := newFiles(t, "")
	if layers, err := files.Layers(context.Background()); err != nil || len(layers) != 0 {
		t.Fatalf("Layers = %v, %v", layers, err)
	}
}
