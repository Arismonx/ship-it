// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package policy merges configuration layers and enforces locked organisation policy.
//
// Layers: defaults < user < repository (.github/ship.json) < organisation policy.
//
// Allowed imports: standard library without I/O, other core packages.
//
// See ADR-0009 and ADR-0014.
package policy
