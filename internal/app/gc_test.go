// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestCollectGarbage(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	ws, store, log := workspaceOf(testkit.HealthyRepository("main"))
	// 25 checkpoints, one per hour, all 100 days old: only the 20 latest
	// survive, plus the one an undo still needs.
	for i := range 25 {
		store.Checkpoints = append(store.Checkpoints, ports.Checkpoint{
			ID: fmt.Sprintf("cp%02d", i), CreatedAt: now.AddDate(0, 0, -100).Add(-time.Duration(i) * time.Hour),
		})
	}
	log.Operations = []oplog.Operation{
		{ID: "op2", Kind: oplog.Deliver, Checkpoint: "cp24"},
		{ID: "op1", Kind: oplog.Deliver, Checkpoint: "cp23"},
		{ID: "op0", Kind: oplog.Undo, Undoes: "op1"},
	}

	dry, err := CollectGarbage(context.Background(), ws, GarbageRequest{Now: now, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Removed) != 4 || dry.Kept != 21 || len(store.Checkpoints) != 25 {
		t.Fatalf("dry run = %+v, store has %d", dry, len(store.Checkpoints))
	}

	report, err := CollectGarbage(context.Background(), ws, GarbageRequest{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 4 || len(store.Checkpoints) != 21 {
		t.Fatalf("report = %+v, store has %d", report, len(store.Checkpoints))
	}
	for _, c := range store.Checkpoints {
		if c.ID == "cp23" {
			t.Error("cp23 belongs to an undone delivery and should have been removed")
		}
	}
	if store.Checkpoints[len(store.Checkpoints)-1].ID != "cp24" {
		t.Error("cp24 is needed by an undoable delivery and must be kept")
	}
}

func TestUndoSurvivesARemovedCheckpoint(t *testing.T) {
	var calls []string
	ws, store, log := workspaceOf(gitAfterDelivery(&calls))
	log.Operations = []oplog.Operation{delivered} // its checkpoint cp1 is gone
	if _, err := Undo(context.Background(), ws, UndoRequest{}); err != nil {
		t.Fatal(err)
	}
	if store.Restored != "" || len(calls) != 3 {
		t.Fatalf("restored %q, calls %q", store.Restored, calls)
	}
}

func TestRunSettings(t *testing.T) {
	ws := Workspace{Config: &testkit.FakeConfigFiles{Repository: `{"ship": {"logs": {"keepRuns": 3}}}`,
		User: `{"ui": {"language": "fr", "reviewScreen": false, "accessible": true, "color": "never"}}`}}
	limits, ui := RunSettings(context.Background(), ws)
	if limits.KeepRuns != 3 || ui != (Interface{Language: "fr", ReviewScreen: false, Accessible: true, Color: ColorNever}) {
		t.Fatalf("limits %+v, ui %+v", limits, ui)
	}
	// Defaults, auto as "", also when the configuration is broken.
	for _, file := range []string{"", `{"ui": {"color": 1}}`} {
		ws.Config = &testkit.FakeConfigFiles{User: file}
		if _, ui := RunSettings(context.Background(), ws); ui != (Interface{ReviewScreen: true, Color: ColorAuto}) {
			t.Errorf("%q: ui %+v", file, ui)
		}
	}
}
