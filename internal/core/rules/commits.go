// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package rules

// Commit is what the commit rules look at.
type Commit struct {
	ID             string
	Message        string
	AuthorEmail    string
	CommitterEmail string
	// Unsigned is true when the commit carries no signature at all. A
	// signature GitHub might not verify is left to GitHub.
	Unsigned bool
}

// Violation is the first rule a commit breaks.
type Violation struct {
	Type Type
	// Pattern is the broken pattern; zero for RequiredSignatures.
	Pattern Pattern
	Commit  Commit
	// Value is what was checked: the e-mail address, or the message.
	Value string
}

// CheckCommits returns the first rule that one of commits (oldest first)
// breaks: message patterns, author and committer e-mail patterns, then the
// signed-commits rule.
func CheckCommits(rules []Rule, commits []Commit) (Violation, bool) {
	checks := []struct {
		t     Type
		value func(Commit) string
	}{
		{CommitMessagePattern, func(c Commit) string { return CommitMessage(c.Message) }},
		{CommitAuthorEmailPattern, func(c Commit) string { return c.AuthorEmail }},
		{CommitterEmailPattern, func(c Commit) string { return c.CommitterEmail }},
	}
	for _, check := range checks {
		patterns := Of(rules, check.t)
		if len(patterns) == 0 {
			continue
		}
		for _, c := range commits {
			value := check.value(c)
			if p, broken := FirstViolated(patterns, value); broken {
				return Violation{Type: check.t, Pattern: p, Commit: c, Value: value}, true
			}
		}
	}
	if Has(rules, RequiredSignatures) {
		for _, c := range commits {
			if c.Unsigned {
				return Violation{Type: RequiredSignatures, Commit: c}, true
			}
		}
	}
	return Violation{}, false
}
