// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"errors"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestFindTicket(t *testing.T) {
	keys := []string{"ACME", "OPS_2"}
	for text, want := range map[string]string{
		"feat/ACME-123-add-login": "ACME-123",
		"feat/acme-123-add-login": "ACME-123",
		"fix/ops_2-7":             "OPS_2-7",
		"feat/xacme-1-login":      "",
		"feat/acme-0-login":       "",
		"feat/42-add-login":       "",
		"chore/acme-12x":          "",
		"fix: ACME-9 and OPS_2-3": "ACME-9",
		"fix: OPS_2-3 and ACME-9": "OPS_2-3",
	} {
		if got := FindTicket(text, keys); got != want {
			t.Errorf("FindTicket(%q) = %q, want %q", text, got, want)
		}
	}
	if got := FindTicket("feat/acme-1", []string{"acme", "A"}); got != "" {
		t.Errorf("invalid keys must be ignored, got %q", got)
	}
}

func TestWithTicket(t *testing.T) {
	h := Header{Type: Feature, Scope: "auth", Subject: "add login"}
	if got := Format(WithTicket(h, "ACME-123"), GitmojiFirst); got != "✨ feat(auth): add login (ACME-123)" {
		t.Errorf("got %q", got)
	}
	h.Subject = "add login for acme-123"
	if got := WithTicket(h, "ACME-123").Subject; got != h.Subject {
		t.Errorf("a subject that mentions the ticket is kept: %q", got)
	}
	if got := WithTicket(h, "").Subject; got != h.Subject {
		t.Error("no ticket, no change")
	}
}

func TestTheTicketDoesNotCountInTheSubjectLength(t *testing.T) {
	h := Header{Type: Feature, Subject: "add a login page with single sign-on via SAML" + " (ACME-12345)"}
	for _, f := range Validate(h) {
		if errors.Is(f, &fault.Fault{Code: fault.SubjectTooLong}) {
			t.Fatalf("the ticket counted: %v", f)
		}
	}
}

func TestBranchesWithTickets(t *testing.T) {
	name := BranchName(Feature, "add login page", BranchOptions{Ticket: "acme-123", Issue: 7})
	if name != "feat/ACME-123-add-login-page" {
		t.Fatalf("BranchName = %q", name)
	}
	if err := ValidateBranchWith(name, []string{"ACME"}); err != nil {
		t.Errorf("a configured key may be upper case: %v", err)
	}
	if err := ValidateBranch(name); err == nil {
		t.Error("without a configured tracker, upper case stays invalid (Conventional Branch)")
	}
	if err := ValidateBranchWith("feat/ACME-123-Add", []string{"ACME"}); err == nil {
		t.Error("only the ticket may be upper case")
	}
}
