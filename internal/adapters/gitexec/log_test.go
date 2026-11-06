// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

func TestLogArgs(t *testing.T) {
	tests := []struct{ in, want []string }{
		{[]string{"config", "--global", "user.email", "dev@example.com"}, []string{"config", "--global", "user.email", redacted}},
		{[]string{"config", "--get", "remote.origin.url"}, []string{"config", "--get", "remote.origin.url"}},
		{[]string{"-c", "http.extraheader=AUTHORIZATION: basic xyz", "fetch"}, []string{"-c", "http.extraheader=" + redacted, "fetch"}},
		{[]string{"-c", "core.quotePath=false", "diff", "HEAD"}, []string{"-c", "core.quotePath=false", "diff", "HEAD"}},
		{[]string{"ls-tree", "-r", "HEAD", "--", "config", "x"}, []string{"ls-tree", "-r", "HEAD", "--", "config", "x"}},
	}
	for _, tt := range tests {
		if got := LogArgs(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("LogArgs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRunLogsEachInvocation(t *testing.T) {
	c := newRepo(t)
	var buf bytes.Buffer
	c.Log = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()
	if _, err := c.Commit(ctx, "✨ feat: secret plans inside\n", ports.CommitOptions{}); err == nil {
		t.Fatal("commit with nothing staged should fail")
	}
	write(t, c.Dir, "README.md", "private file content\n")
	if _, err := c.DiffFromHead(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SetConfig(ctx, "user.email", "someone@example.com", false); err != nil {
		t.Fatal(err)
	}

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		records = append(records, rec)
	}
	if len(records) != 3 {
		t.Fatalf("want 3 records, got %d:\n%s", len(records), buf.String())
	}
	commit := records[0]
	if commit["msg"] != "git.exec" || commit["step"] != "git" || commit["exit_code"] != float64(1) || commit["level"] != "DEBUG" {
		t.Errorf("commit record = %v", commit)
	}
	if _, ok := commit["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
	if records[1]["exit_code"] != float64(0) || records[1]["detail"] != nil {
		t.Errorf("diff record = %v", records[1])
	}
	for _, leak := range []string{"secret plans", "private file content", "someone@example.com"} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("%q leaked into the log:\n%s", leak, buf.String())
		}
	}
}
