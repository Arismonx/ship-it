// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fix"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func brokenRepository(calls *[]string) (testkit.FakeGit, *testkit.FakeForge) {
	git := testkit.HealthyRepository("main")
	git.ConfigValues = nil // no identity
	git.MatchingValues = map[string]string{"remote.backup.mirror": "true"}
	git.ScopedConfig = map[string]string{"local:remote.backup.mirror": "true"}
	git.Tracked = []string{"src/a.ts", ".env.local"}
	git.WorkingFiles = map[string]string{".gitignore": "dist/"}
	git.Calls = calls
	return git, &testkit.FakeForge{User: ports.User{Login: "ada", ID: 42, Name: "Ada Lovelace"}}
}

func TestFixListsThenApplies(t *testing.T) {
	var calls []string
	git, forge := brokenRepository(&calls)
	ws, store, log := workspaceOf(git)
	ws.Forge = forge

	report, err := Fix(context.Background(), ws, FixRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []fix.RemedyKind
	for _, r := range report.Remedies {
		kinds = append(kinds, r.Kind)
	}
	if !reflect.DeepEqual(kinds, []fix.RemedyKind{fix.SetIdentity, fix.DisableMirror, fix.UntrackFiles}) || len(calls) != 0 {
		t.Fatalf("remedies %v, calls %q (listing changes nothing)", kinds, calls)
	}

	report, err = Fix(context.Background(), ws, FixRequest{Apply: true})
	if err != nil || !report.Applied {
		t.Fatalf("apply: %+v, %v", report, err)
	}
	want := []string{
		"config global user.name=Ada Lovelace",
		"config global user.email=42+ada@users.noreply.github.com",
		"config local unset remote.backup.mirror",
		`write .gitignore dist/\n# Added by ship fix: never commit these.\n.env\n.env.*\n!.env.example\n`,
		"untrack .env.local",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls =\n%q\nwant\n%q", calls, want)
	}
	if store.Checkpoints[0].Label != "before-fix" || log.Operations[0].Kind != "fix" {
		t.Fatal("a checkpoint and an oplog entry are expected")
	}

	// ship undo puts everything back, in reverse order.
	calls = nil
	if _, err := Undo(context.Background(), ws, UndoRequest{}); err != nil {
		t.Fatal(err)
	}
	wantUndo := []string{
		"write .gitignore dist/",
		"track .env.local",
		"config local remote.backup.mirror=true",
		"config global unset user.email",
		"config global unset user.name",
	}
	if !reflect.DeepEqual(calls, wantUndo) {
		t.Fatalf("undo calls =\n%q\nwant\n%q", calls, wantUndo)
	}
}

func TestFixWithNothingToDo(t *testing.T) {
	ws, _, log := workspaceOf(testkit.HealthyRepository("main"))
	report, err := Fix(context.Background(), ws, FixRequest{Apply: true})
	if err != nil || len(report.Remedies) != 0 || report.Applied || len(log.Operations) != 0 {
		t.Fatalf("report = %+v, %v", report, err)
	}
}
