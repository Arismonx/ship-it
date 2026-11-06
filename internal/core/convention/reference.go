// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"strconv"
	"strings"
)

// IssueKeyword links a commit or pull request to an issue. The first three
// are GitHub closing keywords; ReferencesIssue only links. The set matches the
// issue prefixes of @gregoiref/cz-config (close, fix, ref).
type IssueKeyword string

// Issue keywords, written in their capitalised form.
const (
	ClosesIssue     IssueKeyword = "Closes"
	FixesIssue      IssueKeyword = "Fixes"
	ResolvesIssue   IssueKeyword = "Resolves"
	ReferencesIssue IssueKeyword = "Refs"
)

// DefaultIssueKeyword is "Closes", the default of @gregoiref/cz-config.
const DefaultIssueKeyword = ClosesIssue

// ClosesOnMerge reports whether GitHub closes the issue when the commit or
// pull request lands on the default branch.
func (k IssueKeyword) ClosesOnMerge() bool { return k != ReferencesIssue }

// IssueReference renders "Closes #12" or "Closes owner/repo#12". GitHub only
// honours closing keywords on the default branch, which is why ship-it writes
// them in the squashed commit and uses Refs in stacked PR bodies (NB4).
func IssueReference(k IssueKeyword, repository string, issue int) string {
	target := "#" + strconv.Itoa(issue)
	if repository = strings.TrimSpace(repository); repository != "" {
		target = repository + target
	}
	return string(k) + " " + target
}
