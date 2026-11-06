// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package notify implements ports.Notifier with native notifications.
//
// WinRT on Windows, osascript on macOS, notify-send on Linux, terminal bell as fallback.
//
// Allowed imports: ports, core, platform, standard library, golang.org/x.
package notify
