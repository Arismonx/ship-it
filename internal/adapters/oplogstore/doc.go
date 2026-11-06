// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package oplogstore persists the operation log under .git/ship/ with atomic writes.
//
// Allowed imports: ports, core, platform, standard library, golang.org/x.
//
// See ADR-0005.
package oplogstore
