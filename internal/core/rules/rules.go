// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package rules evaluates the GitHub ruleset rules that apply to a branch
// (GET /repos/{owner}/{repo}/rules/branches/{branch}) before ship-it pushes:
// the metadata restrictions on the branch name, the commit messages and the
// e-mail addresses, and the signed-commits rule. It never calls GitHub: app
// passes the rules it read through a port.
//
// Semantics follow the GitHub documentation ("Creating rulesets for a
// repository", checked 2026-10-24): literal operators compare the
// pattern as written, without wildcards; regex patterns are RE2, which is Go's
// regexp syntax, and only consider several lines when they start with (?m).
//
// Allowed imports: standard library without I/O, other core packages.
package rules

import (
	"regexp"
	"strings"
)

// Type is a rule type as the REST API names it.
type Type string

// The rule types ship-it evaluates before a push. GitHub knows more (pull
// request reviews, status checks…); those apply after the push and are left
// to GitHub.
const (
	BranchNamePattern        Type = "branch_name_pattern"
	CommitMessagePattern     Type = "commit_message_pattern"
	CommitAuthorEmailPattern Type = "commit_author_email_pattern"
	CommitterEmailPattern    Type = "committer_email_pattern"
	RequiredSignatures       Type = "required_signatures"
	RequiredLinearHistory    Type = "required_linear_history"
	Creation                 Type = "creation"
)

// Operator is how a pattern is compared.
type Operator string

// Operators of the pattern rules (enum of the REST API).
const (
	StartsWith Operator = "starts_with"
	EndsWith   Operator = "ends_with"
	Contains   Operator = "contains"
	Regex      Operator = "regex"
)

// Pattern is the parameters object of a pattern rule.
type Pattern struct {
	// Name is how the rule appears to users; it may be empty.
	Name     string   `json:"name,omitempty"`
	Negate   bool     `json:"negate,omitempty"`
	Operator Operator `json:"operator"`
	Pattern  string   `json:"pattern"`
}

// Rule is one active rule of a ruleset.
type Rule struct {
	Type Type `json:"type"`
	// Pattern is set for the four pattern rules only.
	Pattern *Pattern `json:"parameters,omitempty"`
	// RulesetID identifies the ruleset that defines the rule, 0 if unknown.
	RulesetID int `json:"ruleset_id,omitempty"`
}

// Evaluation is the outcome of checking one value against a pattern.
type Evaluation int

// Outcomes of Check.
const (
	Satisfied Evaluation = iota
	Violated
	// Unknown: the regular expression does not compile in Go (a syntax
	// GitHub might accept beyond RE2); GitHub decides at push time.
	Unknown
)

// Check evaluates value against p: it is satisfied when the pattern matches,
// or, with negate ("must not match"), when it does not.
func (p Pattern) Check(value string) Evaluation {
	var matched bool
	switch p.Operator {
	case StartsWith:
		matched = strings.HasPrefix(value, p.Pattern)
	case EndsWith:
		matched = strings.HasSuffix(value, p.Pattern)
	case Contains:
		matched = strings.Contains(value, p.Pattern)
	case Regex:
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return Unknown
		}
		matched = re.MatchString(value)
	default:
		return Unknown
	}
	if matched != p.Negate {
		return Satisfied
	}
	return Violated
}

// Label names the rule for messages: its name, else its pattern.
func (p Pattern) Label() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Pattern
}

// Describe writes the requirement in the words of GitHub's ruleset form
// ("must start with", "must not match a given regex pattern"), followed by
// the pattern, for messages that quote the rule.
func (p Pattern) Describe() string {
	verb := map[Operator]string{
		StartsWith: "start with", EndsWith: "end with", Contains: "contain", Regex: "match",
	}[p.Operator]
	if verb == "" {
		verb = string(p.Operator)
	}
	must := "must "
	if p.Negate {
		must = "must not "
	}
	return must + verb + " " + p.Pattern
}

// Of returns the patterns of the rules of type t, in order.
func Of(rules []Rule, t Type) []Pattern {
	var patterns []Pattern
	for _, r := range rules {
		if r.Type == t && r.Pattern != nil {
			patterns = append(patterns, *r.Pattern)
		}
	}
	return patterns
}

// Has reports whether a rule of type t applies.
func Has(rules []Rule, t Type) bool {
	for _, r := range rules {
		if r.Type == t {
			return true
		}
	}
	return false
}

// FirstViolated returns the first pattern that value violates. Patterns
// that cannot be evaluated are skipped.
func FirstViolated(patterns []Pattern, value string) (Pattern, bool) {
	for _, p := range patterns {
		if p.Check(value) == Violated {
			return p, true
		}
	}
	return Pattern{}, false
}

// CommitMessage is the message as GitHub evaluates it: git stores a message
// with its final newline, and GitHub's documentation advises "\n?$" rather
// than "$" for end-of-line anchors, which only makes sense with that newline.
func CommitMessage(message string) string {
	message = strings.TrimRight(message, "\n")
	return message + "\n"
}
