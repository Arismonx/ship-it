// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
)

// packRepository returns a repository with at least packThreshold files to
// checkpoint, so that their blobs are written with fast-import.
func packRepository(t *testing.T) (*gitexec.Client, *hashCounter, map[string]string) {
	t.Helper()
	c := newRepo(t)
	files := map[string]string{
		".gitignore": "*.log\n",
		"app.txt":    "v1\n",
	}
	for i := 0; len(files) < packThreshold+10; i++ {
		files[fmt.Sprintf("src/file%03d.go", i)] = fmt.Sprintf("package src\n\nconst N%d = %d\n", i, i)
	}
	for name, content := range files {
		write(t, c.Dir, name, content)
	}
	return c, &hashCounter{GitRunner: c}, files
}

// TestImportIsByteExact: blobs written by fast-import have the id
// `git hash-object --no-filters` gives, whatever core.autocrlf and
// .gitattributes say, and restore the files byte for byte.
func TestImportIsByteExact(t *testing.T) {
	c, counter, files := packRepository(t)
	dir := c.Dir
	run(t, c, "config", "core.autocrlf", "true")
	special := map[string]string{
		".gitattributes":  "* text=auto\n*.crlf text eol=crlf\n*.lf text eol=lf\n",
		"crlf.txt":        "line one\r\nline two\r\n",
		"mixed.txt":       "line one\r\nline two\nline three\r\n",
		"rules.crlf":      "lf only\nhere\n",
		"rules.lf":        "crlf here\r\nand here\r\n",
		"no-newline.txt":  "no trailing newline",
		"empty.txt":       "",
		"binary.bin":      string([]byte{0x00, 0xFF, '\r', '\n', 0x10, '\n', 'd', 'a', 't', 'a', ' ', '1', '\n'}),
		"fake-stream.txt": "blob\nmark :1\ndata 3\nabc\ndone\n",
	}
	for name, content := range special {
		write(t, dir, name, content)
		files[name] = content
	}
	link := false
	if err := os.Symlink("src/file000.go", filepath.Join(dir, "link.go")); err == nil {
		link = true
	}

	store := New(counter)
	ctx := context.Background()
	cp, err := store.Create(ctx, "pack")
	if err != nil {
		t.Fatal(err)
	}
	if counter.imports != 1 || len(counter.hashed) != 0 {
		t.Fatalf("fast-import runs = %d, hash-object paths = %d: want one import of every file", counter.imports, len(counter.hashed))
	}
	if packs, _ := filepath.Glob(filepath.Join(c.Dir, ".git", "ship", "store.git", "objects", "pack", "*.pack")); len(packs) == 0 {
		t.Error("the blobs were not written as a pack")
	}

	listing := inStore(t, c, "ls-tree", "-r", "-z", cp.ID)
	checked := 0
	for _, record := range strings.Split(listing, "\x00") {
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			continue
		}
		if fields[0] == modeSymlink {
			continue // hashed from its target below
		}
		want := strings.TrimSpace(run(t, c, "hash-object", "--no-filters", "--", path))
		if fields[2] != want {
			t.Errorf("%s: blob %s, hash-object --no-filters gives %s", path, fields[2], want)
		}
		checked++
	}
	if checked != len(files) {
		t.Errorf("checked %d blobs, want %d", checked, len(files))
	}
	// The check is meaningful: with filters, these files hash differently.
	for _, name := range []string{"crlf.txt", "rules.lf"} {
		filtered := strings.TrimSpace(run(t, c, "hash-object", "--", name))
		raw := strings.TrimSpace(run(t, c, "hash-object", "--no-filters", "--", name))
		if filtered == raw {
			t.Errorf("%s: the filters leave it unchanged, the test proves nothing", name)
		}
	}
	if link {
		if got := inStore(t, c, "cat-file", "blob", cp.ID+":link.go"); got != "src/file000.go" {
			t.Errorf("link.go blob = %q, want its target", got)
		}
	}

	for name := range files {
		write(t, dir, name, "overwritten\n")
	}
	if err := store.RestoreFiles(ctx, cp.ID); err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		if got := read(t, dir, name); got != want {
			t.Errorf("%s restored as %q, want %q", name, got, want)
		}
	}
}

// TestCacheAfterImport: the blobs written by fast-import enter the stat
// cache, so the next checkpoint hashes only the changed file.
func TestCacheAfterImport(t *testing.T) {
	c, counter, files := packRepository(t)
	dir := c.Dir
	var names []string
	for name := range files {
		names = append(names, name)
	}
	age(t, dir, names...)
	store := New(counter)
	ctx := context.Background()
	if _, err := store.Create(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if counter.imports != 1 {
		t.Fatalf("first checkpoint: %d fast-import runs, want 1", counter.imports)
	}
	write(t, dir, "src/file007.go", "package src // changed\n")
	age(t, dir, "src/file007.go")
	cp, err := store.Create(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	if counter.imports != 1 || strings.Join(counter.hashed, " ") != "src/file007.go" {
		t.Errorf("second checkpoint: %d imports, hashed %q; want only src/file007.go hashed", counter.imports, counter.hashed)
	}
	if got := storeContent(t, store, cp.ID, "src/file007.go"); got != "package src // changed\n" {
		t.Errorf("changed file = %q", got)
	}
	if got := storeContent(t, store, cp.ID, "src/file001.go"); got != files["src/file001.go"] {
		t.Errorf("cached file = %q", got)
	}
}

func TestReadMarksRejectsAnIncompleteTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marks")
	for name, table := range map[string]string{
		"missing":   ":1 aaaa\n",
		"duplicate": ":1 aaaa\n:1 bbbb\n",
		"unknown":   ":1 aaaa\n:3 bbbb\n",
	} {
		if err := os.WriteFile(path, []byte(table), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readMarks(path, 2); err == nil {
			t.Errorf("%s: readMarks accepted %q", name, table)
		}
	}
	if err := os.WriteFile(path, []byte(":2 bbbb\n:1 aaaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ids, err := readMarks(path, 2); err != nil || strings.Join(ids, " ") != "aaaa bbbb" {
		t.Errorf("readMarks = %q, %v", ids, err)
	}
}

// TestImportFailsOnAVanishedFile: a file deleted between the listing and
// its reading fails the import (--done: a cut stream is refused) instead of
// hanging or storing part of the snapshot.
func TestImportFailsOnAVanishedFile(t *testing.T) {
	c, _, files := packRepository(t)
	store := New(c)
	ctx := context.Background()
	loc, _, err := store.locate(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	var entries []fileEntry
	for name := range files {
		entries = append(entries, fileEntry{path: name, mode: modeRegular})
	}
	entries = append(entries, fileEntry{path: "vanished.txt", mode: modeRegular})
	if _, err := store.importEntries(ctx, loc, entries); err == nil {
		t.Fatal("importEntries succeeded without vanished.txt")
	}
	if marks, _ := filepath.Glob(filepath.Join(loc.store, "*.marks")); len(marks) != 0 {
		t.Errorf("marks file left behind: %v", marks)
	}
}
