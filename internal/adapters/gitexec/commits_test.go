// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

func TestCommitsBetween(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	mustRun(t, c, "switch", "--quiet", "-c", "feat/x")
	write(t, c.Dir, "a.txt", "a\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "commit", "--quiet", "-s", "-m", "✨ feat: add a", "-m", "Because b needs it.")
	write(t, c.Dir, "b.txt", "b\n")
	mustRun(t, c, "add", ".")
	mustRun(t, c, "-c", "user.name=Bob", "-c", "user.email=bob@example.invalid", "commit", "--quiet", "-m", "✨ feat: add b")

	commits, err := c.CommitsBetween(ctx, "main", "HEAD")
	if err != nil || len(commits) != 2 {
		t.Fatalf("CommitsBetween = %+v, %v", commits, err)
	}
	first, second := commits[0], commits[1]
	if first.AuthorName != "ship-it test" || first.AuthorEmail != "test@example.invalid" || len(first.Parents) != 1 || len(first.ID) < 40 {
		t.Errorf("first = %+v", first)
	}
	if first.Message != "✨ feat: add a\n\nBecause b needs it.\n\nSigned-off-by: ship-it test <test@example.invalid>" {
		t.Errorf("first message = %q", first.Message)
	}
	if second.AuthorName != "Bob" || second.CommitterEmail != "bob@example.invalid" || second.Message != "✨ feat: add b" {
		t.Errorf("second = %+v", second)
	}
	if convention.CheckSignoff(convention.SignedCommit{AuthorName: first.AuthorName, AuthorEmail: first.AuthorEmail,
		CommitterName: first.CommitterName, CommitterEmail: first.CommitterEmail, Message: first.Message}) != nil {
		t.Error("git commit -s must pass the sign-off check")
	}
	if none, err := c.CommitsBetween(ctx, "HEAD", "HEAD"); err != nil || len(none) != 0 {
		t.Errorf("empty range = %+v, %v", none, err)
	}
}

// TestTrailersMatchGit compares convention.Trailers with git's own parser.
func TestTrailersMatchGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, message := range []string{
		"✨ feat: add a\n\nBecause.",
		"Signed-off-by: A <a@x.org>",
		"fix: b\n\nBody.\n\nSigned-off-by: A <a@x.org>\n",
		"fix: b\n\nCo-Authored-By: C <c@x.org>\nSigned-off-by: A <a@x.org>",
		"fix: b\n\nSigned-off-by: A <a@x.org>\n\nMore text.",
		"fix: b\n\nnot a trailer line\nSigned-off-by: A <a@x.org>",
		"fix: b\n\nline one\nline two\nline three\nline four\nSigned-off-by: A <a@x.org>",
		"fix: b\n\nReviewed-by: R <r@x.org>\nsome prose",
		"fix: b\n\nNote: a long\n  value\nSigned-off-by: A <a@x.org>",
		// Not compared: "https://…" lines inside a block. The documentation says
		// they are not trailers, yet git 2.52's --parse prints them as "https: //…".
		// ship-it follows the documentation; sign-offs are unaffected.
		"fix: b\n\nSigned-off-by : A <a@x.org>",
		"fix: b\n\nSigned-off-by: A <a@x.org>\n\n\n",
		"fix: b\n\n(cherry picked from commit abc)\nprose\nprose\nprose",
	} {
		cmd := exec.Command("git", "interpret-trailers", "--parse", "--no-divider")
		cmd.Stdin = strings.NewReader(message)
		// No user or system configuration: no configured trailer may change the result.
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git interpret-trailers: %v", err)
		}
		var ours []string
		for _, tr := range convention.Trailers(message) {
			ours = append(ours, tr.Key+": "+tr.Value)
		}
		got, want := strings.Join(ours, "\n"), strings.TrimRight(string(out), "\n")
		if got != want {
			t.Errorf("message %q:\n ship-it %q\n git     %q", message, got, want)
		}
	}
}
