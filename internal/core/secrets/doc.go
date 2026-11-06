// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package secrets holds the rules that detect secrets and personal data in added lines.
//
// It runs before any commit, push or AI request.
//
// Allowed imports: standard library without I/O, other core packages.
//
// See docs/design/compliance.md.
package secrets
