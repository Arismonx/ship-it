// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"regexp"
	"strings"
)

// External trackers (Jira, Linear) name their tickets "<KEY>-<number>", the
// key being an upper-case project key. Jira recognises a ticket anywhere in a
// commit message, in upper case only; ship-it carries the ticket of the branch
// into the end of the subject, where Conventional Commits stay valid
// ("feat(auth): add login (ACME-123)"), decision of 2026-10-24.

// TrackerKeyPattern is the shape of a project key: Jira's rule ("starts with
// a letter; upper-case letters, digits and underscores", Atlassian, "Changing
// the project key format"), which Linear team keys also satisfy.
var TrackerKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)

// ticketSuffix matches a ticket reference ending a subject: " (ACME-123)".
var ticketSuffix = regexp.MustCompile(`\s\([A-Z][A-Z0-9_]+-[1-9][0-9]*\)$`)

// FindTicket returns the first ticket of one of keys in text, without regard
// to case, written in upper case as trackers expect ("feat/acme-12-login" →
// "ACME-12"); "" when there is none. A key must stand alone: "XACME-1" is not
// a ticket of ACME.
func FindTicket(text string, keys []string) string {
	upper := strings.ToUpper(text)
	best, at := "", len(upper)+1
	for _, key := range keys {
		if !TrackerKeyPattern.MatchString(key) {
			continue
		}
		re := regexp.MustCompile(`(?:^|[^A-Z0-9_])(` + regexp.QuoteMeta(key) + `-[1-9][0-9]*)(?:$|[^A-Z0-9_])`)
		if m := re.FindStringSubmatchIndex(upper); m != nil && m[2] < at {
			best, at = upper[m[2]:m[3]], m[2]
		}
	}
	return best
}

// WithTicket ends the subject with " (TICKET)" unless it already mentions
// the ticket.
func WithTicket(h Header, ticket string) Header {
	if ticket == "" || strings.Contains(strings.ToUpper(h.Subject), ticket) {
		return h
	}
	h.Subject = strings.TrimSpace(h.Subject) + " (" + ticket + ")"
	return h
}

// subjectWithoutTicket is the subject whose length the convention bounds:
// the ticket reference ship-it appends is not prose and does not count.
func subjectWithoutTicket(subject string) string {
	return ticketSuffix.ReplaceAllString(subject, "")
}
