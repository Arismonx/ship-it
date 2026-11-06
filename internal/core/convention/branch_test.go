// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"strings"
	"testing"
)

// Examples published by Conventional Branch 1.1.0 (conventionalbranch.org).
func TestValidateBranchSpecExamples(t *testing.T) {
	valid := []string{
		"feature/add-login-page", "fix/header-bug", "release/v1.2.0",
		"chore/upgrade-node-v20.11", "claude/security-patch", "main",
		"feat/123-add-login", "hotfix/x", "codex/a.b-c",
	}
	invalid := []string{
		"Feature/Add-Login", "fix/header bug", "fix/header_bug", "feature/new--login",
		"feature/-new-login", "release/v1.-2.0", "unknown/some-task",
		"feat/", "feat", "feat/x.lock", "feat/a..b", "feat/a/b",
	}
	for _, b := range valid {
		if err := ValidateBranch(b); err != nil {
			t.Errorf("ValidateBranch(%q) = %v, want valid", b, err)
		}
	}
	for _, b := range invalid {
		if ValidateBranch(b) == nil {
			t.Errorf("ValidateBranch(%q) = nil, want error", b)
		}
	}
}

func TestBranchName(t *testing.T) {
	tests := []struct {
		commitType CommitType
		subject    string
		opts       BranchOptions
		want       string
	}{
		{Feature, "add login page", BranchOptions{}, "feat/add-login-page"},
		{Feature, "add the login page for users", BranchOptions{Issue: 123}, "feat/123-add-login-page-users"},
		{Documentation, "update README", BranchOptions{}, "chore/update-readme"},
		{SecurityFix, "rotate leaked token", BranchOptions{}, "fix/rotate-leaked-token"},
		{BugFix, "Corriger l'été déjà cassé", BranchOptions{}, "fix/corriger-ete-deja-casse"},
		{BugFix, "handle nil", BranchOptions{Issue: 7, Agent: ClaudeAgent}, "claude/7-handle-nil"},
		{BugFix, "crash on startup", BranchOptions{Hotfix: true}, "hotfix/crash-startup"},
		{Feature, "crash on startup", BranchOptions{Hotfix: true}, "feat/crash-startup"},
		{BugFix, "fix crash on startup", BranchOptions{}, "fix/crash-startup"},
		{BugFix, "fix it", BranchOptions{}, "fix/fix"},
		{Feature, "!!!", BranchOptions{}, "feat/work"},
		{Feature, "support stacked pull requests natively on github", BranchOptions{}, "feat/support-stacked-pull-requests"},
	}
	for _, tt := range tests {
		got := BranchName(tt.commitType, tt.subject, tt.opts)
		if got != tt.want {
			t.Errorf("BranchName(%s, %q, %+v) = %q, want %q", tt.commitType, tt.subject, tt.opts, got, tt.want)
		}
		if err := ValidateBranch(got); err != nil {
			t.Errorf("BranchName produced invalid branch %q: %v", got, err)
		}
	}
}

func TestBranchDescriptionIsShort(t *testing.T) {
	got := BranchDescription("implement extraordinarily comprehensive internationalisation configuration")
	if len(got) > DefaultBranchMaxLength {
		t.Fatalf("BranchDescription() = %q (%d bytes), max %d", got, len(got), DefaultBranchMaxLength)
	}
	if n := strings.Count(got, "-") + 1; n > DefaultBranchMaxWords {
		t.Fatalf("BranchDescription() = %q has %d words, max %d", got, n, DefaultBranchMaxWords)
	}
}

func TestUniqueBranchName(t *testing.T) {
	existing := map[string]bool{"feat/login": true, "feat/login-2": true}
	if got := UniqueBranchName("feat/login", func(n string) bool { return existing[n] }); got != "feat/login-3" {
		t.Fatalf("UniqueBranchName() = %q, want feat/login-3", got)
	}
	if got := UniqueBranchName("feat/other", func(n string) bool { return existing[n] }); got != "feat/other" {
		t.Fatalf("UniqueBranchName() = %q, want feat/other", got)
	}
}

// FuzzBranchName checks that generated branch names are always valid and short.
func TestFrequentScopes(t *testing.T) {
	subjects := []string{"feat(api): a", "fix(api): b", "feat(ui): c", "docs(ui): d", "chore(ci): e", "random", "fix(api): f"}
	if got := FrequentScopes(subjects, 2); strings.Join(got, ",") != "api,ui" {
		t.Fatalf("FrequentScopes = %v", got)
	}
}

func TestIssueFromBranch(t *testing.T) {
	for name, want := range map[string]int{
		"feat/123-add-login": 123,
		"fix/7":              7,
		"feat/add-login":     0,
		"feat/0-x":           0,
		"feat/012-x":         0,
		"main":               0,
		"claude/42-x":        42,
	} {
		if got := IssueFromBranch(name); got != want {
			t.Errorf("IssueFromBranch(%q) = %d, want %d", name, got, want)
		}
	}
}

func FuzzBranchName(f *testing.F) {
	for _, s := range []string{"add login", "", "--", "Été 2026 !", "a.lock", strings.Repeat("x ", 50), strings.Repeat("y", 80)} {
		f.Add(s, 0, false)
	}
	f.Fuzz(func(t *testing.T, subject string, issue int, hotfix bool) {
		if issue < 0 || issue > 1_000_000 {
			issue = 0
		}
		for _, spec := range CommitTypes() {
			name := BranchName(spec.Type, subject, BranchOptions{Issue: issue, Hotfix: hotfix})
			if err := ValidateBranch(name); err != nil {
				t.Fatalf("BranchName(%s, %q) = %q is invalid: %v", spec.Type, subject, name, err)
			}
			_, description, _ := strings.Cut(name, "/")
			if len(description) > DefaultBranchMaxLength+len("1000000-") {
				t.Fatalf("BranchName(%s, %q) = %q is too long", spec.Type, subject, name)
			}
		}
	})
}
