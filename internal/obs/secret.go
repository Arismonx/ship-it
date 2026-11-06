// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import "log/slog"

// Redacted is what a Secret renders as in logs.
const Redacted = "[REDACTED]"

// Secret holds a sensitive value (token, password, URL with credentials).
// It renders as [REDACTED] through slog's LogValuer interface and through
// fmt verbs, so it cannot leak into the journal or the black box.
type Secret string

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(Redacted) }

// String implements fmt.Stringer.
func (Secret) String() string { return Redacted }

// GoString implements fmt.GoStringer, covering the %#v verb.
func (Secret) GoString() string { return Redacted }

// Reveal returns the underlying value. Call it only where the value is used,
// never to log it.
func (s Secret) Reveal() string { return string(s) }
