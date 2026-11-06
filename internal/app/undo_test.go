// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

var delivered = oplog.Operation{
	ID: "op1", Kind: oplog.Deliver, Checkpoint: "cp1", Title: "✨ feat: add vat",
	Before:        oplog.RepositoryView{Branch: "main", Head: "h0"},
	After:         oplog.RepositoryView{Branch: "feat/add-vat", Head: "c1"},
	CreatedBranch: true, Commit: "c1",
}

func gitAfterDelivery(calls *[]string) testkit.FakeGit {
	git := testkit.HealthyRepository("feat/add-vat")
	git.WorkingTree.Branch.Commit = "c1"
	git.Calls = calls
	return git
}

func TestUndoRevertsTheLastDelivery(t *testing.T) {
	var calls []string
	ws, store, log := workspaceOf(gitAfterDelivery(&calls))
	log.Operations = []oplog.Operation{delivered}
	store.Checkpoints = []ports.Checkpoint{{ID: "cp1", Label: "deliver"}}

	result, err := Undo(context.Background(), ws, UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"attach main", "reset-index h0", "delete feat/add-vat at c1"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
	if store.Restored != "cp1" {
		t.Errorf("checkpoint restored = %q, want cp1", store.Restored)
	}
	if store.Checkpoints[0].Label != "before-undo" {
		t.Errorf("a safety checkpoint must be taken first: %+v", store.Checkpoints)
	}
	if result.Branch != "main" || result.DeletedBranch != "feat/add-vat" || result.Undone != "op1" {
		t.Errorf("result = %+v", result)
	}
	undo := log.Operations[0]
	if undo.Kind != oplog.Undo || undo.Undoes != "op1" || undo.After != delivered.Before || result.Operation != undo.ID {
		t.Errorf("undo entry = %+v", undo)
	}

	// The same delivery cannot be undone twice.
	if _, err := Undo(context.Background(), ws, UndoRequest{}); !errors.Is(err, &fault.Fault{Code: fault.NothingToUndo}) {
		t.Fatalf("second undo error = %v", err)
	}
}

func TestUndoRefusals(t *testing.T) {
	moved := gitAfterDelivery(nil)
	moved.WorkingTree.Branch.Commit = "c2"
	dirty := gitAfterDelivery(nil)
	dirty.WorkingTree.Files = []ports.FileChange{{Path: "a.go", State: ports.Tracked, Unstaged: ports.Modified}}
	cases := []struct {
		name string
		git  testkit.FakeGit
		ops  []oplog.Operation
		code fault.Code
	}{
		{"empty log", gitAfterDelivery(nil), nil, fault.NothingToUndo},
		{"commit added on top", moved, []oplog.Operation{delivered}, fault.RepositoryMovedSinceOperation},
		{"uncommitted changes", dirty, []oplog.Operation{delivered}, fault.UndoNeedsCleanTree},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			tt.git.Calls = &calls
			ws, store, log := workspaceOf(tt.git)
			log.Operations = tt.ops
			store.Checkpoints = []ports.Checkpoint{{ID: "cp1"}}
			_, err := Undo(context.Background(), ws, UndoRequest{})
			if !errors.Is(err, &fault.Fault{Code: tt.code}) {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			if len(calls) != 0 || len(store.Checkpoints) != 1 {
				t.Fatalf("a refused undo must change nothing: calls %q, checkpoints %d", calls, len(store.Checkpoints))
			}
		})
	}
}
