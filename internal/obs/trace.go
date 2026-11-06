// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// TraceContext is the W3C Trace Context of a request
// (https://www.w3.org/TR/trace-context/). SpanID is the span of ship-it's own
// work; ParentSpanID is the caller's span, taken from its traceparent.
type TraceContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
}

type traceKey struct{}

// WithTrace returns a context whose log records carry tc.
func WithTrace(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceKey{}, tc)
}

// TraceFrom returns the trace context of ctx, if any.
func TraceFrom(ctx context.Context) (TraceContext, bool) {
	tc, ok := ctx.Value(traceKey{}).(TraceContext)
	return tc, ok
}

// ParseTraceparent reads a W3C traceparent header value
// ("00-<32 hex trace-id>-<16 hex parent-id>-<2 hex flags>") and returns a
// child trace context: same trace, a new span whose parent is the caller's.
// Invalid values (wrong lengths, upper-case hex, all-zero ids, version ff)
// are rejected, as the specification requires.
func ParseTraceparent(value string) (TraceContext, bool) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) < 4 {
		return TraceContext{}, false
	}
	version, traceID, parentID, flags := parts[0], parts[1], parts[2], parts[3]
	if !isLowerHex(version, 2) || version == "ff" || (version == "00" && len(parts) != 4) {
		return TraceContext{}, false
	}
	if !isLowerHex(traceID, 32) || !isLowerHex(parentID, 16) || !isLowerHex(flags, 2) {
		return TraceContext{}, false
	}
	if strings.Trim(traceID, "0") == "" || strings.Trim(parentID, "0") == "" {
		return TraceContext{}, false
	}
	return TraceContext{TraceID: traceID, SpanID: NewSpanID(), ParentSpanID: parentID}, true
}

// NewSpanID returns a random, non-zero 8-byte span id in lower-case hex.
func NewSpanID() string {
	var b [8]byte
	for {
		_, _ = rand.Read(b[:]) // crypto/rand.Read never fails since Go 1.24
		if b != [8]byte{} {
			return hex.EncodeToString(b[:])
		}
	}
}

func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
