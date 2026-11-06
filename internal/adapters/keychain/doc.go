// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package keychain implements ports.Credentials on the operating system keychain.
//
// Windows Credential Manager, macOS security, Linux secret-tool; one token per profile.
//
// Allowed imports: ports, core, platform, standard library, golang.org/x.
//
// See ADR-0003.
package keychain
