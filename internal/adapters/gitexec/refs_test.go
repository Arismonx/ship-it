// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func revParse(t *testing.T, c *Client, rev string) string {
	t.Helper()
	out, err := c.Run(context.Background(), "rev-parse", rev)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestRefMovesLeaveTheWorkingTreeAlone(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	h0 := revParse(t, c, "HEAD")
	write(t, c.Dir, "a.txt", "work\n")
	if err := c.SwitchToNewBranch(ctx, "feat/a"); err != nil {
		t.Fatal(err)
	}
	if err := c.StageAll(ctx); err != nil {
		t.Fatal(err)
	}
	c1, err := c.Commit(ctx, "✨ feat: add a\n", ports.CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, c, "config", "branch.feat/a.remote", "origin") // as a push -u leaves it

	// A compare-and-swap on a stale value must fail and change nothing.
	if err := c.DeleteBranch(ctx, "feat/a", h0); err == nil {
		t.Fatal("DeleteBranch accepted a stale expected value")
	}
	if err := c.MoveBranch(ctx, "feat/a", h0, h0); err == nil {
		t.Fatal("MoveBranch accepted a stale expected value")
	}

	if err := c.AttachHead(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if err := c.ResetIndex(ctx, h0); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteBranch(ctx, "feat/a", c1); err != nil {
		t.Fatal(err)
	}
	status, err := c.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Branch.Name != "main" || len(status.Files) != 1 || status.Files[0].Path != "a.txt" {
		t.Fatalf("status = %+v, want a.txt untracked on main", status)
	}
	if exists, _ := c.LocalBranchExists(ctx, "feat/a"); exists {
		t.Fatal("feat/a still exists")
	}
	if _, found, _ := c.ConfigValue(ctx, "branch.feat/a.remote"); found {
		t.Fatal("the settings of the deleted branch must go too")
	}
	if content, _ := os.ReadFile(filepath.Join(c.Dir, "a.txt")); string(content) != "work\n" {
		t.Fatalf("a.txt = %q, the working tree must not change", content)
	}

	if err := c.DetachHead(ctx, h0); err != nil {
		t.Fatal(err)
	}
	if status, _ := c.Status(ctx); !status.Branch.Detached {
		t.Fatal("HEAD is not detached")
	}
}

func TestResetIndexToEmpty(t *testing.T) {
	ctx := context.Background()
	c := newRepo(t)
	if err := c.ResetIndex(ctx, ""); err != nil {
		t.Fatal(err)
	}
	out, _ := c.Run(ctx, "ls-files")
	if len(out) != 0 {
		t.Fatalf("index still lists %q", out)
	}
}
