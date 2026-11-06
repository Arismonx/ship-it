// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"testing"
)

// TestRecentSubjects: newest first, at most limit, and nothing (no error)
// before the first commit, in one git process when there are commits.
func TestRecentSubjects(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	write(t, c.Dir, "a.go", "package a\n")
	mustRun(t, c, "add", "a.go")
	mustRun(t, c, "commit", "--quiet", "-m", "✨ feat: a")

	var journal bytes.Buffer
	c.Log = slog.New(slog.NewTextHandler(&journal, &slog.HandlerOptions{Level: slog.LevelDebug}))
	subjects, err := c.RecentSubjects(ctx, 5)
	if err != nil || !slices.Equal(subjects, []string{"✨ feat: a", "🎉 chore: init"}) {
		t.Fatalf("subjects = %q, %v", subjects, err)
	}
	if runs := bytes.Count(journal.Bytes(), []byte("msg=git.exec")); runs != 1 {
		t.Errorf("%d git processes, want 1", runs)
	}
	if subjects, _ := c.RecentSubjects(ctx, 1); len(subjects) != 1 {
		t.Errorf("limit 1: %q", subjects)
	}

	unborn := New(t.TempDir())
	mustRun(t, unborn, "init", "--quiet")
	if subjects, err := unborn.RecentSubjects(ctx, 5); err != nil || subjects != nil {
		t.Errorf("unborn: %q, %v", subjects, err)
	}
}
