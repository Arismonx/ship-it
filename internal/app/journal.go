// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"log/slog"
	"time"
)

// Journal message keys and attributes written by the use cases. Keys are
// stable identifiers, not sentences (docs/reference/logs.md §3).
const (
	msgStepDone   = "step.done"
	msgStepFailed = "step.failed"
	attrStep      = "step"
	attrDuration  = "duration_ms"
	attrOperation = "op_id"
	attrDryRun    = "dry_run"
)

// logger returns the workspace logger, or one that discards everything.
func (ws Workspace) logger() *slog.Logger {
	if ws.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return ws.Log
}

// begin starts timing a step. The returned function logs it with its
// duration: INFO when it succeeded, WARN with the error otherwise. Steps are
// named "<use case>" or "<use case>/<part>" (deliver, publish/push…).
func (ws Workspace) begin(ctx context.Context, step string) func(err error, attrs ...slog.Attr) {
	start := time.Now()
	return func(err error, attrs ...slog.Attr) {
		all := append([]slog.Attr{
			slog.String(attrStep, step), slog.Int64(attrDuration, time.Since(start).Milliseconds()),
		}, attrs...)
		if ws.IsDryRun() {
			// "publish/push done" only planned the push.
			all = append(all, slog.Bool(attrDryRun, true))
		}
		if err != nil {
			all = append(all, slog.String("error", err.Error()))
			ws.logger().LogAttrs(ctx, slog.LevelWarn, msgStepFailed, all...)
			return
		}
		ws.logger().LogAttrs(ctx, slog.LevelInfo, msgStepDone, all...)
	}
}

// operation names the oplog entry a step created, when it created one.
func operation(id string) []slog.Attr {
	if id == "" {
		return nil
	}
	return []slog.Attr{slog.String(attrOperation, id)}
}
