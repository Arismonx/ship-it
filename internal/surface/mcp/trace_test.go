// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/obs"
)

const traceparent = "00-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7-01"

func journalRecords(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) == nil && rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

func TestTraceparentFromMetaReachesTheJournal(t *testing.T) {
	var file bytes.Buffer
	j := obs.NewJournal(obs.JournalOptions{File: &file})
	h := newHarness()
	h.ws.Log = j.Logger
	srv := &Server{Workspace: h.ws, Version: "1.2.3", Journal: j}
	withTrace := strings.Replace(modern(1, "tools/call", `"name":"ship_undo","arguments":{}`),
		`"_meta":{`, `"_meta":{"traceparent":"`+traceparent+`",`, 1)
	var out bytes.Buffer
	in := strings.Join([]string{withTrace, modern(2, "tools/list", "")}, "\n") + "\n"
	if err := srv.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	requests := journalRecords(t, &file, "mcp.request")
	if len(requests) != 2 {
		t.Fatalf("want 2 mcp.request records:\n%s", file.String())
	}
	traced, untraced := requests[0], requests[1]
	if traced["step"] != "mcp/tools/call/ship_undo" || traced[obs.KeyTraceID] != "0af7651916cd43dd8448eb211c80319c" ||
		traced[obs.KeyParentSpanID] != "00f067aa0ba902b7" {
		t.Errorf("traced record = %v", traced)
	}
	if span, _ := traced[obs.KeySpanID].(string); len(span) != 16 {
		t.Errorf("span_id = %v", traced[obs.KeySpanID])
	}
	if _, ok := untraced[obs.KeyTraceID]; ok {
		t.Errorf("a request without traceparent got a trace: %v", untraced)
	}
	// The use case's own step (nothing to undo here) shares the request's
	// trace and span.
	steps := journalRecords(t, &file, "step.failed")
	if len(steps) != 1 || steps[0]["step"] != "undo" || steps[0][obs.KeyTraceID] != traced[obs.KeyTraceID] || steps[0][obs.KeySpanID] != traced[obs.KeySpanID] {
		t.Errorf("undo step = %v", steps)
	}
}

func TestPanicBecomesAnInternalError(t *testing.T) {
	j := obs.NewJournal(obs.JournalOptions{})
	dir := t.TempDir()
	if err := j.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	srv := &Server{Workspace: app.Workspace{}, Version: "1.2.3", Journal: j} // no adapters: the tool panics
	var out bytes.Buffer
	in := modern(1, "tools/call", `"name":"ship_status","arguments":{"offline":true}`) + "\n" + modern(2, "tools/list", "") + "\n"
	if err := srv.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "internal error") || !strings.Contains(lines[1], `"tools"`) {
		t.Fatalf("responses:\n%s", out.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, string(j.RunID)+obs.BlackBoxSuffix))
	if err != nil || !strings.Contains(string(data), "run.panic") {
		t.Fatalf("black box: %v\n%s", err, data)
	}
}
