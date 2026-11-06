// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package app holds the use cases: one exported function per user intent.
//
// Deliver, Undo, Status, Sync, Init, Release, Doctor, Fix. It orchestrates core and ports and never prompts.
//
// Allowed imports: core packages, ports, i18n keys.
//
// See ARCHITECTURE.md.
package app
