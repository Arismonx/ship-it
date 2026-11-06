// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"errors"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
)

// needsHuman lists the faults only a person can fix: logging in, deciding
// what to do with work made after an operation.
var needsHuman = map[fault.Code]bool{
	fault.NotAuthenticated:              true,
	fault.TokenRejected:                 true,
	fault.UndoNeedsCleanTree:            true,
	fault.RepositoryMovedSinceOperation: true,
	fault.UndoNeedsConsent:              true,
	fault.ConfigFileChanged:             true,
	fault.MCPManualEdit:                 true,
	fault.MergeNeedsConfirmation:        true,
}

// NeedsConsent reports whether err asks the human to agree to a change on
// GitHub; the surface asks, then calls again with consent.
func NeedsConsent(err error) bool {
	var f *Fault
	return errors.As(err, &f) && f.Code == fault.UndoNeedsConsent
}

// NeedsHuman reports whether err asks for a person, so that surfaces stop and
// hand over (exit code 2) instead of letting an agent retry.
func NeedsHuman(err error) bool {
	var f *Fault
	return errors.As(err, &f) && needsHuman[f.Code]
}

// IsPolicyRefusal reports whether err is a refusal of the organisation
// policy, which surfaces exit with code 5.
func IsPolicyRefusal(err error) bool {
	var f *Fault
	return errors.As(err, &f) && f.Code == fault.PolicyConflict
}

// IsLocalRepository reports whether err only says the repository has no
// GitHub remote, a normal setup in which work stays local.
func IsLocalRepository(err error) bool {
	var f *Fault
	return errors.As(err, &f) && f.Code == fault.NoGitHubRemote
}

// IsUnexpected reports whether err is a failure ship-it has no answer for:
// not one of its faults, or the fault that wraps an unexpected error. The
// surfaces keep the black box for those only (docs/reference/logs.md).
func IsUnexpected(err error) bool {
	var f *fault.Fault
	if !errors.As(err, &f) {
		return true
	}
	return f.Code == fault.Unexpected
}

// Known turns the errors of the ports that have an answer into faults:
// git run outside a repository, GitHub that does not answer. Surfaces pass
// every error through it before rendering, so that an expected situation
// never reads as a raw Go error nor leaves a black box. Other errors are
// returned as they are.
func Known(err error) error {
	var f *Fault
	if err == nil || errors.As(err, &f) {
		return err
	}
	var unreachable *ports.UnreachableError
	switch {
	case errors.Is(err, ports.ErrNotARepository):
		return fault.New(fault.NotInRepository, "app.not_a_repository")
	case errors.As(err, &unreachable):
		return fault.New(fault.GitHubNoAnswer, "app.github_unreachable", "host", unreachable.Host)
	}
	return err
}
