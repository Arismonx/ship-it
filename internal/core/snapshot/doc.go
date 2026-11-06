// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package snapshot models the work in progress: changed files, their added
// and removed lines, and the category each file belongs to.
//
// It is built from data returned by ports.Git and never reads the disk itself.
//
// Allowed imports: standard library without I/O, other core packages.
// See docs/design/architecture.md.
package snapshot
