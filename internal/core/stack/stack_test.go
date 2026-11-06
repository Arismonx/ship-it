// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package stack

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/split"
)

func TestLayers(t *testing.T) {
	files := []split.File{
		{Path: "src/api/cart.go", Lines: 300}, {Path: "src/api/vat.go", Lines: 50},
		{Path: "docs/cart.md", Lines: 40}, {Path: "web/cart.ts", Lines: 120},
		{Path: "README.md", Lines: 5}, {Path: "build/ci.yml", Lines: 10},
	}
	tests := []struct {
		name  string
		max   int
		areas []string
	}{
		{"every area its layer", 5, []string{"src/api/", "web/", "docs/", "build/", "README.md"}},
		{"the smaller areas grouped on top", 3, []string{"src/api/", "web/", "docs/, build/, README.md"}},
		{"never fewer than two", 1, []string{"src/api/", "web/, docs/, build/, README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layers, err := Layers(files, tt.max)
			if err != nil {
				t.Fatal(err)
			}
			var areas []string
			total := 0
			for _, l := range layers {
				areas = append(areas, l.Area)
				total += len(l.Files)
			}
			if !reflect.DeepEqual(areas, tt.areas) || total != len(files) {
				t.Errorf("areas = %q (%d files)", areas, total)
			}
		})
	}
	if _, err := Layers([]split.File{{Path: "src/api/a.go", Lines: 900}, {Path: "src/api/b.go", Lines: 9}}, 5); !errors.Is(err, &fault.Fault{Code: fault.StackNeedsLayers}) {
		t.Errorf("one area: %v", err)
	}
}

func TestChoose(t *testing.T) {
	files := []split.File{{Path: "api/a.go", Lines: 3}, {Path: "web/b.ts", Lines: 4}, {Path: "docs/c.md", Lines: 1}}
	layers, err := Choose(files, [][]string{{"api/"}, {"web/", "docs/c.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 || !reflect.DeepEqual(layers[1].Files, []string{"web/b.ts", "docs/c.md"}) || layers[1].Lines != 5 {
		t.Errorf("layers = %+v", layers)
	}
	for _, sel := range [][][]string{{{"api/"}}, {{"api/"}, {"nowhere/"}}} {
		if _, err := Choose(files, sel); !errors.Is(err, &fault.Fault{Code: fault.StackNeedsLayers}) {
			t.Errorf("%v: %v", sel, err)
		}
	}
}

func stackOp(id string, branches ...string) oplog.Operation {
	rec := &oplog.StackRecord{Base: "main"}
	for _, b := range branches {
		rec.Layers = append(rec.Layers, oplog.StackLayer{Branch: b})
	}
	return oplog.Operation{ID: id, Kind: oplog.Stack, Stack: rec}
}

func TestChains(t *testing.T) {
	ops := []oplog.Operation{ // newest first
		{ID: "5", Kind: oplog.Undo, Undoes: "4"},
		{ID: "4", Kind: oplog.Deliver, CreatedBranch: true, StackParent: "feat/c", After: oplog.RepositoryView{Branch: "feat/undone"}},
		{ID: "3", Kind: oplog.Deliver, CreatedBranch: true, StackParent: "feat/b", After: oplog.RepositoryView{Branch: "feat/c"}},
		{ID: "2", Kind: oplog.Deliver, CreatedBranch: true, After: oplog.RepositoryView{Branch: "fix/other"}},
		stackOp("1", "feat/a", "feat/b"),
	}
	chains := Chains(ops)
	if len(chains) != 1 || !reflect.DeepEqual(chains[0].Branches, []string{"feat/a", "feat/b", "feat/c"}) {
		t.Fatalf("chains = %+v", chains)
	}
	c, ok := Find(chains, "feat/b")
	if !ok || c.Top() != "feat/c" || c.Parent(0) != "main" || c.Parent(2) != "feat/b" {
		t.Errorf("find = %+v %v", c, ok)
	}
	if _, ok := Find(chains, "fix/other"); ok {
		t.Error("fix/other is not in a stack")
	}
	left := c.Without(func(b string) bool { return b == "feat/a" })
	if !reflect.DeepEqual(left.Branches, []string{"feat/b", "feat/c"}) || left.Parent(0) != "main" {
		t.Errorf("without = %+v", left)
	}
}

func TestPlanSync(t *testing.T) {
	c := Chain{Base: "main", Branches: []string{"a", "b", "c"}}
	tests := []struct {
		name   string
		merged []string
		want   SyncPlan
	}{
		{"nothing merged", nil, SyncPlan{Remaining: []SyncStep{
			{Branch: "a", Parent: "main", OldParent: "main"}, {Branch: "b", Parent: "a", OldParent: "a"}, {Branch: "c", Parent: "b", OldParent: "b"}}}},
		{"the bottom merged", []string{"a"}, SyncPlan{Merged: []string{"a"}, Remaining: []SyncStep{
			{Branch: "b", Parent: "main", OldParent: "a", Retarget: true}, {Branch: "c", Parent: "b", OldParent: "b"}}}},
		{"two merged", []string{"a", "b"}, SyncPlan{Merged: []string{"a", "b"}, Remaining: []SyncStep{
			{Branch: "c", Parent: "main", OldParent: "b", Retarget: true}}}},
		{"all merged", []string{"a", "b", "c"}, SyncPlan{Merged: []string{"a", "b", "c"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var states []LayerState
			for _, b := range c.Branches {
				states = append(states, LayerState{Branch: b, Merged: contains(tt.merged, b)})
			}
			if got := PlanSync(c, states); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PlanSync = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestChainNoteAndMergeTarget(t *testing.T) {
	if got := ChainNote(2, 3, 41); got != "Part 2 of 3, after #41." {
		t.Errorf("note = %q", got)
	}
	if got := ChainNote(1, 3, 0); got != "Part 1 of 3." {
		t.Errorf("note = %q", got)
	}
	if MergeTarget(true, []int{4, 5, 6}) != 6 || MergeTarget(false, []int{4, 5, 6}) != 4 || MergeTarget(true, nil) != 0 {
		t.Error("merge target")
	}
}

func TestWithChainNote(t *testing.T) {
	tests := []struct{ body, note, want, old string }{
		{"## Summary\nx\n\nPart 2 of 2, after #1.\n", "Part 2 of 3, after #1.", "## Summary\nx\n\nPart 2 of 3, after #1.\n", "Part 2 of 2, after #1."},
		{"## Summary\nx", "Part 1 of 2.", "## Summary\nx\n\nPart 1 of 2.\n", ""},
		{"x\n\nPart 1 of 3.\nend\n", "", "x\n\nend\n", "Part 1 of 3."},
		{"Not a Part 1 of 2. line", "", "Not a Part 1 of 2. line", ""},
	}
	for _, tt := range tests {
		got, old := WithChainNote(tt.body, tt.note)
		if got != tt.want || old != tt.old {
			t.Errorf("WithChainNote(%q, %q) = %q, %q; want %q, %q", tt.body, tt.note, got, old, tt.want, tt.old)
		}
	}
}

func TestChainsFollowRenames(t *testing.T) {
	ops := []oplog.Operation{ // newest first
		{ID: "3", Kind: oplog.Rename, Before: oplog.RepositoryView{Branch: "feat/c"}, After: oplog.RepositoryView{Branch: "feature/c"}},
		{ID: "2", Kind: oplog.Deliver, StackParent: "feature/b", CreatedBranch: true, After: oplog.RepositoryView{Branch: "feat/c"}},
		{ID: "1", Kind: oplog.StackPublish, Stack: &oplog.StackRecord{Renames: []oplog.StackRename{{From: "feat/b", To: "feature/b"}}}},
		{ID: "0", Kind: oplog.Stack, Stack: &oplog.StackRecord{Base: "main", Layers: []oplog.StackLayer{{Branch: "feat/a"}, {Branch: "feat/b"}}}},
	}
	if got := Chains(ops); len(got) != 1 || !reflect.DeepEqual(got[0].Branches, []string{"feat/a", "feature/b", "feature/c"}) {
		t.Fatalf("chains = %+v", got)
	}
}
