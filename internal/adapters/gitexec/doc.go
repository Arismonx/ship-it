// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package gitexec implements ports.Git by executing the git binary.
//
// It never goes through a shell and parses -z porcelain output.
//
// Allowed imports: ports, core, platform, standard library, golang.org/x.
//
// See ADR-0002.
package gitexec
