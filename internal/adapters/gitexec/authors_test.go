// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRecentAuthors: #46. The authors of the commits that changed the
// paths, newest first; paths are literal, so "*.go" names no file.
func TestRecentAuthors(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	commit := func(name, email, file string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(c.Dir, file), []byte(name+email), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", file}, {"-c", "user.name=" + name, "-c", "user.email=" + email, "commit", "--quiet", "-m", "change " + file}} {
			if _, err := c.Run(ctx, args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	commit("Ada", "ada@example.com", "vat.go")
	commit("Bob", "bob@example.com", "other.go")
	commit("Cy", "cy@example.com", "vat.go")

	got, err := c.RecentAuthors(ctx, "HEAD", "6 months ago", []string{"vat.go"})
	if err != nil || len(got) != 2 || got[0].Name != "Cy" || got[1].Email != "ada@example.com" || len(got[0].SHA) < 40 {
		t.Fatalf("RecentAuthors = %+v, %v", got, err)
	}
	if got, err := c.RecentAuthors(ctx, "HEAD", "6 months ago", []string{"*.go"}); err != nil || len(got) != 0 {
		t.Fatalf("a pattern is not a path: %+v, %v", got, err)
	}
}
