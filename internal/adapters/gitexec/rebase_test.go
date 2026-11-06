// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"testing"
)

// branchBehind returns a repository on "work" (one commit touching file)
// while "base" moved on with its own commit touching baseFile.
func branchBehind(t *testing.T, file, baseFile string) *Client {
	t.Helper()
	c := newRepo(t)
	mustRun(t, c, "switch", "--quiet", "--create", "work")
	write(t, c.Dir, file, "mine\n")
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "mine")
	mustRun(t, c, "switch", "--quiet", "--create", "base", "main")
	write(t, c.Dir, baseFile, "theirs\n")
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "theirs")
	mustRun(t, c, "switch", "--quiet", "work")
	return c
}

func TestRebaseCleanly(t *testing.T) {
	ctx := context.Background()
	c := branchBehind(t, "mine.txt", "theirs.txt")
	write(t, c.Dir, "wip.txt", "draft\n") // local changes are carried
	conflicted, err := c.Rebase(ctx, "base")
	if err != nil || conflicted {
		t.Fatalf("Rebase = %v, %v", conflicted, err)
	}
	if ok, _ := c.IsAncestor(ctx, "base", "HEAD"); !ok {
		t.Fatal("work is not on top of base")
	}
	if status, _ := c.Status(ctx); len(status.Files) != 1 || status.Files[0].Path != "wip.txt" {
		t.Fatalf("local changes lost: %+v", status.Files)
	}
}

func TestRebaseConflictLeavesEverythingAsItWas(t *testing.T) {
	ctx := context.Background()
	c := branchBehind(t, "README.md", "README.md")
	before, _ := c.CommitOf(ctx, "HEAD")
	conflicted, err := c.Rebase(ctx, "base")
	if err != nil || !conflicted {
		t.Fatalf("Rebase = %v, %v", conflicted, err)
	}
	after, _ := c.CommitOf(ctx, "HEAD")
	inProgress, _ := c.OperationInProgress(ctx)
	if after != before || inProgress != "" {
		t.Fatalf("after a conflict: HEAD %s (was %s), operation %q", after, before, inProgress)
	}
}
