// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package oplogstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/core/oplog"
)

type gitDir string

func (d gitDir) GitDir(context.Context) (string, error) { return string(d), nil }

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	ids := []string{"01A", "01B", "01C", "01D"}
	s := New(gitDir(dir))
	s.NewID = func() string { id := ids[0]; ids = ids[1:]; return id }
	s.Now = func() time.Time { return time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC) }
	return s, dir
}

func TestEmptyLog(t *testing.T) {
	s, _ := newStore(t)
	ops, err := s.Recent(context.Background(), 10)
	if err != nil || len(ops) != 0 {
		t.Fatalf("Recent = %v, %v", ops, err)
	}
}

func TestAppendAndRecent(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	first, err := s.Append(ctx, oplog.Operation{Kind: oplog.Deliver, Title: "✨ feat: add vat",
		Before: oplog.RepositoryView{Branch: "main", Head: "h0"}, After: oplog.RepositoryView{Branch: "feat/vat", Head: "c1"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "01A" || first.StartedAt.IsZero() {
		t.Fatalf("Append did not fill id and time: %+v", first)
	}
	if _, err := s.Append(ctx, oplog.Operation{Kind: oplog.Undo, Undoes: first.ID}); err != nil {
		t.Fatal(err)
	}

	ops, err := s.Recent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[0].Kind != oplog.Undo || ops[1].ID != "01A" {
		t.Fatalf("Recent = %+v, want newest first", ops)
	}
	if ops[1].After.Branch != "feat/vat" || ops[1].Title != "✨ feat: add vat" {
		t.Fatalf("round trip lost data: %+v", ops[1])
	}
	if limited, _ := s.Recent(ctx, 1); len(limited) != 1 || limited[0].ID != "01B" {
		t.Fatalf("Recent(1) = %+v", limited)
	}
}

func TestRecentSkipsDamagedAndTemporaryFiles(t *testing.T) {
	ctx := context.Background()
	s, dir := newStore(t)
	if _, err := s.Append(ctx, oplog.Operation{Kind: oplog.Deliver}); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(dir, "ship", "oplog")
	for name, content := range map[string]string{
		"01Z.json":  "{truncated",
		".tmp-1234": "{}",
		"01Y.json":  `{"id":"other","kind":"deliver"}`,
	} {
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ops, err := s.Recent(ctx, 10)
	if err != nil || len(ops) != 1 || ops[0].ID != "01A" {
		t.Fatalf("Recent = %+v, %v", ops, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(logDir, ".tmp-*")); len(leftovers) != 1 {
		t.Fatalf("Append left temporary files behind: %v", leftovers)
	}
}
