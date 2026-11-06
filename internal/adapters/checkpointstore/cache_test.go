// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hashCounter records the paths given to `hash-object --stdin-paths` and
// the number of `fast-import` runs.
type hashCounter struct {
	GitRunner
	hashed  []string
	imports int
}

func (h *hashCounter) RunWithInput(ctx context.Context, env []string, stdin []byte, args ...string) ([]byte, error) {
	if len(args) > 0 && args[len(args)-1] == "--stdin-paths" {
		h.hashed = append(h.hashed, strings.Fields(string(stdin))...)
	}
	return h.GitRunner.RunWithInput(ctx, env, stdin, args...)
}

func (h *hashCounter) RunWithReader(ctx context.Context, env []string, stdin io.Reader, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "fast-import" {
		h.imports++
	}
	return h.GitRunner.RunWithReader(ctx, env, stdin, args...)
}

// age sets the modification time of files an hour back: older than the
// racy window, so their stamp can be trusted.
func age(t *testing.T, dir string, names ...string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	for _, name := range names {
		if err := os.Chtimes(filepath.Join(dir, filepath.FromSlash(name)), old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func storeContent(t *testing.T, s *Store, id, path string) string {
	t.Helper()
	c := s.Git.(*hashCounter).GitRunner
	gitDir, _ := c.GitDir(context.Background())
	out, err := c.RunWithInput(context.Background(), []string{"GIT_DIR=" + filepath.Join(gitDir, "ship", "store.git")}, nil, "cat-file", "blob", id+":"+path)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestCacheHashesOnlyChangedFiles: a second checkpoint reads only the files
// whose stamp changed, and still stores every file.
func TestCacheHashesOnlyChangedFiles(t *testing.T) {
	c := newRepo(t)
	write(t, c.Dir, "a.txt", "aaaa\n")
	write(t, c.Dir, "b.txt", "bbbb\n")
	age(t, c.Dir, ".gitignore", "app.txt", "a.txt", "b.txt")
	counter := &hashCounter{GitRunner: c}
	store := New(counter)
	ctx := context.Background()
	if _, err := store.Create(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if len(counter.hashed) != 4 {
		t.Fatalf("first checkpoint hashed %q, want every file", counter.hashed)
	}

	counter.hashed = nil
	write(t, c.Dir, "b.txt", "BBBB changed\n")
	age(t, c.Dir, "b.txt")
	cp, err := store.Create(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(counter.hashed, " ") != "b.txt" {
		t.Errorf("second checkpoint hashed %q, want only b.txt", counter.hashed)
	}
	if got := storeContent(t, store, cp.ID, "a.txt"); got != "aaaa\n" {
		t.Errorf("a.txt (cached) = %q", got)
	}
	if got := storeContent(t, store, cp.ID, "b.txt"); got != "BBBB changed\n" {
		t.Errorf("b.txt = %q", got)
	}
}

// TestCacheDistrustsRecentFiles: a file rewritten with the same size and
// modification time right after a checkpoint (a coarse clock) is hashed
// again, since its time falls in the racy window.
func TestCacheDistrustsRecentFiles(t *testing.T) {
	c := newRepo(t)
	write(t, c.Dir, "a.txt", "aaaa\n")
	counter := &hashCounter{GitRunner: c}
	store := New(counter)
	ctx := context.Background()
	if _, err := store.Create(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(c.Dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, c.Dir, "a.txt", "zzzz\n")
	if err := os.Chtimes(filepath.Join(c.Dir, "a.txt"), info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	cp, err := store.Create(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	if got := storeContent(t, store, cp.ID, "a.txt"); got != "zzzz\n" {
		t.Fatalf("a.txt = %q: a racy stamp was trusted", got)
	}
}

// TestCacheWithAMissingBlob: a cached blob pruned from the store makes the
// tree invalid; the checkpoint is taken again from the files.
func TestCacheWithAMissingBlob(t *testing.T) {
	c := newRepo(t)
	write(t, c.Dir, "a.txt", "aaaa\n")
	age(t, c.Dir, ".gitignore", "app.txt", "a.txt")
	counter := &hashCounter{GitRunner: c}
	store := New(counter)
	ctx := context.Background()
	if _, err := store.Create(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	gitDir, _ := c.GitDir(ctx)
	cachePath := filepath.Join(gitDir, "ship", "store.git", cacheFile)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	// Point a.txt at a blob the store does not have.
	missing := "0123456789abcdef0123456789abcdef01234567"
	var patched [][]byte
	for _, record := range bytes.Split(data, []byte{0}) {
		if bytes.HasSuffix(record, []byte("\ta.txt")) {
			record = append([]byte(missing), record[len(missing):]...)
		}
		patched = append(patched, record)
	}
	if err := os.WriteFile(cachePath, bytes.Join(patched, []byte{0}), 0o644); err != nil {
		t.Fatal(err)
	}
	counter.hashed = nil
	cp, err := store.Create(ctx, "two")
	if err != nil {
		t.Fatalf("a stale cache must not fail the checkpoint: %v", err)
	}
	if got := storeContent(t, store, cp.ID, "a.txt"); got != "aaaa\n" {
		t.Fatalf("a.txt = %q", got)
	}
	if len(counter.hashed) != 3 {
		t.Errorf("hashed %q: the retry hashes every file", counter.hashed)
	}
}
