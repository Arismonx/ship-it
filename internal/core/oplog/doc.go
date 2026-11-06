// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package oplog models operations, before/after views and compensating actions.
//
// It decides how to undo an operation; app applies the steps through ports.
// The log is append-only: an undo is a new entry, never a modification.
//
// Allowed imports: standard library without I/O, other core packages.
//
// See ADR-0005 and docs/design/undo-storage-optimizations.md.
package oplog
