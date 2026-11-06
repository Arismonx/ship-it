// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func TestCommitSignsOffWithTheCommitter(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	write(t, c.Dir, "a.txt", "a\n")
	mustRun(t, c, "add", "a.txt")
	if _, err := c.Commit(ctx, "✨ feat: add a\n", ports.CommitOptions{Signoff: true}); err != nil {
		t.Fatal(err)
	}
	messages, err := c.RecentMessages(ctx, 5)
	if err != nil || len(messages) != 2 || !strings.HasSuffix(messages[0], "Signed-off-by: ship-it test <test@example.invalid>") {
		t.Fatalf("RecentMessages = %q, %v", messages, err)
	}
	write(t, c.Dir, "b.txt", "b\n")
	if _, err := c.CommitPaths(ctx, "✨ feat: add b\n", []string{"b.txt"}, ports.CommitOptions{Signoff: true}); err != nil {
		t.Fatal(err)
	}
	if messages, _ := c.RecentMessages(ctx, 1); !strings.Contains(messages[0], "Signed-off-by:") {
		t.Errorf("CommitPaths without the sign-off: %q", messages)
	}
}

// brokenSigner makes every signature fail: a commit that tries to sign
// fails, one that does not sign passes.
func brokenSigner(t *testing.T, c *Client) {
	t.Helper()
	mustRun(t, c, "config", "commit.gpgsign", "true")
	mustRun(t, c, "config", "gpg.format", "openpgp")
	mustRun(t, c, "config", "gpg.program", "ship-it-no-such-gpg")
}

func TestSignModes(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	brokenSigner(t, c)
	write(t, c.Dir, "a.txt", "a\n")
	mustRun(t, c, "add", "a.txt")
	if _, err := c.Commit(ctx, "✨ feat: add a\n", ports.CommitOptions{}); err == nil {
		t.Fatal("the default follows commit.gpgSign: the broken signer must fail")
	}
	id, err := c.Commit(ctx, "✨ feat: add a\n", ports.CommitOptions{Sign: ports.SignNever})
	if err != nil {
		t.Fatalf("--no-gpg-sign must not call the signer: %v", err)
	}
	if signed, _ := c.SignedCommits(ctx, []string{id}); signed[id] {
		t.Error("the commit is signed")
	}
	mustRun(t, c, "config", "commit.gpgsign", "false")
	write(t, c.Dir, "b.txt", "b\n")
	mustRun(t, c, "add", "b.txt")
	if _, err := c.Commit(ctx, "✨ feat: add b\n", ports.CommitOptions{Sign: ports.SignAlways}); err == nil {
		t.Fatal("-S must call the signer even when commit.gpgSign is false")
	}
}

func TestMergeCleanly(t *testing.T) {
	ctx := context.Background()
	c := branchBehind(t, "mine.txt", "theirs.txt")
	brokenSigner(t, c)
	write(t, c.Dir, "wip.txt", "draft\n")
	conflicted, err := c.Merge(ctx, "base", ports.CommitOptions{Sign: ports.SignNever})
	if err != nil || conflicted {
		t.Fatalf("Merge = %v, %v", conflicted, err)
	}
	if ok, _ := c.IsAncestor(ctx, "base", "HEAD"); !ok {
		t.Fatal("base is not merged")
	}
	if parents, _ := c.Run(ctx, "rev-list", "--parents", "-n", "1", "HEAD"); len(strings.Fields(string(parents))) != 3 {
		t.Errorf("not a merge commit: %s", parents)
	}
	if status, _ := c.Status(ctx); len(status.Files) != 1 || status.Files[0].Path != "wip.txt" {
		t.Fatalf("local changes lost: %+v", status.Files)
	}
}

func TestMergeConflictLeavesEverythingAsItWas(t *testing.T) {
	ctx := context.Background()
	c := branchBehind(t, "README.md", "README.md")
	before, _ := c.CommitOf(ctx, "HEAD")
	conflicted, err := c.Merge(ctx, "base", ports.CommitOptions{})
	if err != nil || !conflicted {
		t.Fatalf("Merge = %v, %v", conflicted, err)
	}
	after, _ := c.CommitOf(ctx, "HEAD")
	inProgress, _ := c.OperationInProgress(ctx)
	if after != before || inProgress != "" {
		t.Fatalf("after a conflict: HEAD %s (was %s), operation %q", after, before, inProgress)
	}
}

func TestRebaseWithoutSigning(t *testing.T) {
	ctx := context.Background()
	c := branchBehind(t, "mine.txt", "theirs.txt")
	brokenSigner(t, c)
	if conflicted, err := c.RebaseWith(ctx, "base", ports.CommitOptions{Sign: ports.SignNever}); err != nil || conflicted {
		t.Fatalf("RebaseWith = %v, %v", conflicted, err)
	}
	if ok, _ := c.IsAncestor(ctx, "base", "HEAD"); !ok {
		t.Fatal("work is not on top of base")
	}
}
