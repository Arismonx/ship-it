// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestLog(t *testing.T) {
	ws, _, log := workspaceOf(testkit.HealthyRepository("main"))
	log.Operations = []oplog.Operation{
		{ID: "op4", Kind: oplog.Undo, Undoes: "op3"},
		{ID: "op3", Kind: oplog.Deliver, Title: "✨ feat: b", After: oplog.RepositoryView{Branch: "feat/b"}},
		{ID: "op2", Kind: oplog.Tidy, DeletedBranches: []oplog.BranchRef{{Name: "feat/a", Commit: "c1"}}},
		{ID: "op1", Kind: oplog.Deliver, Title: "✨ feat: a"},
	}
	entries, err := Log(context.Background(), ws, 10)
	if err != nil || len(entries) != 4 {
		t.Fatalf("Log = %+v, %v", entries, err)
	}
	if entries[1].UndoneBy != "op4" || entries[1].Next {
		t.Errorf("op3 is undone: %+v", entries[1])
	}
	if !entries[2].Next || entries[2].Deleted[0] != "feat/a" {
		t.Errorf("op2 is what ship undo reverts next: %+v", entries[2])
	}
}
