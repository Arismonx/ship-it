// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package rules

import "testing"

func TestCheckEveryOperatorAndNegate(t *testing.T) {
	tests := []struct {
		name    string
		pattern Pattern
		value   string
		want    Evaluation
	}{
		{"starts with", Pattern{Operator: StartsWith, Pattern: "feature/"}, "feature/login", Satisfied},
		{"starts with, missing", Pattern{Operator: StartsWith, Pattern: "feature/"}, "feat/login", Violated},
		{"starts with is case sensitive", Pattern{Operator: StartsWith, Pattern: "ABC-"}, "abc-1", Violated},
		{"starts with, no wildcard", Pattern{Operator: StartsWith, Pattern: "feat/*"}, "feat/login", Violated},
		{"must not start with", Pattern{Operator: StartsWith, Pattern: "wip/", Negate: true}, "wip/x", Violated},
		{"must not start with, fine", Pattern{Operator: StartsWith, Pattern: "wip/", Negate: true}, "feat/x", Satisfied},
		{"ends with", Pattern{Operator: EndsWith, Pattern: "-ok"}, "feat/x-ok", Satisfied},
		{"ends with, missing", Pattern{Operator: EndsWith, Pattern: "-ok"}, "feat/x", Violated},
		{"must not end with", Pattern{Operator: EndsWith, Pattern: ".tmp", Negate: true}, "feat/x.tmp", Violated},
		{"contains", Pattern{Operator: Contains, Pattern: "JIRA"}, "feat/JIRA-1", Satisfied},
		{"contains, missing", Pattern{Operator: Contains, Pattern: "JIRA"}, "feat/x", Violated},
		{"must not contain", Pattern{Operator: Contains, Pattern: "fixup!", Negate: true}, "fixup! feat: x\n", Violated},
		{"regex", Pattern{Operator: Regex, Pattern: `^(feat|fix)/`}, "fix/crash", Satisfied},
		{"regex, no match", Pattern{Operator: Regex, Pattern: `^(feat|fix)/`}, "chore/x", Violated},
		{"must not match regex", Pattern{Operator: Regex, Pattern: `(?i)wip`, Negate: true}, "feat/WIP-x", Violated},
		// GitHub: without (?m), ^ and $ apply to the whole message.
		{"regex without (?m)", Pattern{Operator: Regex, Pattern: `^Refs: #[0-9]+$`}, "feat: x\n\nRefs: #12\n", Violated},
		{"regex with (?m)", Pattern{Operator: Regex, Pattern: `(?m)^Refs: #[0-9]+$`}, "feat: x\n\nRefs: #12\n", Satisfied},
		// GitHub's own example for a first line of at most 50 characters.
		{"documented \\n?$ anchor", Pattern{Operator: Regex, Pattern: `\A.{1,50}\n?$`}, "feat: x\n", Satisfied},
		{"negative lookahead is not RE2", Pattern{Operator: Regex, Pattern: `^(?!wip)`}, "feat/x", Unknown},
		{"unknown operator", Pattern{Operator: "glob", Pattern: "*"}, "x", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pattern.Check(tt.value); got != tt.want {
				t.Fatalf("Check(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestDescribeQuotesTheRule(t *testing.T) {
	if got := (Pattern{Operator: StartsWith, Pattern: "feature/"}).Describe(); got != "must start with feature/" {
		t.Fatal(got)
	}
	if got := (Pattern{Operator: Regex, Pattern: "^x", Negate: true}).Describe(); got != "must not match ^x" {
		t.Fatal(got)
	}
	if got := (Pattern{Name: "Jira key", Pattern: "x"}).Label(); got != "Jira key" {
		t.Fatal(got)
	}
	if got := (Pattern{Pattern: "x"}).Label(); got != "x" {
		t.Fatal(got)
	}
}

func TestCompliantBranch(t *testing.T) {
	tests := []struct {
		name     string
		branch   string
		patterns []Pattern
		want     string
		found    bool
	}{
		{"no rule", "feat/12-login", nil, "feat/12-login", true},
		{"already compliant", "feat/12-login", []Pattern{{Operator: StartsWith, Pattern: "feat/"}}, "feat/12-login", true},
		{"long alias keeps Conventional Branch", "feat/12-login", []Pattern{{Operator: StartsWith, Pattern: "feature/"}}, "feature/12-login", true},
		{"required prefix replaces ship's", "chore/bump-go", []Pattern{{Operator: StartsWith, Pattern: "team-a/"}}, "team-a/bump-go", true},
		{"required prefix without slash is prepended", "feat/login", []Pattern{{Operator: StartsWith, Pattern: "x-"}}, "x-feat/login", true},
		{"required suffix", "feat/login", []Pattern{{Operator: EndsWith, Pattern: "-v2"}}, "feat/login-v2", true},
		{"prefix and suffix together", "feat/login", []Pattern{{Operator: StartsWith, Pattern: "team/"}, {Operator: EndsWith, Pattern: "-x"}}, "team/login-x", true},
		{"regex wants no prefix", "chore/bump-go", []Pattern{{Operator: Regex, Pattern: `^[a-z0-9-]+$`}}, "bump-go", true},
		{"regex wants an upper-case ticket", "feat/abc-123-login", []Pattern{{Operator: Regex, Pattern: `^feat/[A-Z]+-[0-9]+-`}}, "feat/ABC-123-login", true},
		{"regex wants the alias", "fix/crash", []Pattern{{Operator: Regex, Pattern: `^(feature|bugfix)/`}}, "bugfix/crash", true},
		{"must not start with: prefix dropped", "feat/login", []Pattern{{Operator: StartsWith, Pattern: "feat/", Negate: true}}, "feature/login", true},
		{"contains is never invented", "feat/login", []Pattern{{Operator: Contains, Pattern: "JIRA"}}, "", false},
		{"regex nothing derives", "feat/login", []Pattern{{Operator: Regex, Pattern: `^[A-Z]+-[0-9]+$`}}, "", false},
		{"unevaluable regex is left to GitHub", "feat/login", []Pattern{{Operator: Regex, Pattern: `^(?=x)`}}, "feat/login", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := CompliantBranch(tt.branch, tt.patterns)
			if got != tt.want || found != tt.found {
				t.Fatalf("CompliantBranch(%q) = %q, %v; want %q, %v", tt.branch, got, found, tt.want, tt.found)
			}
		})
	}
}

func TestValidRefName(t *testing.T) {
	for name, want := range map[string]bool{
		"feat/x": true, "ABC-1": true, "": false, "a b": false, "a..b": false, "/a": false, "a/": false,
		"a//b": false, "a/.b": false, "a.lock": false, "a.": false, "@": false, "a@{b": false, "a~1": false, "-a": false,
	} {
		if got := ValidRefName(name); got != want {
			t.Errorf("ValidRefName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCheckCommits(t *testing.T) {
	commits := []Commit{
		{ID: "a1", Message: "✨ feat: add login", AuthorEmail: "ada@acme.example", CommitterEmail: "ada@acme.example"},
		{ID: "b2", Message: "🐛 fix: crash", AuthorEmail: "ada@gmail.example", CommitterEmail: "ada@acme.example", Unsigned: true},
	}
	pattern := func(t Type, op Operator, p string, negate bool) Rule {
		return Rule{Type: t, Pattern: &Pattern{Operator: op, Pattern: p, Negate: negate}}
	}
	tests := []struct {
		name   string
		rules  []Rule
		want   Type
		commit string
	}{
		{"nothing applies", []Rule{{Type: RequiredLinearHistory}}, "", ""},
		{"message regex", []Rule{pattern(CommitMessagePattern, Regex, `^(feat|fix): `, false)}, CommitMessagePattern, "a1"},
		{"message accepted", []Rule{pattern(CommitMessagePattern, Contains, ": ", false)}, "", ""},
		{"author email", []Rule{pattern(CommitAuthorEmailPattern, EndsWith, "@acme.example", false)}, CommitAuthorEmailPattern, "b2"},
		{"committer email negated", []Rule{pattern(CommitterEmailPattern, Contains, "acme", true)}, CommitterEmailPattern, "a1"},
		{"signatures", []Rule{{Type: RequiredSignatures}}, RequiredSignatures, "b2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, broken := CheckCommits(tt.rules, commits)
			if broken != (tt.want != "") || v.Type != tt.want || v.Commit.ID != tt.commit {
				t.Fatalf("CheckCommits = %+v, %v; want %s on %s", v, broken, tt.want, tt.commit)
			}
		})
	}
}

// FuzzCompliantBranch: whatever the rule, a proposed name is a valid branch
// name that breaks no rule.
func FuzzCompliantBranch(f *testing.F) {
	f.Add("feat/12-login", "feature/", 0)
	f.Add("chore/abc-1-x", `^[A-Z]+-[0-9]+`, 3)
	f.Add("fix/x", "-v2", 1)
	f.Fuzz(func(t *testing.T, branch, pattern string, op int) {
		operators := []Operator{StartsWith, EndsWith, Contains, Regex}
		p := Pattern{Operator: operators[(op%4+4)%4], Pattern: pattern, Negate: op%2 == 1}
		got, found := CompliantBranch(branch, []Pattern{p})
		if !found {
			return
		}
		if !ValidRefName(got) || p.Check(got) == Violated {
			t.Fatalf("CompliantBranch(%q, %+v) = %q", branch, p, got)
		}
	})
}

func TestCommitMessageEndsWithOneNewline(t *testing.T) {
	if got := CommitMessage("feat: x\n\n"); got != "feat: x\n" {
		t.Fatalf("%q", got)
	}
}
