// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/retention"
)

// garbageScanDepth bounds how many checkpoints one collection examines.
const garbageScanDepth = 100_000

// GarbageRequest configures `ship gc`.
type GarbageRequest struct {
	Now time.Time
	// DryRun reports what would be removed without removing it.
	DryRun bool
}

// GarbageReport lists the checkpoints removed (or removable on a dry run).
type GarbageReport struct {
	Removed []CheckpointView `json:"removed"`
	Kept    int              `json:"kept"`
	DryRun  bool             `json:"dry_run"`
	// Logs reports the run journals pruned; absent when there are none to
	// look at.
	Logs *LogsReport `json:"logs,omitempty"`
}

// LogsReport counts the run journals removed (or removable on a dry run).
type LogsReport struct {
	RemovedRuns int   `json:"removed_runs"`
	FreedBytes  int64 `json:"freed_bytes"`
}

// LogLimits bound the run journals (ship.logs, docs/reference/logs.md).
type LogLimits struct {
	KeepRuns      int
	MaxBytes      int64
	BlackBoxBytes int
}

// RunLogLimits reads ship.logs. A configuration that cannot be read gives
// the defaults: logging never stops the work, and the command that needs the
// settings reports the problem itself.
func RunLogLimits(ctx context.Context, ws Workspace) LogLimits {
	limits, _ := RunSettings(ctx, ws)
	return limits
}

// RunSettings reads, in one pass, what a run needs before its command: the
// journal's limits (ship.logs) and the interface settings (ui). A
// configuration that cannot be read gives the defaults, as for RunLogLimits.
func RunSettings(ctx context.Context, ws Workspace) (LogLimits, Interface) {
	s := config.Defaults()
	if eff, err := LoadSettings(ctx, ws); err == nil {
		s = eff.Settings
	}
	logs := s.Ship.Logs
	ui := Interface{Language: s.UI.Language, ReviewScreen: s.UI.ReviewScreen, Accessible: s.UI.Accessible, Color: s.UI.Color}
	if ui.Language == config.LanguageAuto {
		ui.Language = ""
	}
	return LogLimits{KeepRuns: logs.KeepRuns, MaxBytes: int64(logs.MaxBytes), BlackBoxBytes: logs.BlackBoxBytes}, ui
}

// Interface are the ui settings, for the terminal.
type Interface struct {
	// Language is "en" or "fr", "" when ui.language is auto.
	Language     string
	ReviewScreen bool
	Accessible   bool
	// Color is ColorAuto, ColorAlways or ColorNever.
	Color string
}

// Values of ui.color.
const (
	ColorAuto   = config.ColorAuto
	ColorAlways = config.ColorAlways
	ColorNever  = config.ColorNever
)

// CollectGarbage removes old checkpoints following retention.DefaultPolicy,
// then old run journals beyond ship.logs.keepRuns and ship.logs.maxBytes. Checkpoints of deliveries
// that can still be undone are never removed.
func CollectGarbage(ctx context.Context, ws Workspace, req GarbageRequest) (GarbageReport, error) {
	done := ws.begin(ctx, "gc")
	report, err := collectGarbage(ctx, ws, req)
	if err == nil && ws.RunLogs != nil {
		var logs LogsReport
		limits := RunLogLimits(ctx, ws)
		logs.RemovedRuns, logs.FreedBytes, err = ws.RunLogs.Prune(ctx, limits.KeepRuns, limits.MaxBytes, req.DryRun)
		report.Logs = &logs
	}
	done(err, slog.Int("removed", len(report.Removed)), slog.Bool("dry_run", req.DryRun))
	return report, err
}

func collectGarbage(ctx context.Context, ws Workspace, req GarbageRequest) (GarbageReport, error) {
	checkpoints, err := ws.Checkpoints.List(ctx, garbageScanDepth)
	if err != nil {
		return GarbageReport{}, err
	}
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return GarbageReport{}, err
	}
	protected := undoableCheckpoints(ops)

	items := make([]retention.Item, len(checkpoints))
	byID := make(map[string]CheckpointView, len(checkpoints))
	for i, c := range checkpoints {
		items[i] = retention.Item{ID: c.ID, CreatedAt: c.CreatedAt}
		byID[c.ID] = viewOf(c)
	}
	expired := retention.Expired(items, req.Now, retention.DefaultPolicy, protected)

	report := GarbageReport{Kept: len(checkpoints) - len(expired), DryRun: req.DryRun, Removed: []CheckpointView{}}
	for _, id := range expired {
		report.Removed = append(report.Removed, byID[id])
	}
	if req.DryRun || len(expired) == 0 {
		return report, nil
	}
	return report, ws.Checkpoints.Remove(ctx, expired)
}

// undoableCheckpoints returns the checkpoints of deliveries not undone yet.
func undoableCheckpoints(ops []oplog.Operation) map[string]bool {
	protected := map[string]bool{}
	for _, op := range oplog.Undoable(ops) {
		if op.Checkpoint != "" {
			protected[op.Checkpoint] = true
		}
	}
	return protected
}
