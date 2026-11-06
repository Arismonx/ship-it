// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package obs is ship-it's observability layer: the technical journal.
//
// It builds a log/slog logger that writes INFO and above to a JSONL journal
// and keeps every record, DEBUG included, in a bounded in-memory black box
// that is written to disk only when a run fails (docs/reference/logs.md).
// Records carry OpenTelemetry-compatible fields (severity, W3C trace context)
// and are scrubbed of secrets, e-mails and absolute paths. Retention and the
// timeline reader (ship trace) live here too.
//
// Allowed imports: standard library, internal/platform.
package obs
