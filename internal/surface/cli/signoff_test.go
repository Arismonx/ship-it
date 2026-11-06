// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func signed(id, message string) ports.CommitRecord {
	return ports.CommitRecord{ID: id, Parents: []string{"p"}, AuthorName: "Ada", AuthorEmail: "ada@example.org",
		CommitterName: "Ada", CommitterEmail: "ada@example.org", Message: message}
}

func TestCheckSignoff(t *testing.T) {
	var asked string
	git := testkit.HealthyRepository("feat/x")
	git.DefaultBranch = "main"
	git.RangeAsked = &asked
	git.Range = []ports.CommitRecord{
		signed("1111111111111111", "✨ feat: a\n\nSigned-off-by: Ada <ada@example.org>"),
		{ID: "m", Parents: []string{"a", "b"}, Message: "Merge branch 'main'"},
	}
	code, _, stderr := runWith(git, "", false, "check", "signoff")
	if code != ExitOK || asked != "origin/main..HEAD" || !strings.Contains(stderr, "1 commit signed off (origin/main..HEAD)") {
		t.Fatalf("exit %d, range %q, stderr %q", code, asked, stderr)
	}

	git.Range = append(git.Range,
		signed("2222222222222222", "🐛 fix: b"),
		signed("3333333333333333", "🐛 fix: c\n\nSigned-off-by: Bob <bob@example.org>"))
	code, _, stderr = runWith(git, "", false, "check", "signoff", "--from", "v1.0.0", "--to", "feat/x")
	if code != ExitError || asked != "v1.0.0..feat/x" {
		t.Fatalf("exit %d, range %q", code, asked)
	}
	for _, want := range []string{
		"✗ 222222222222 🐛 fix: b", "SHIP-E0114", "✗ 333333333333 🐛 fix: c", "SHIP-E0115", `found Bob <bob@example.org>`,
		"git rebase --signoff v1.0.0",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr misses %q:\n%s", want, stderr)
		}
	}

	code, stdout, _ := runWith(git, "", false, "check", "signoff", "--json")
	var result struct {
		Checked  int `json:"checked"`
		Exempt   int `json:"exempt"`
		Failures []struct {
			Commit string `json:"commit"`
		} `json:"failures"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil || code != ExitError ||
		result.Checked != 4 || result.Exempt != 1 || len(result.Failures) != 2 {
		t.Fatalf("json: exit %d, %+v, %v", code, result, err)
	}
}

func TestCheckSignoffWithoutBase(t *testing.T) {
	git := testkit.HealthyRepository("feat/x")
	git.DefaultBranch = ""
	code, _, stderr := runWith(git, "", false, "check", "signoff")
	if code != ExitError || !strings.Contains(stderr, "SHIP-E0116") || !strings.Contains(stderr, "--from <base>") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if code, _, _ := runWith(git, "", false, "check", "signoff", "extra"); code != ExitUsage {
		t.Fatalf("extra argument: exit %d", code)
	}
}
