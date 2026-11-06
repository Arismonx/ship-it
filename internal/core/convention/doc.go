// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package convention formats and parses commit and pull request titles.
//
// It owns the "✨ type(scope)!: subject" grammar, the gitmoji table, branch naming and semver bump rules.
//
// Allowed imports: standard library without I/O, other core packages.
//
// See ADR-0007 and docs/reference/conventions.md.
package convention
