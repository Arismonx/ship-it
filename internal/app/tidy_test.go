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

// afterMerge: on feat/vat whose pull request #7 was merged, with work in
// progress; fix/old is another merged branch; feat/next is not merged.
func afterMerge(calls *[]string) (testkit.FakeGit, *testkit.FakeForge) {
	git := testkit.HealthyRepository("feat/vat", "notes.md")
	git.Calls = calls
	git.DefaultBranch = "main"
	git.Refs = map[string]string{"refs/heads/feat/vat": "c1", "refs/heads/fix/old": "c9", "refs/heads/feat/next": "n1", "refs/heads/main": "m0"}
	forge := &testkit.FakeForge{
		Latest: map[string]ports.PullRequest{"feat/vat": {Number: 7, Merged: true, HeadBranch: "feat/vat", HeadSHA: "c1"}},
		Merged: []ports.PullRequest{
			{Number: 7, Merged: true, HeadBranch: "feat/vat", HeadSHA: "c1"},
			{Number: 5, Merged: true, HeadBranch: "fix/old", HeadSHA: "c9"},
		},
	}
	return git, forge
}

func TestTidyLeavesAMergedBranch(t *testing.T) {
	var calls []string
	git, forge := afterMerge(&calls)
	ws, store, log := workspaceOf(git)
	ws.Forge = forge

	result, err := Tidy(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch origin", "stash", "switch-to main", "fast-forward origin/main", "stash-pop",
		"delete feat/vat at c1", "delete fix/old at c9"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls =\n%q\nwant\n%q", calls, want)
	}
	if result.LeftBranch != "feat/vat" || result.PullRequest != 7 || !reflect.DeepEqual(result.Deleted, []string{"feat/vat", "fix/old"}) {
		t.Errorf("result = %+v", result)
	}
	if store.Checkpoints[0].Label != "before-tidy" {
		t.Error("a checkpoint must be taken before leaving the branch")
	}
	op := log.Operations[0]
	if op.Kind != oplog.Tidy || len(op.DeletedBranches) != 2 || op.DeletedBranches[1] != (oplog.BranchRef{Name: "fix/old", Commit: "c9"}) {
		t.Fatalf("operation = %+v", op)
	}

	// ship undo recreates both branches.
	calls = nil
	undone, err := Undo(context.Background(), ws, UndoRequest{})
	if err != nil || !reflect.DeepEqual(undone.Recreated, []string{"feat/vat", "fix/old"}) {
		t.Fatalf("undo = %+v, %v", undone, err)
	}
	if !reflect.DeepEqual(calls, []string{"recreate feat/vat at c1", "recreate fix/old at c9"}) {
		t.Fatalf("undo calls = %q", calls)
	}
}

func TestTidyStaysWhenTheWorkWouldConflict(t *testing.T) {
	var calls []string
	git, forge := afterMerge(&calls)
	git.StashConflicts = true
	ws, _, _ := workspaceOf(git)
	ws.Forge = forge

	result, err := Tidy(context.Background(), ws)
	if err != nil || !result.StayedBecauseOfConflicts {
		t.Fatalf("result = %+v, %v", result, err)
	}
	// All or nothing: the stash went back on the same commit; only the other
	// merged branch is cleaned.
	want := []string{"fetch origin", "stash", "stash-pop", "delete fix/old at c9"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q", calls)
	}
}

func TestTidyKeepsCommitsAddedAfterTheMerge(t *testing.T) {
	var calls []string
	git, forge := afterMerge(&calls)
	git.Refs["refs/heads/feat/vat"] = "c2" // a commit made after the merge
	git.WorkingTree.Files = nil
	ws, _, _ := workspaceOf(git)
	ws.Forge = forge

	result, err := Tidy(context.Background(), ws)
	if err != nil || !reflect.DeepEqual(result.Kept, []string{"feat/vat"}) || !reflect.DeepEqual(result.Deleted, []string{"fix/old"}) {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestTidyOffline(t *testing.T) {
	git, forge := afterMerge(nil)
	git.FetchErr = errors.New("could not resolve host")
	ws, _, log := workspaceOf(git)
	ws.Forge = forge
	_, err := Tidy(context.Background(), ws)
	if !errors.Is(err, &fault.Fault{Code: fault.FetchFailed}) || len(log.Operations) != 0 {
		t.Fatalf("error = %v", err)
	}
}

func TestTidyOnAnOpenPullRequestOnlyCleansOthers(t *testing.T) {
	var calls []string
	git, forge := afterMerge(&calls)
	forge.Latest["feat/vat"] = ports.PullRequest{Number: 8, State: "open", HeadBranch: "feat/vat", HeadSHA: "c1"}
	ws, _, _ := workspaceOf(git)
	ws.Forge = forge
	result, err := Tidy(context.Background(), ws)
	if err != nil || result.LeftBranch != "" || !reflect.DeepEqual(result.Deleted, []string{"fix/old"}) {
		t.Fatalf("result = %+v, %v, calls %q", result, err, calls)
	}
}
