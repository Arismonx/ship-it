// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package oplog

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestPlanUndoOfAStack(t *testing.T) {
	op := Operation{
		Kind: Stack, Checkpoint: "cp",
		Before: RepositoryView{Branch: "main", Head: "base"},
		After:  RepositoryView{Branch: "feat/b", Head: "c2"},
		Stack:  &StackRecord{Base: "main", Layers: []StackLayer{{Branch: "feat/a", Commit: "c1"}, {Branch: "feat/b", Commit: "c2"}}},
	}
	steps, err := PlanUndo(op, op.After)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{Kind: AttachHead, Branch: "main"}, {Kind: ResetIndex, Commit: "base"}, {Kind: RestoreCheckpoint, Checkpoint: "cp"},
		{Kind: DeleteBranch, Branch: "feat/b", Expected: "c2"}, {Kind: DeleteBranch, Branch: "feat/a", Expected: "c1"},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("steps = %+v", steps)
	}
	if !NeedsCleanTree(op) {
		t.Error("undoing a stack restores files: the tree must be clean")
	}
	if _, err := PlanUndo(op, RepositoryView{Branch: "feat/b", Head: "c3"}); !errors.Is(err, &fault.Fault{Code: fault.RepositoryMovedSinceOperation}) {
		t.Errorf("moved: %v", err)
	}
}

func TestPlanUndoOfAPublishedStack(t *testing.T) {
	op := Operation{Kind: StackPublish, Stack: &StackRecord{Native: true, Number: 7, Layers: []StackLayer{
		{Branch: "feat/a", Commit: "c1", PullRequest: 41, Created: true},
		{Branch: "feat/b", Commit: "c2", PullRequest: 42, Created: true},
	}}}
	steps, err := PlanUndo(op, RepositoryView{})
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{Kind: Unstack, Number: 7},
		{Kind: ClosePullRequest, Number: 42, Branch: "feat/b"}, {Kind: DeleteRemoteBranch, Branch: "feat/b", Expected: "c2"},
		{Kind: ClosePullRequest, Number: 41, Branch: "feat/a"}, {Kind: DeleteRemoteBranch, Branch: "feat/a", Expected: "c1"},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("steps = %+v", steps)
	}
	for _, s := range steps {
		if !s.Remote() {
			t.Errorf("%s changes GitHub: it needs consent", s.Kind)
		}
	}
	if u := Undoable([]Operation{op}); len(u) != 1 {
		t.Error("a published stack is undoable")
	}
}

func TestPlanUndoOfAStackSync(t *testing.T) {
	op := Operation{
		Kind: StackSync, Checkpoint: "cp",
		Before: RepositoryView{Branch: "feat/a", Head: "a1"},
		After:  RepositoryView{Branch: "feat/b", Head: "b2"},
		Stack: &StackRecord{Layers: []StackLayer{
			{Branch: "feat/a", Previous: "a1", Deleted: true},
			{Branch: "feat/b", Previous: "b1", Commit: "b2"},
			{Branch: "feat/c", Previous: "c1", Commit: "c1"},
		}},
	}
	steps, err := PlanUndo(op, op.After)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{Kind: RecreateBranch, Branch: "feat/a", Commit: "a1"},
		{Kind: MoveBranch, Branch: "feat/b", Commit: "b1", Expected: "b2"},
		{Kind: AttachHead, Branch: "feat/a"}, {Kind: ResetIndex, Commit: "a1"}, {Kind: RestoreCheckpoint, Checkpoint: "cp"},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("steps = %+v", steps)
	}
}
