// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/ports"
)

func TestFilesAtAndReadFileAt(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	write(t, c.Dir, ".github/PULL_REQUEST_TEMPLATE.md", "## Why\n")
	write(t, c.Dir, ".github/workflows/ci.yml", "on: push\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "commit", "--quiet", "-m", "📝 docs: add a template")
	// The working tree changes; the tree of HEAD is what is read.
	write(t, c.Dir, ".github/PULL_REQUEST_TEMPLATE.md", "## Edited, not committed\n")

	root, err := c.FilesAt(ctx, "HEAD", "")
	if err != nil || !reflect.DeepEqual(root, []string{".github", "README.md"}) {
		t.Fatalf("FilesAt(root) = %q, %v", root, err)
	}
	github, err := c.FilesAt(ctx, "HEAD", ".github")
	if err != nil || !reflect.DeepEqual(github, []string{".github/PULL_REQUEST_TEMPLATE.md", ".github/workflows"}) {
		t.Fatalf("FilesAt(.github) = %q, %v", github, err)
	}
	if none, err := c.FilesAt(ctx, "HEAD", "docs"); err != nil || len(none) != 0 {
		t.Fatalf("FilesAt(docs) = %q, %v", none, err)
	}

	content, err := c.ReadFileAt(ctx, "HEAD", ".github/PULL_REQUEST_TEMPLATE.md", 1<<10)
	if err != nil || string(content) != "## Why\n" {
		t.Fatalf("ReadFileAt = %q, %v", content, err)
	}
	if _, err := c.ReadFileAt(ctx, "HEAD", ".github/PULL_REQUEST_TEMPLATE.md", 3); !errors.Is(err, ports.ErrFileTooLarge) {
		t.Fatalf("over the limit: %v", err)
	}
	if _, err := c.ReadFileAt(ctx, "HEAD", "missing.md", 1<<10); err == nil {
		t.Fatal("a missing file must fail")
	}
}

func TestCommitMessagesSince(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	mustRun(t, c, "switch", "--quiet", "-c", "feat/x")
	write(t, c.Dir, "a.txt", "a\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "commit", "--quiet", "-m", "✨ feat: add a", "-m", "Because b needs it.")
	write(t, c.Dir, "b.txt", "b\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "commit", "--quiet", "-m", "✨ feat: add b")

	messages, err := c.CommitMessagesSince(ctx, "main")
	want := []string{"✨ feat: add a\n\nBecause b needs it.", "✨ feat: add b"}
	if err != nil || !reflect.DeepEqual(messages, want) {
		t.Fatalf("CommitMessagesSince = %q, %v; want %q", messages, err, want)
	}
}

// TestSignedCommits signs a commit with a throwaway SSH key. git's own %G?
// says "N" for it (no gpg.ssh.allowedSignersFile to verify it with), which
// is why SignedCommits reads the gpgsig header instead.
func TestSignedCommits(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed")
	}
	c := newRepo(t)
	ctx := context.Background()
	key := filepath.Join(t.TempDir(), "id")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Skipf("ssh-keygen failed: %v %s", err, out)
	}
	unsigned := revParse(t, c, "HEAD")
	mustRun(t, c, "config", "gpg.format", "ssh")
	mustRun(t, c, "config", "user.signingkey", filepath.ToSlash(key))
	write(t, c.Dir, "a.txt", "a\n")
	mustRun(t, c, "add", ".")
	if _, err := c.Run(ctx, "commit", "--quiet", "-S", "-m", "✨ feat: signed"); err != nil {
		t.Skipf("ssh signing unavailable: %v", err)
	}
	signed := revParse(t, c, "HEAD")
	got, err := c.SignedCommits(ctx, []string{unsigned, signed})
	if err != nil || got[unsigned] || !got[signed] {
		t.Fatalf("SignedCommits = %v, %v", got, err)
	}
	if status, _ := c.Run(ctx, "log", "-1", "--format=%G?"); strings.TrimSpace(string(status)) != "N" {
		t.Logf("git now reports %q for an unverifiable SSH signature", status)
	}
	if none, err := c.SignedCommits(ctx, nil); err != nil || len(none) != 0 {
		t.Fatalf("no commit: %v, %v", none, err)
	}
}

func TestRenameBranch(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	mustRun(t, c, "switch", "--quiet", "-c", "feat/x")
	mustRun(t, c, "config", "branch.feat/x.description", "mine")
	head := revParse(t, c, "HEAD")

	if err := c.RenameBranch(ctx, "feat/x", "feature/x", "0000000000000000000000000000000000000000"); err == nil {
		t.Fatal("a branch that moved must not be renamed")
	}
	if err := c.RenameBranch(ctx, "feat/x", "main", head); err == nil {
		t.Fatal("an existing target must be refused")
	}
	if err := c.RenameBranch(ctx, "feat/x", "feature/x", head); err != nil {
		t.Fatal(err)
	}
	if out, _ := c.Run(ctx, "branch", "--show-current"); strings.TrimSpace(string(out)) != "feature/x" {
		t.Fatalf("HEAD follows the rename, got %q", out)
	}
	if exists, _ := c.LocalBranchExists(ctx, "feat/x"); exists {
		t.Fatal("the old name is gone")
	}
	if value, found, _ := c.ConfigValue(ctx, "branch.feature/x.description"); !found || value != "mine" {
		t.Fatalf("the settings move along: %q %v", value, found)
	}
	// And back, as ship undo does.
	if err := c.RenameBranch(ctx, "feature/x", "feat/x", head); err != nil {
		t.Fatal(err)
	}
}

// TestCommitPatchesShowEveryCommit: what a push sends is each commit, so a
// line added then removed is in the patches, and only the commits missing
// from every excluded ref are listed.
func TestCommitPatchesShowEveryCommit(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	mustRun(t, c, "switch", "--quiet", "-c", "feat/x")
	write(t, c.Dir, "deploy.sh", "export KEY=added-then-removed\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "commit", "--quiet", "-m", "feat: add deploy")
	write(t, c.Dir, "deploy.sh", "export KEY=$KEY\n")
	mustRun(t, c, "commit", "--quiet", "-am", "fix: read the key from the environment")

	patches, err := c.CommitPatches(ctx, []string{"main"})
	if err != nil || !strings.Contains(patches, "+export KEY=added-then-removed") || strings.Contains(patches, "+hello") {
		t.Fatalf("CommitPatches(main) = %q, %v", patches, err)
	}
	all, err := c.CommitPatches(ctx, nil)
	if err != nil || !strings.Contains(all, "+hello") || !strings.Contains(all, "+export KEY=added-then-removed") {
		t.Fatalf("a first publication sends the whole history: %q, %v", all, err)
	}
}

// TestCommitPatchesShowConflictResolutions: a line typed while resolving a
// merge conflict exists only in the merge commit, and is scanned.
func TestCommitPatchesShowConflictResolutions(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	write(t, c.Dir, "config.env", "KEY=base\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "commit", "--quiet", "-m", "chore: add config")
	mustRun(t, c, "switch", "--quiet", "-c", "feat/x")
	write(t, c.Dir, "config.env", "KEY=mine\n")
	mustRun(t, c, "commit", "--quiet", "-am", "feat: mine")
	mustRun(t, c, "switch", "--quiet", "main")
	write(t, c.Dir, "config.env", "KEY=theirs\n")
	mustRun(t, c, "commit", "--quiet", "-am", "feat: theirs")
	mustRun(t, c, "switch", "--quiet", "feat/x")
	if _, err := c.Run(ctx, "merge", "--quiet", "main"); err == nil {
		t.Fatal("the merge must conflict")
	}
	write(t, c.Dir, "config.env", "KEY=resolved-with-a-pasted-secret\n")
	mustRun(t, c, "commit", "--quiet", "-am", "chore: merge main")

	patches, err := c.CommitPatches(ctx, []string{"main"})
	if err != nil || !strings.Contains(patches, "+KEY=resolved-with-a-pasted-secret") {
		t.Fatalf("the resolution must be in the patches: %q, %v", patches, err)
	}
	// And the diff parser the secret scan uses sees it as an added line.
	found := false
	for _, f := range snapshot.ParseUnifiedDiff(patches) {
		for _, l := range f.Added {
			found = found || (f.Path == "config.env" && l.Text == "KEY=resolved-with-a-pasted-secret")
		}
	}
	if !found {
		t.Fatalf("ParseUnifiedDiff misses the resolution in %q", patches)
	}
}
