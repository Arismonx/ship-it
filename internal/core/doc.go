// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package core is the root of the pure domain of ship-it.
//
// Subpackages hold every decision ship-it makes; none of them performs I/O.
//
// Allowed imports: standard library without I/O (no os, os/exec, net), other core packages.
//
// See ARCHITECTURE.md.
package core
