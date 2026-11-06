// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package ghstatus implements ports.StatusPage with the githubstatus.com API.
//
// Used to explain failures and to drive the offline queue.
//
// Allowed imports: ports, core, platform, standard library, golang.org/x.
//
// See docs/design/github-frictions.md.
package ghstatus
