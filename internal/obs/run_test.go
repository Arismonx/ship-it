// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAttachKeepsEarlyRecords(t *testing.T) {
	j := NewJournal(JournalOptions{})
	j.Logger.Info("run.start")
	dir := filepath.Join(t.TempDir(), "logs")
	if err := j.Attach(dir); err != nil {
		t.Fatal(err)
	}
	j.Logger.Info("run.end")
	j.Logger.Debug("git.exec")
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, string(j.RunID)+JournalSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "run.start") || !strings.Contains(string(data), "run.end") {
		t.Errorf("journal misses records:\n%s", data)
	}
	if strings.Contains(string(data), "git.exec") {
		t.Error("DEBUG reached the file journal")
	}
}

func TestDiscardWritesNothing(t *testing.T) {
	j := NewJournal(JournalOptions{})
	j.Discard()
	j.Logger.Info("run.start")
	if path, err := j.SaveBlackBox(); path != "" || err != nil {
		t.Fatalf("SaveBlackBox() = %q, %v; want nothing written", path, err)
	}
}

func TestSaveBlackBox(t *testing.T) {
	j := NewJournal(JournalOptions{})
	dir := t.TempDir()
	if err := j.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	j.Logger.Debug("git.exec", KeyStep, "git", KeyDurationMS, 3)
	path, err := j.SaveBlackBox()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, string(j.RunID)+BlackBoxSuffix) {
		t.Fatalf("path = %q", path)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "git.exec") {
		t.Fatalf("black box file misses DEBUG records:\n%s", data)
	}
}

func TestEchoTo(t *testing.T) {
	j := NewJournal(JournalOptions{})
	var stderr bytes.Buffer
	j.Logger.Debug("before")
	j.EchoTo(&stderr, slog.LevelDebug)
	j.Logger.Debug("git.exec", "exit_code", 0)
	if !strings.Contains(stderr.String(), "msg=git.exec") || strings.Contains(stderr.String(), "before") {
		t.Fatalf("echo = %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "time=") {
		t.Error("the terminal echo should not repeat the time")
	}
}

func TestTraceAndRepositoryAttributes(t *testing.T) {
	var file bytes.Buffer
	j := NewJournal(JournalOptions{File: &file})
	j.SetRepository(RepoHash(bytes.Repeat([]byte{7}, SaltSize), "/work/shop"))
	tc, ok := ParseTraceparent("00-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7-01")
	if !ok {
		t.Fatal("valid traceparent rejected")
	}
	j.Logger.InfoContext(WithTrace(context.Background(), tc), "mcp.request")
	var rec map[string]any
	if err := json.Unmarshal(file.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec[KeyTraceID] != "0af7651916cd43dd8448eb211c80319c" || rec[KeyParentSpanID] != "00f067aa0ba902b7" {
		t.Errorf("trace attributes = %v", rec)
	}
	if span, _ := rec[KeySpanID].(string); len(span) != 16 || span == "00f067aa0ba902b7" {
		t.Errorf("span_id = %v, want a new child span", rec[KeySpanID])
	}
	if hash, _ := rec[KeyRepoHash].(string); !strings.HasPrefix(hash, "hmac-sha256:") || strings.Contains(hash, "shop") {
		t.Errorf("repo_hash = %v", rec[KeyRepoHash])
	}
}

func TestParseTraceparentRejectsInvalid(t *testing.T) {
	for _, v := range []string{
		"",
		"00-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7",       // no flags
		"00-0AF7651916CD43DD8448EB211C80319C-00f067aa0ba902b7-01",    // upper case
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",    // zero trace
		"00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01",    // zero parent
		"ff-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7-01",    // version ff
		"00-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7-01-xx", // v00 has 4 parts
	} {
		if _, ok := ParseTraceparent(v); ok {
			t.Errorf("ParseTraceparent(%q) accepted", v)
		}
	}
	// A future version may add fields after the flags.
	if _, ok := ParseTraceparent("01-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7-01-future"); !ok {
		t.Error("a higher version with extra fields must be accepted")
	}
}

// runAt creates the files of a run created at t, of size bytes each.
func runAt(t *testing.T, dir string, at time.Time, size int, blackBox bool) RunID {
	t.Helper()
	id := newRunIDAt(at)
	names := []string{string(id) + JournalSuffix}
	if blackBox {
		names = append(names, string(id)+BlackBoxSuffix)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), bytes.Repeat([]byte("x"), size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestPruneKeepsTheNewestRuns(t *testing.T) {
	dir := t.TempDir()
	base := time.UnixMilli(1_800_000_000_000)
	var ids []RunID
	for i := range 25 {
		ids = append(ids, runAt(t, dir, base.Add(time.Duration(i)*time.Second), 10, i%5 == 0))
	}
	// A foreign file is never touched.
	os.WriteFile(filepath.Join(dir, "notes.jsonl"), []byte("mine"), 0o600)
	current := ids[3] // an old id, as when the clock moved back: still kept

	dry, err := Prune(dir, current, DefaultRetention, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Removed) != 5 || dry.Kept != 20 {
		t.Fatalf("dry run = %+v", dry)
	}
	if runs, _ := ListRuns(dir); len(runs) != 25 {
		t.Fatalf("dry run removed files: %d runs left", len(runs))
	}

	report, err := Prune(dir, current, DefaultRetention, false)
	if err != nil {
		t.Fatal(err)
	}
	runs, _ := ListRuns(dir)
	if len(report.Removed) != 5 || len(runs) != 20 {
		t.Fatalf("report = %+v, %d runs left", report, len(runs))
	}
	if runs[0].ID != ids[24] {
		t.Errorf("newest run %s was not kept first", ids[24])
	}
	found := false
	for _, r := range runs {
		found = found || r.ID == current
	}
	if !found {
		t.Error("the current run was removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.jsonl")); err != nil {
		t.Error("a foreign file was removed")
	}
}

func TestPruneBoundsTheSize(t *testing.T) {
	dir := t.TempDir()
	base := time.UnixMilli(1_800_000_000_000)
	for i := range 5 {
		runAt(t, dir, base.Add(time.Duration(i)*time.Second), 400, false)
	}
	report, err := Prune(dir, "", Retention{Runs: 20, Bytes: 1000}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 3 || report.FreedBytes != 1200 || report.Kept != 2 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunIDTime(t *testing.T) {
	at := time.UnixMilli(1_761_210_000_123).UTC()
	got, ok := RunIDTime(newRunIDAt(at))
	if !ok || !got.Equal(at) {
		t.Fatalf("RunIDTime() = %v, %v; want %v", got, ok, at)
	}
	for _, bad := range []RunID{"", "notes", "8ZZZZZZZZZZZZZZZZZZZZZZZZZ", "01JB0000000000000000000000U"} {
		if _, ok := RunIDTime(bad); ok {
			t.Errorf("RunIDTime(%q) accepted", bad)
		}
	}
}

func TestTimeline(t *testing.T) {
	var file bytes.Buffer
	j := NewJournal(JournalOptions{File: &file, FileLevel: slog.LevelDebug})
	j.Logger.Debug("git.exec", KeyStep, "git", KeyDurationMS, 5, "cmd", []string{"status", "--porcelain"}, "exit_code", 0)
	j.Logger.Warn("step.failed", KeyStep, "publish/push", KeyDurationMS, 812, "error", errors.New("rejected"))
	j.Logger.Info("run.end") // not a step

	fromFile, err := ReadEvents(&file)
	if err != nil {
		t.Fatal(err)
	}
	steps := Steps(fromFile)
	if len(steps) != 2 || len(Steps(j.Events())) != 2 {
		t.Fatalf("steps = %+v", steps)
	}
	var out bytes.Buffer
	if err := WriteTimeline(&out, steps); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// Sorted by start: the push began 812 ms before it was logged.
	if len(lines) != 2 || !strings.Contains(lines[1], "cmd=status --porcelain") ||
		!strings.HasPrefix(lines[0], "✗") || !strings.Contains(lines[0], "812ms") || !strings.Contains(lines[0], "error=rejected") {
		t.Fatalf("timeline:\n%s", out.String())
	}
}

func TestReadEventsSkipsBrokenLines(t *testing.T) {
	events, err := ReadEvents(strings.NewReader("{\"msg\":\"a\"}\nnot json\n{\"msg\":\"b\",\"step\":\"x\"}\n{\"msg\":"))
	if err != nil || len(events) != 2 || events[1].Step != "x" {
		t.Fatalf("events = %+v, %v", events, err)
	}
}

func TestSetLimits(t *testing.T) {
	j := NewJournal(JournalOptions{})
	for range 100 {
		j.Logger.Debug("git.exec", "cmd", strings.Repeat("x", 200))
	}
	j.SetLimits(Retention{Runs: 5}, 1000)
	var box bytes.Buffer
	j.DumpBlackBox(&box)
	if box.Len() > 1000 || j.BlackBox.Dropped() == 0 {
		t.Fatalf("black box holds %d bytes after shrinking to 1000", box.Len())
	}
	if r := j.Retention(); r.Runs != 5 || r.Bytes != DefaultRetention.Bytes {
		t.Fatalf("retention = %+v; a zero value must keep the current one", r)
	}
	j.SetLimits(Retention{}, 0)
	if j.Retention() != (Retention{Runs: 5, Bytes: DefaultRetention.Bytes}) {
		t.Fatal("zero limits must change nothing")
	}
}
