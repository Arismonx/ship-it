// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package oplog

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
)

var fromTrunk = Operation{
	ID: "op2", Kind: Deliver, Checkpoint: "cp1",
	Before:        RepositoryView{Branch: "main", Head: "h0"},
	After:         RepositoryView{Branch: "feat/vat", Head: "c1"},
	CreatedBranch: true, Commit: "c1",
}

func TestPlanUndo(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		want []Step
	}{
		{
			name: "new branch from the trunk",
			op:   fromTrunk,
			want: []Step{
				{Kind: AttachHead, Branch: "main"},
				{Kind: ResetIndex, Commit: "h0"},
				{Kind: RestoreCheckpoint, Checkpoint: "cp1"},
				{Kind: DeleteBranch, Branch: "feat/vat", Expected: "c1"},
			},
		},
		{
			name: "new branch from a detached head",
			op: Operation{Kind: Deliver, CreatedBranch: true,
				Before: RepositoryView{Head: "h0", Detached: true}, After: RepositoryView{Branch: "feat/vat", Head: "c1"}},
			want: []Step{
				{Kind: DetachHead, Commit: "h0"},
				{Kind: ResetIndex, Commit: "h0"},
				{Kind: DeleteBranch, Branch: "feat/vat", Expected: "c1"},
			},
		},
		{
			name: "commit on an existing branch",
			op: Operation{Kind: Deliver, Checkpoint: "cp1",
				Before: RepositoryView{Branch: "fix/x", Head: "h0"}, After: RepositoryView{Branch: "fix/x", Head: "c1"}},
			want: []Step{
				{Kind: MoveBranch, Branch: "fix/x", Commit: "h0", Expected: "c1"},
				{Kind: ResetIndex, Commit: "h0"},
				{Kind: RestoreCheckpoint, Checkpoint: "cp1"},
			},
		},
		{
			name: "first commit of an existing branch",
			op: Operation{Kind: Deliver,
				Before: RepositoryView{Branch: "feat/x"}, After: RepositoryView{Branch: "feat/x", Head: "c1"}},
			want: []Step{
				{Kind: DeleteBranch, Branch: "feat/x", Expected: "c1"},
				{Kind: ResetIndex},
			},
		},
		{
			name: "interrupted before the commit",
			op: Operation{Kind: Deliver, CreatedBranch: true,
				Before: RepositoryView{Branch: "main", Head: "h0"}, After: RepositoryView{Branch: "feat/vat", Head: "h0"}},
			want: []Step{
				{Kind: AttachHead, Branch: "main"},
				{Kind: ResetIndex, Commit: "h0"},
				{Kind: DeleteBranch, Branch: "feat/vat", Expected: "h0"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := PlanUndo(tt.op, tt.op.After)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(steps, tt.want) {
				t.Fatalf("steps =\n%+v\nwant\n%+v", steps, tt.want)
			}
		})
	}
}

func TestUndoAPartialDeliveryLeavesTheFilesAlone(t *testing.T) {
	op := fromTrunk
	op.Only = []string{"src/api/"}
	steps, _ := PlanUndo(op, op.After)
	for _, s := range steps {
		if s.Kind == RestoreCheckpoint {
			t.Fatalf("a partial delivery must not restore its checkpoint: %+v", steps)
		}
	}
	if NeedsCleanTree(op) {
		t.Fatal("the rest of the work is uncommitted on purpose")
	}
}

func TestPlanUndoRefusesWhenTheRepositoryMoved(t *testing.T) {
	for _, current := range []RepositoryView{
		{Branch: "feat/vat", Head: "c2"}, // a commit was added on top
		{Branch: "main", Head: "h0"},     // the user switched branch
		{Head: "c1", Detached: true},     // the user detached HEAD
	} {
		_, err := PlanUndo(fromTrunk, current)
		if !errors.Is(err, &fault.Fault{Code: fault.RepositoryMovedSinceOperation}) {
			t.Errorf("current %+v: error = %v", current, err)
		}
	}
}

func TestPlanUndoRefusesAnUndoEntry(t *testing.T) {
	_, err := PlanUndo(Operation{Kind: Undo}, RepositoryView{})
	if !errors.Is(err, &fault.Fault{Code: fault.NothingToUndo}) {
		t.Fatalf("error = %v", err)
	}
}

func TestPublishedWorkIsNotUndoneLocally(t *testing.T) {
	log := []Operation{{ID: "op3", Kind: Publish, PullRequestURL: "https://github.com/o/r/pull/7"}, fromTrunk}
	last, _ := LastUndoable(log)
	_, err := PlanUndo(last, fromTrunk.After)
	if !errors.Is(err, &fault.Fault{Code: fault.UndoOfPublishedWork}) {
		t.Fatalf("error = %v", err)
	}
}

func TestUndoAPublicationClosesWhatItOpened(t *testing.T) {
	op := Operation{Kind: Publish, PullRequest: 8, PullRequestCreated: true, Commit: "c1",
		After: RepositoryView{Branch: "feat/vat", Head: "c1"}}
	steps, err := PlanUndo(op, RepositoryView{})
	want := []Step{{Kind: ClosePullRequest, Number: 8}, {Kind: DeleteRemoteBranch, Branch: "feat/vat", Expected: "c1"}}
	if err != nil || !reflect.DeepEqual(steps, want) || !steps[0].Remote() || !steps[1].Remote() {
		t.Fatalf("steps = %+v, %v", steps, err)
	}
}

func TestUndoAPublicationPutsTheLinkedIssueBack(t *testing.T) {
	linked := &LinkedIssueChange{Issue: 42, Project: forge.Project{Owner: "acme", Kind: forge.ProjectOfOrganization, Number: 7},
		Item: 500, Field: 2, Before: "", After: "p", Assigned: "grace"}
	op := Operation{Kind: Publish, PullRequest: 8, PullRequestCreated: true, Commit: "c1",
		After: RepositoryView{Branch: "feat/vat", Head: "c1"}, LinkedIssue: linked}
	steps, err := PlanUndo(op, RepositoryView{})
	want := []Step{
		{Kind: ClosePullRequest, Number: 8}, {Kind: DeleteRemoteBranch, Branch: "feat/vat", Expected: "c1"},
		{Kind: UnassignIssue, Number: 42, Assignee: "grace"},
		{Kind: RestoreProjectField, Project: &linked.Project, Number: 42, Item: 500, Field: 2, Value: "", Expected: "p"},
	}
	if err != nil || !reflect.DeepEqual(steps, want) || !steps[2].Remote() || !steps[3].Remote() {
		t.Fatalf("steps = %+v, %v", steps, err)
	}
	// Only assigned: no project step.
	op.LinkedIssue = &LinkedIssueChange{Issue: 42, Assigned: "grace"}
	if steps, _ := PlanUndo(op, RepositoryView{}); len(steps) != 3 || steps[2].Kind != UnassignIssue {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestUndoAFix(t *testing.T) {
	op := Operation{Kind: Fix,
		ConfigChanges: []ConfigChange{{Key: "user.name", Global: true}, {Key: "core.longpaths", Old: "false", HadOld: true}},
		Untracked:     []string{".env.local"},
		FilesBefore:   []FileBefore{{Path: ".gitignore", Content: "dist/\n", Existed: true}},
	}
	steps, err := PlanUndo(op, RepositoryView{})
	if err != nil || len(steps) != 4 {
		t.Fatalf("steps = %+v, %v", steps, err)
	}
	if steps[0].Kind != RestoreFile || steps[1].Kind != TrackFiles || steps[2].Config.Key != "core.longpaths" || steps[3].Config.Key != "user.name" {
		t.Fatalf("order = %+v", steps)
	}
	if NeedsCleanTree(op) {
		t.Fatal("undoing a fix touches no work in progress")
	}
}

func TestUndoATidyRecreatesTheBranches(t *testing.T) {
	op := Operation{Kind: Tidy, DeletedBranches: []BranchRef{{"feat/a", "c1"}, {"fix/b", "c2"}}}
	steps, err := PlanUndo(op, RepositoryView{Branch: "main", Head: "anything"})
	want := []Step{{Kind: RecreateBranch, Branch: "feat/a", Commit: "c1"}, {Kind: RecreateBranch, Branch: "fix/b", Commit: "c2"}}
	if err != nil || !reflect.DeepEqual(steps, want) || NeedsCleanTree(op) {
		t.Fatalf("steps = %+v, %v", steps, err)
	}
}

func TestUndoAConfigEditRestoresTheFile(t *testing.T) {
	edit := &ConfigFileEdit{Scope: "repository", Path: "/r/.github/ship.json", Key: "ship.publish.mode",
		Before: "{\n  // ours\n}\n", BeforeExisted: true, After: "{\n  // ours\n  \"ship\": {\"publish\": {\"mode\": \"never\"}}\n}\n"}
	op := Operation{ID: "op1", Kind: Configure, Title: "ship.publish.mode", ConfigFile: edit}
	steps, err := PlanUndo(op, RepositoryView{Branch: "anywhere", Head: "moved"})
	if err != nil || !reflect.DeepEqual(steps, []Step{{Kind: RestoreConfigFile, ConfigFile: edit}}) || NeedsCleanTree(op) {
		t.Fatalf("steps = %+v, %v", steps, err)
	}
	if next, ok := LastUndoable([]Operation{op}); !ok || next.ID != "op1" {
		t.Fatal("a configuration edit is undoable")
	}
	if _, err := PlanUndo(Operation{Kind: Configure}, RepositoryView{}); !errors.Is(err, &fault.Fault{Code: fault.NothingToUndo}) {
		t.Fatalf("an edit without its file: %v", err)
	}
}

func TestUndoARenameGivesTheNameBack(t *testing.T) {
	op := Operation{ID: "r", Kind: Rename,
		Before: RepositoryView{Branch: "feat/login", Head: "c1"}, After: RepositoryView{Branch: "feature/login", Head: "c1"}}
	steps, err := PlanUndo(op, op.After)
	want := []Step{{Kind: RenameBranch, Branch: "feature/login", Target: "feat/login", Expected: "c1"}}
	if err != nil || !reflect.DeepEqual(steps, want) {
		t.Fatalf("PlanUndo = %+v, %v", steps, err)
	}
	if _, err := PlanUndo(op, RepositoryView{Branch: "feature/login", Head: "c2"}); !errors.Is(err, &fault.Fault{Code: fault.RepositoryMovedSinceOperation}) {
		t.Fatalf("a commit made since must stop the undo: %v", err)
	}
	if got, ok := LastUndoable([]Operation{op}); !ok || got.ID != "r" {
		t.Fatal("a rename is undoable")
	}
}

func TestLastUndoable(t *testing.T) {
	log := []Operation{
		{ID: "op3", Kind: Undo, Undoes: "op2"},
		fromTrunk,
		{ID: "op1", Kind: Deliver},
	}
	op, ok := LastUndoable(log)
	if !ok || op.ID != "op1" {
		t.Fatalf("LastUndoable = %+v, %v; want op1 (op2 is already undone)", op, ok)
	}
	if _, ok := LastUndoable([]Operation{{ID: "u", Kind: Undo, Undoes: "op1"}, {ID: "op1", Kind: Deliver}}); ok {
		t.Fatal("everything is undone")
	}
}
