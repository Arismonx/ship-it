// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		out = append(out, rec)
	}
	return out
}

func TestUndoIsJournaledWithItsOperation(t *testing.T) {
	var calls []string
	ws, _, log := workspaceOf(gitAfterDelivery(&calls))
	log.Operations = []oplog.Operation{delivered}
	var buf bytes.Buffer
	ws.Log = slog.New(slog.NewJSONHandler(&buf, nil))
	result, err := Undo(context.Background(), ws, UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	recs := records(t, &buf)
	last := recs[len(recs)-1]
	if last["msg"] != msgStepDone || last["step"] != "undo" || last["op_id"] != result.Operation || last["level"] != "INFO" {
		t.Fatalf("undo record = %v", last)
	}
	if _, ok := last["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
}

func TestFailedStepIsAWarning(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	var buf bytes.Buffer
	ws.Log = slog.New(slog.NewJSONHandler(&buf, nil))
	if _, err := Undo(context.Background(), ws, UndoRequest{}); err == nil {
		t.Fatal("nothing to undo should fail")
	}
	rec := records(t, &buf)[0]
	if rec["msg"] != msgStepFailed || rec["level"] != "WARN" || rec["error"] == nil {
		t.Fatalf("record = %v", rec)
	}
}

type fakeRunLogs struct {
	dryRun   bool
	keepRuns int
	maxBytes int64
	err      error
}

func (f *fakeRunLogs) Prune(_ context.Context, keepRuns int, maxBytes int64, dryRun bool) (int, int64, error) {
	f.dryRun, f.keepRuns, f.maxBytes = dryRun, keepRuns, maxBytes
	return 3, 1234, f.err
}

func TestCollectGarbagePrunesRunLogs(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	logs := &fakeRunLogs{}
	ws.RunLogs = logs
	report, err := CollectGarbage(context.Background(), ws, GarbageRequest{Now: time.Now(), DryRun: true})
	if err != nil || !logs.dryRun || report.Logs == nil || report.Logs.RemovedRuns != 3 || report.Logs.FreedBytes != 1234 {
		t.Fatalf("report = %+v, %v (dry run passed: %v)", report, err, logs.dryRun)
	}
	if logs.keepRuns != 20 || logs.maxBytes != 10<<20 {
		t.Fatalf("default limits not passed: %d runs, %d bytes", logs.keepRuns, logs.maxBytes)
	}
	logs.err = errors.New("permission denied")
	if _, err := CollectGarbage(context.Background(), ws, GarbageRequest{Now: time.Now()}); err == nil {
		t.Fatal("a pruning error must be reported")
	}
}
