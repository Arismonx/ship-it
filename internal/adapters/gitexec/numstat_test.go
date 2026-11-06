// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func TestParseNumstat(t *testing.T) {
	out := "3\t1\tsrc/a.go\x00-\t-\tlogo.png\x00\n0\t2\tdossier é/b.md\x00"
	got, err := ParseNumstat([]byte(out))
	want := []ports.FileStat{
		{Path: "src/a.go", Added: 3, Deleted: 1},
		{Path: "logo.png", Binary: true},
		{Path: "dossier é/b.md", Deleted: 2},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseNumstat = %+v, %v", got, err)
	}
	if _, err := ParseNumstat([]byte("x\ty\tz\x00")); err == nil {
		t.Fatal("non-numeric counts must be rejected")
	}
}

func FuzzParseNumstat(f *testing.F) {
	f.Add([]byte("3\t1\ta.go\x00-\t-\tb.png\x00"))
	f.Add([]byte("\t\t\x00"))
	f.Fuzz(func(t *testing.T, out []byte) {
		stats, err := ParseNumstat(out)
		for _, s := range stats {
			if err == nil && s.Path == "" {
				t.Fatalf("empty path accepted from %q", out)
			}
		}
	})
}

func TestDiffStatAndGeneratedPathsOnARealRepository(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	base, err := c.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	write(t, c.Dir, ".gitattributes", "api/*.pb.go linguist-generated\nvendor/** linguist-generated=true\n")
	write(t, c.Dir, "api/x.pb.go", "package api\n")
	write(t, c.Dir, "vendor/lib/y.go", "package lib\n")
	write(t, c.Dir, "main.go", "package main\n\nfunc main() {}\n")
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "feat: add files")

	stats, err := c.DiffStatSince(ctx, string(base[:len(base)-1]))
	if err != nil || len(stats) != 4 {
		t.Fatalf("DiffStatSince = %+v, %v", stats, err)
	}
	generated, err := c.GeneratedPaths(ctx, []string{"api/x.pb.go", "vendor/lib/y.go", "main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !generated["api/x.pb.go"] || !generated["vendor/lib/y.go"] || generated["main.go"] {
		t.Fatalf("GeneratedPaths = %v", generated)
	}
	if root, err := c.DiffStatSince(ctx, ""); err != nil || len(root) != 4 {
		t.Fatalf("DiffStatSince(HEAD only) = %+v, %v", root, err)
	}
}
