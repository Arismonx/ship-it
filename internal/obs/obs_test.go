// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestJournalSplitsLevels(t *testing.T) {
	var file bytes.Buffer
	j := NewJournal(JournalOptions{File: &file, RunID: "RUN1"})
	j.Logger.Debug("plan.detail", "files", 3)
	j.Logger.Info("push.done", "duration_ms", 812)

	if strings.Contains(file.String(), "plan.detail") {
		t.Error("DEBUG record reached the file journal")
	}
	if !strings.Contains(file.String(), "push.done") {
		t.Error("INFO record missing from the file journal")
	}
	var box bytes.Buffer
	if err := j.DumpBlackBox(&box); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(box.String(), "plan.detail") || !strings.Contains(box.String(), "push.done") {
		t.Errorf("black box incomplete: %s", box.String())
	}
}

func TestJournalRecordFields(t *testing.T) {
	var file bytes.Buffer
	j := NewJournal(JournalOptions{File: &file, RunID: "RUN1"})
	j.Logger.Warn("gate.failed")
	var rec map[string]any
	if err := json.Unmarshal(file.Bytes(), &rec); err != nil {
		t.Fatalf("journal line is not JSON: %v (%s)", err, file.String())
	}
	if rec[KeyRunID] != "RUN1" {
		t.Errorf("run_id = %v", rec[KeyRunID])
	}
	if rec[KeySeverityNumber] != float64(13) {
		t.Errorf("severity_number = %v, want 13 (OTel WARN)", rec[KeySeverityNumber])
	}
}

func TestOTelSeverity(t *testing.T) {
	cases := map[slog.Level]int{slog.LevelDebug: 5, slog.LevelInfo: 9, slog.LevelWarn: 13, slog.LevelError: 17, -100: 1, 100: 24}
	for level, want := range cases {
		if got := OTelSeverity(level); got != want {
			t.Errorf("OTelSeverity(%d) = %d, want %d", level, got, want)
		}
	}
}

func TestBlackBoxKeepsMostRecent(t *testing.T) {
	box := NewBlackBox(10)
	for i := 0; i < 5; i++ {
		fmt.Fprintf(box, "line%d\n", i)
	}
	var out bytes.Buffer
	box.WriteTo(&out)
	if got := out.String(); got != "line4\n" {
		t.Fatalf("black box = %q, want only the most recent line", got)
	}
	if box.Dropped() != 4 {
		t.Fatalf("Dropped() = %d, want 4", box.Dropped())
	}
}

func TestSecretNeverLeaks(t *testing.T) {
	// Assembled at run time so that ship-it's secret scan, run on its own
	// code, does not flag this fake token.
	token := strings.Join([]string{"gh", "p_0123456789abcdefghijklmnopqrstuvwxyz"}, "")
	var file bytes.Buffer
	j := NewJournal(JournalOptions{File: &file})
	j.Logger.Info("auth.ok", "token", Secret(token))
	var box bytes.Buffer
	j.DumpBlackBox(&box)
	formatted := fmt.Sprintf("%v %s %+v %#v", Secret(token), Secret(token), Secret(token), Secret(token))
	for name, s := range map[string]string{"file": file.String(), "blackbox": box.String(), "fmt": formatted} {
		if strings.Contains(s, token) {
			t.Errorf("secret leaked into %s: %s", name, s)
		}
	}
	if Secret(token).Reveal() != token {
		t.Error("Reveal() must return the value")
	}
}

func TestRunIDSortsByTime(t *testing.T) {
	a := newRunIDAt(time.UnixMilli(1_700_000_000_000))
	b := newRunIDAt(time.UnixMilli(1_700_000_000_001))
	if len(a) != 26 || len(b) != 26 {
		t.Fatalf("run IDs must have 26 characters: %q %q", a, b)
	}
	if !(a < b) {
		t.Fatalf("run IDs not time-ordered: %q >= %q", a, b)
	}
}
