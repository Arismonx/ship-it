// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"strings"
	"testing"
)

// target builds a branch "target" from the first commit with one change.
func target(t *testing.T, c *Client, file, content string) {
	t.Helper()
	mustRun(t, c, "switch", "--quiet", "--create", "target")
	write(t, c.Dir, file, content)
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "target change")
	mustRun(t, c, "switch", "--quiet", "main")
}

func TestStashAppliesCleanlyOn(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(t *testing.T, c *Client)
		clean bool
	}{
		{"unrelated change", func(t *testing.T, c *Client) {
			target(t, c, "other.txt", "other\n")
			write(t, c.Dir, "README.md", "hello\nmine\n")
		}, true},
		{"same line changed", func(t *testing.T, c *Client) {
			target(t, c, "README.md", "theirs\n")
			write(t, c.Dir, "README.md", "mine\n")
		}, false},
		{"untracked file collides", func(t *testing.T, c *Client) {
			target(t, c, "new.txt", "theirs\n")
			write(t, c.Dir, "new.txt", "mine\n")
		}, false},
		{"untracked file elsewhere", func(t *testing.T, c *Client) {
			target(t, c, "other.txt", "theirs\n")
			write(t, c.Dir, "draft.txt", "mine\n")
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newRepo(t)
			tt.setup(t, c)
			if err := c.Stash(ctx, "ship-it test"); err != nil {
				t.Fatal(err)
			}
			clean, err := c.StashAppliesCleanlyOn(ctx, "target")
			if err != nil || clean != tt.clean {
				t.Fatalf("StashAppliesCleanlyOn = %v, %v; want %v", clean, err, tt.clean)
			}
			// The check touched nothing: popping on the original commit works.
			if err := c.StashPop(ctx); err != nil {
				t.Fatalf("stash pop after the check: %v", err)
			}
		})
	}
}

// TestDeleteRemoteBranchIsLeased pins the behaviour ship undo relies on:
// git-push(1) does not say whether --force-with-lease guards deletions.
func TestDeleteRemoteBranchIsLeased(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	remote := t.TempDir()
	mustRun(t, c, "init", "--quiet", "--bare", remote)
	mustRun(t, c, "remote", "add", "origin", remote)
	mustRun(t, c, "push", "--quiet", "origin", "main:refs/heads/x")
	first, _ := c.CommitOf(ctx, "HEAD")
	write(t, c.Dir, "b.txt", "b\n")
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "b")
	mustRun(t, c, "push", "--quiet", "--force", "origin", "HEAD:refs/heads/x") // someone moved x
	second, _ := c.CommitOf(ctx, "HEAD")

	if err := c.DeleteRemoteBranch(ctx, "origin", "x", first); err == nil {
		t.Fatal("a stale lease must refuse the deletion")
	}
	if out, _ := c.Run(ctx, "ls-remote", "origin", "refs/heads/x"); len(out) == 0 {
		t.Fatal("x was deleted despite the stale lease")
	}
	if err := c.DeleteRemoteBranch(ctx, "origin", "x", second); err != nil {
		t.Fatal(err)
	}
	if out, _ := c.Run(ctx, "ls-remote", "origin", "refs/heads/x"); len(out) != 0 {
		t.Fatal("x should be gone")
	}
}

// TestPushCommitIsLeased pins what undoing a stack sync relies on: a branch
// goes back only where nobody pushed since, and a deleted one comes back
// only where nobody made it again.
func TestPushCommitIsLeased(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	remote := t.TempDir()
	mustRun(t, c, "init", "--quiet", "--bare", remote)
	mustRun(t, c, "remote", "add", "origin", remote)
	first, _ := c.CommitOf(ctx, "HEAD")
	write(t, c.Dir, "b.txt", "b\n")
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "b")
	second, _ := c.CommitOf(ctx, "HEAD")
	mustRun(t, c, "push", "--quiet", "origin", second+":refs/heads/x")
	tip := func(branch string) string {
		out, _ := c.Run(ctx, "ls-remote", "origin", "refs/heads/"+branch)
		return strings.Fields(string(out) + " ")[0]
	}

	if err := c.PushCommit(ctx, "origin", "x", first, first); err == nil || tip("x") != second {
		t.Fatalf("a stale lease must keep their push: %v, x at %s", err, tip("x"))
	}
	if err := c.PushCommit(ctx, "origin", "x", first, second); err != nil || tip("x") != first {
		t.Fatalf("x back to the first commit: %v, x at %s", err, tip("x"))
	}
	// "": only where the branch does not exist.
	if err := c.PushCommit(ctx, "origin", "x", second, ""); err == nil || tip("x") != first {
		t.Fatalf("an existing branch must be kept: %v", err)
	}
	if err := c.PushCommit(ctx, "origin", "y", second, ""); err != nil || tip("y") != second {
		t.Fatalf("y recreated: %v, at %s", err, tip("y"))
	}
}

func TestBranchHelpers(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	head, err := c.CommitOf(ctx, "HEAD")
	if err != nil || len(head) < 40 {
		t.Fatalf("CommitOf(HEAD) = %q, %v", head, err)
	}
	if missing, err := c.CommitOf(ctx, "refs/heads/nope"); missing != "" || err != nil {
		t.Fatalf("CommitOf(missing) = %q, %v", missing, err)
	}
	if err := c.RecreateBranch(ctx, "feat/x", head); err != nil {
		t.Fatal(err)
	}
	if err := c.RecreateBranch(ctx, "feat/x", head); err == nil {
		t.Fatal("RecreateBranch must refuse an existing branch")
	}
	branches, err := c.LocalBranches(ctx)
	if err != nil || branches["feat/x"] != head || branches["main"] != head {
		t.Fatalf("LocalBranches = %v, %v", branches, err)
	}
	if ok, err := c.IsAncestor(ctx, "main", "feat/x"); !ok || err != nil {
		t.Fatalf("IsAncestor = %v, %v", ok, err)
	}
	write(t, c.Dir, "b.txt", "b\n")
	mustRun(t, c, "add", "--all")
	mustRun(t, c, "commit", "--quiet", "-m", "b")
	if ok, _ := c.IsAncestor(ctx, "main", "feat/x"); ok {
		t.Fatal("main moved ahead: not an ancestor any more")
	}
}
