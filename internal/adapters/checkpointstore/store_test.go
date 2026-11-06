// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
)

func newRepo(t *testing.T) *gitexec.Client {
	t.Helper()
	c := gitexec.New(t.TempDir())
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
	write(t, c.Dir, ".gitignore", "*.log\n")
	write(t, c.Dir, "app.txt", "v1\n")
	run(t, c, "add", "--all")
	run(t, c, "commit", "--quiet", "-m", "init")
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

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(t *testing.T, c *gitexec.Client, args ...string) string {
	t.Helper()
	out, err := c.Run(context.Background(), args...)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// inStore runs git against the private checkpoint store of c.
func inStore(t *testing.T, c *gitexec.Client, args ...string) string {
	t.Helper()
	gitDir, err := c.GitDir(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.RunWithEnv(context.Background(), []string{"GIT_DIR=" + filepath.Join(gitDir, "ship", "store.git")}, args...)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestCreateLeavesUserStateUntouched(t *testing.T) {
	c := newRepo(t)
	write(t, c.Dir, "app.txt", "v2 unstaged\n")
	write(t, c.Dir, "new/untracked.txt", "draft\n")
	write(t, c.Dir, "debug.log", "ignored\n")
	run(t, c, "add", "app.txt") // a staged change the checkpoint must not alter
	write(t, c.Dir, "app.txt", "v3 unstaged on top\n")
	statusBefore := run(t, c, "status", "--porcelain=v2")

	store := New(c)
	store.Now = func() time.Time { return time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) }
	cp, err := store.Create(context.Background(), "deliver")
	if err != nil {
		t.Fatal(err)
	}

	if after := run(t, c, "status", "--porcelain=v2"); after != statusBefore {
		t.Fatalf("checkpoint changed the user's status:\nbefore %s\nafter %s", statusBefore, after)
	}
	tree := inStore(t, c, "ls-tree", "-r", "--name-only", cp.ID)
	for _, want := range []string{"app.txt", "new/untracked.txt", ".gitignore"} {
		if !strings.Contains(tree, want) {
			t.Errorf("checkpoint misses %s:\n%s", want, tree)
		}
	}
	if strings.Contains(tree, "debug.log") {
		t.Error("checkpoint contains an ignored file")
	}
	if content := inStore(t, c, "cat-file", "blob", cp.ID+":app.txt"); content != "v3 unstaged on top\n" {
		t.Errorf("checkpoint holds %q, want the working tree version", content)
	}
	if cp.Branch != "main" || cp.Label != "deliver" || cp.Head == "" {
		t.Errorf("metadata = %+v", cp)
	}
	if refs := run(t, c, "for-each-ref", "--format=%(refname)"); strings.Contains(refs, "ship") {
		t.Errorf("checkpoint leaked into the user's refs: %s", refs)
	}
	if log := run(t, c, "log", "--all", "--oneline"); strings.Contains(log, "checkpoint") {
		t.Errorf("checkpoint visible in git log --all:\n%s", log)
	}
	if _, err := c.Run(context.Background(), "cat-file", "-e", cp.ID); err == nil {
		t.Error("checkpoint object must live in the private store, not in the user's object database")
	}
}

func TestRemovePrunesOnlyTheGivenCheckpoints(t *testing.T) {
	c := newRepo(t)
	store := New(c)
	ctx := context.Background()
	var ids []string
	for _, content := range []string{"one\n", "two\n", "three\n"} {
		write(t, c.Dir, "app.txt", content)
		cp, err := store.Create(ctx, "step")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, cp.ID)
	}
	if err := store.Remove(ctx, []string{ids[0], ids[2], "0000000000000000000000000000000000000000"}); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != ids[1] {
		t.Fatalf("List() after Remove = %+v, want only %s", list, ids[1])
	}
	gitDir, _ := c.GitDir(ctx)
	if _, err := c.RunWithEnv(ctx, []string{"GIT_DIR=" + filepath.Join(gitDir, "ship", "store.git")}, "cat-file", "-e", ids[0]); err == nil {
		t.Error("the removed checkpoint object was not pruned")
	}
	if err := store.RestoreFiles(ctx, ids[1]); err != nil || read(t, c.Dir, "app.txt") != "two\n" {
		t.Fatalf("the kept checkpoint no longer restores: %v", err)
	}
}

func TestListAndRestore(t *testing.T) {
	c := newRepo(t)
	store := New(c)
	ctx := context.Background()

	write(t, c.Dir, "app.txt", "first\n")
	first, err := store.Create(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	write(t, c.Dir, "app.txt", "second\n")
	second, err := store.Create(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	if parents := strings.TrimSpace(inStore(t, c, "log", "-1", "--format=%P", second.ID)); parents != "" {
		t.Fatalf("checkpoints must be independent, so removing one keeps the others' ids; parents %s", parents)
	}

	list, err := store.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[0].Label != "two" || list[1].Label != "one" {
		t.Fatalf("List() = %+v", list)
	}

	write(t, c.Dir, "app.txt", "broken\n")
	if err := store.RestoreFiles(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dir, "app.txt"); got != "first\n" {
		t.Fatalf("restored content = %q", got)
	}
}

func TestRestoreIsByteExact(t *testing.T) {
	c := newRepo(t)
	run(t, c, "config", "core.autocrlf", "true")
	write(t, c.Dir, ".gitattributes", "* text=auto\n")
	mixed := "line one\r\nline two\nline three\r\n"
	binary := string([]byte{0x00, 0xFF, '\r', '\n', 0x10, '\n'})
	write(t, c.Dir, "mixed.txt", mixed)
	write(t, c.Dir, "blob.bin", binary)

	store := New(c)
	ctx := context.Background()
	cp, err := store.Create(ctx, "exact")
	if err != nil {
		t.Fatal(err)
	}
	write(t, c.Dir, "mixed.txt", "overwritten\n")
	write(t, c.Dir, "blob.bin", "x")
	if err := store.RestoreFiles(ctx, cp.ID); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c.Dir, "mixed.txt"); got != mixed {
		t.Errorf("mixed.txt = %q, want %q", got, mixed)
	}
	if got := read(t, c.Dir, "blob.bin"); got != binary {
		t.Errorf("blob.bin = %q, want %q", got, binary)
	}
}

func TestListWithoutCheckpoints(t *testing.T) {
	list, err := New(newRepo(t)).List(context.Background(), 5)
	if err != nil || len(list) != 0 {
		t.Fatalf("List() = %v, %v", list, err)
	}
}

// TestRestoreNeverWritesThroughALink: a file replaced by a link to a file
// outside the working tree since the checkpoint comes back as a file, and the
// outside file is left alone.
func TestRestoreNeverWritesThroughALink(t *testing.T) {
	c := newRepo(t)
	write(t, c.Dir, "config.yml", "mine: 1\n")
	store := New(c)
	ctx := context.Background()
	cp, err := store.Create(ctx, "before-link")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "shared.yml")
	if err := os.WriteFile(outside, []byte("shared: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(c.Dir, "config.yml")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links unavailable here: %v", err)
	}
	if err := store.RestoreFiles(ctx, cp.ID); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(outside); string(data) != "shared: 1\n" {
		t.Fatalf("the file outside the tree was overwritten: %q", data)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink != 0 || read(t, c.Dir, "config.yml") != "mine: 1\n" {
		t.Fatalf("config.yml must come back as the checkpointed file: %v, %v", info, err)
	}
}
