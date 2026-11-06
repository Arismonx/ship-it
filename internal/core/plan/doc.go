// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package plan defines Plan and the deterministic planner.
//
// A plan is what ship-it proposes before acting; AI providers may only refine it.
// Every decision comes with a Reason whose key is rendered by internal/i18n.
//
// Allowed imports: standard library without I/O, other core packages.
//
// See ADR-0004.
package plan
