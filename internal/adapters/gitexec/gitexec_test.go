// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/ports"
)

// newRepo creates a real repository with one commit in a temporary directory.
func newRepo(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()
	c := New(dir)
	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"config", "user.name", "ship-it test"},
		{"config", "user.email", "test@example.invalid"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := c.Run(ctx, args...); err != nil {
			t.Skipf("git unavailable: %v", err)
		}
	}
	write(t, dir, "README.md", "hello\n")
	mustRun(t, c, "add", "README.md")
	mustRun(t, c, "commit", "--quiet", "-m", "🎉 chore: init")
	return c
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRun(t *testing.T, c *Client, args ...string) {
	t.Helper()
	_, err := c.Run(context.Background(), args...)
	// Under heavy parallel load, git for Windows sometimes fails to start the
	// local transport; retry as Client.retry does for ship-it itself.
	for attempt := 0; err != nil && attempt < 2 && strings.Contains(err.Error(), "Could not read from remote repository"); attempt++ {
		time.Sleep(500 * time.Millisecond)
		_, err = c.Run(context.Background(), args...)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientAgainstRealRepository(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()

	status, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch.Name != "main" || status.Branch.Commit == "" || !status.Clean() {
		t.Fatalf("fresh repository status = %+v", status)
	}

	write(t, c.Dir, "README.md", "hello world\n")
	write(t, c.Dir, "docs/é space.md", "new\n")
	mustRun(t, c, "mv", "README.md", "READ_ME.md")
	status, err = c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]ports.FileChange{}
	for _, f := range status.Files {
		byPath[f.Path] = f
	}
	if f := byPath["docs/é space.md"]; f.State != ports.Untracked {
		t.Errorf("non-ASCII untracked file not reported unquoted: %+v", status.Files)
	}
	if f := byPath["READ_ME.md"]; f.OriginalPath != "README.md" {
		t.Errorf("rename not reported: %+v", status.Files)
	}

	top, err := c.TopLevel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gitDir, err := c.GitDir(ctx)
	if err != nil || filepath.Base(gitDir) != ".git" || filepath.Dir(gitDir) != top {
		t.Errorf("GitDir() = %q, %v (top %q)", gitDir, err, top)
	}
	if v, err := c.Version(ctx); err != nil || v == "" {
		t.Errorf("Version() = %q, %v", v, err)
	}
}

func TestConfigRemotesAndOperations(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()

	if name, found, err := c.ConfigValue(ctx, "user.name"); err != nil || !found || name != "ship-it test" {
		t.Errorf("ConfigValue(user.name) = %q, %v, %v", name, found, err)
	}
	if _, found, err := c.ConfigValue(ctx, "ship.nothing"); err != nil || found {
		t.Errorf("unset key: found=%v err=%v", found, err)
	}

	mustRun(t, c, "remote", "add", "origin", "https://example.invalid/a.git")
	mustRun(t, c, "remote", "add", "backup", "https://example.invalid/b.git")
	mustRun(t, c, "config", "remote.backup.mirror", "true")
	remotes, err := c.Remotes(ctx)
	if err != nil || len(remotes) != 2 {
		t.Errorf("Remotes() = %v, %v", remotes, err)
	}
	mirrors, err := c.ConfigMatching(ctx, `^remote\..*\.mirror$`)
	if err != nil || mirrors["remote.backup.mirror"] != "true" {
		t.Errorf("ConfigMatching() = %v, %v", mirrors, err)
	}
	if none, err := c.ConfigMatching(ctx, `^nothing\.`); err != nil || len(none) != 0 {
		t.Errorf("ConfigMatching(no match) = %v, %v", none, err)
	}

	if op, err := c.OperationInProgress(ctx); err != nil || op != "" {
		t.Errorf("idle repository reports %q, %v", op, err)
	}
	// Create a real conflict: the same line changed on two branches.
	mustRun(t, c, "switch", "--quiet", "-c", "other")
	write(t, c.Dir, "README.md", "other\n")
	mustRun(t, c, "commit", "--quiet", "-am", "other")
	mustRun(t, c, "switch", "--quiet", "main")
	write(t, c.Dir, "README.md", "main\n")
	mustRun(t, c, "commit", "--quiet", "-am", "main")
	if _, err := c.Run(ctx, "merge", "other"); err == nil {
		t.Fatal("expected a merge conflict")
	}
	if op, err := c.OperationInProgress(ctx); err != nil || op != "merge" {
		t.Errorf("OperationInProgress() = %q, %v; want merge", op, err)
	}
}

func TestRunReportsGitErrors(t *testing.T) {
	c := New(t.TempDir())
	_, err := c.Run(context.Background(), "rev-parse", "--show-toplevel")
	gitErr, ok := err.(*Error)
	if !ok || gitErr.ExitCode == 0 || gitErr.Stderr == "" {
		t.Fatalf("Run() outside a repository error = %#v", err)
	}
	if !errors.Is(err, ports.ErrNotARepository) {
		t.Errorf("outside a repository, the error must match ports.ErrNotARepository: %v", err)
	}
	if (&Error{ExitCode: 128, Stderr: "fatal: bad revision"}).Is(ports.ErrNotARepository) {
		t.Error("another fatal error is not outside a repository")
	}
}
