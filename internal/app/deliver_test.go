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

func deliverableGit(branch string, calls *[]string) testkit.FakeGit {
	git := testkit.HealthyRepository(branch, "a.go")
	git.ExistingBranches = map[string]bool{"feat/add-login": true}
	git.Calls = calls
	return git
}

func workspaceOf(git ports.Git) (Workspace, *testkit.FakeCheckpointStore, *testkit.FakeOperationLog) {
	store, log := &testkit.FakeCheckpointStore{}, &testkit.FakeOperationLog{}
	return Workspace{Git: git, Checkpoints: store, Operations: log, OperatingSystem: "linux"}, store, log
}

func TestDeliverFromTrunkCreatesAUniqueBranch(t *testing.T) {
	var calls []string
	ws, store, log := workspaceOf(deliverableGit("main", &calls))
	result, err := DeliverLocally(context.Background(), ws,
		DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"switch feat/add-login-2", "stage", "commit ✨ feat: add login\n"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
	if !result.CreatedBranch || result.Branch != "feat/add-login-2" || result.Commit != testkit.FakeCommitID {
		t.Errorf("result = %+v", result)
	}
	if len(store.Checkpoints) != 1 || store.Checkpoints[0].Label != "deliver" {
		t.Errorf("a checkpoint must be taken before changing anything: %+v", store.Checkpoints)
	}
	wantOp := oplog.Operation{
		ID: "op1", Kind: oplog.Deliver, Checkpoint: store.Checkpoints[0].ID,
		Before:        oplog.RepositoryView{Branch: "main", Head: "abc"},
		After:         oplog.RepositoryView{Branch: "feat/add-login-2", Head: testkit.FakeCommitID},
		CreatedBranch: true, Commit: testkit.FakeCommitID, Title: "✨ feat: add login",
	}
	if len(log.Operations) != 1 || !reflect.DeepEqual(log.Operations[0], wantOp) || result.Operation != "op1" {
		t.Errorf("operation log = %+v, want %+v", log.Operations, wantOp)
	}
}

func TestDeliverOnFeatureBranchCommitsInPlace(t *testing.T) {
	var calls []string
	ws, _, log := workspaceOf(deliverableGit("fix/crash", &calls))
	result, err := DeliverLocally(context.Background(), ws,
		DeliveryRequest{Title: "🐛 fix: handle nil config", Branch: "fix/handle-nil-config"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedBranch || result.Branch != "fix/crash" || len(calls) != 2 {
		t.Fatalf("result = %+v, calls = %q", result, calls)
	}
	if op := log.Operations[0]; op.CreatedBranch || op.Before.Head != "abc" || op.After.Head != testkit.FakeCommitID {
		t.Errorf("operation = %+v", op)
	}
}

func TestTheFirstCommitLandsOnTheTrunk(t *testing.T) {
	var calls []string
	git := deliverableGit("main", &calls)
	git.WorkingTree.Branch.Commit = "" // unborn: no commit yet
	ws, _, _ := workspaceOf(git)
	result, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🎉 chore: init", Branch: "chore/init"})
	if err != nil || result.CreatedBranch || result.Branch != "main" || calls[0] != "stage" {
		t.Fatalf("result = %+v, calls %q, %v", result, calls, err)
	}
}

func TestDeliverRefusals(t *testing.T) {
	ctx := context.Background()
	noIdentity := deliverableGit("main", nil)
	noIdentity.ConfigValues = nil
	cases := []struct {
		name string
		git  testkit.FakeGit
		req  DeliveryRequest
		code fault.Code
	}{
		{"doctor blocking", noIdentity, DeliveryRequest{Title: "feat: x", Branch: "feat/x"}, fault.DoctorBlocking},
		{"invalid title", deliverableGit("main", nil), DeliveryRequest{Title: "Add stuff.", Branch: "feat/x"}, fault.MalformedHeader},
		{"invalid branch", deliverableGit("main", nil), DeliveryRequest{Title: "feat: x", Branch: "Feature/X"}, fault.UnknownBranchPrefix},
		{"clean tree", testkit.HealthyRepository("main"), DeliveryRequest{Title: "feat: x", Branch: "feat/x"}, fault.NothingToShip},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ws, _, log := workspaceOf(tt.git)
			_, err := DeliverLocally(ctx, ws, tt.req)
			if !errors.Is(err, &fault.Fault{Code: tt.code}) {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			if len(log.Operations) != 0 {
				t.Fatalf("a refused delivery changed nothing and must not be logged: %+v", log.Operations)
			}
		})
	}
}

func TestDeliverFollowsTheGitmojiHabit(t *testing.T) {
	for _, tt := range []struct {
		name, setting string
		history       []string
		want          string
	}{
		{"young repository", "", nil, "commit 🐛 fix: handle nil\n"},
		{"history without emoji", "", []string{"fix: a", "feat: b", "docs: c", "chore: d", "test: e"}, "commit fix: handle nil\n"},
		{"setting beats the history", `{"ship": {"commits": {"gitmoji": "after-type"}}}`, []string{"fix: a", "feat: b", "docs: c", "chore: d", "test: e"}, "commit fix: 🐛 handle nil\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			git := deliverableGit("fix/crash", &calls)
			git.History = tt.history
			ws, _, _ := workspaceOf(git)
			ws.Config = &testkit.FakeConfigFiles{Repository: tt.setting}
			if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "fix: handle nil", Branch: "fix/handle-nil"}); err != nil {
				t.Fatal(err)
			}
			if calls[len(calls)-1] != tt.want {
				t.Fatalf("calls = %q, want %q", calls, tt.want)
			}
		})
	}
}

func TestTheBranchTicketEndsTheTitles(t *testing.T) {
	var calls []string
	git := deliverableGit("feat/acme-42-add-login", &calls)
	ws, _, _ := workspaceOf(git)
	ws.Config = &testkit.FakeConfigFiles{User: `{"contexts": [{"match": {"owner": "acme"}, "ship": {"issues": {"trackerKeys": ["ACME"]}}}]}`}
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat(auth): add login", Branch: "feat/add-login"}); err != nil {
		t.Fatal(err)
	}
	if got := calls[len(calls)-1]; got != "commit ✨ feat(auth): add login (ACME-42)\n" {
		t.Fatalf("commit = %q", got)
	}

	forge := &testkit.FakeForge{}
	pub := publishableGit(nil)
	pub.WorkingTree.Branch.Name = "feat/acme-42-add-login"
	pub.CommitMessages = []string{"✨ feat(auth): add login"} // made outside ship-it
	pws, _ := publishWorkspace(pub, forge)
	pws.Config = ws.Config
	if _, err := Publish(context.Background(), pws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := forge.Created[0].Title; got != "✨ feat(auth): add login (ACME-42)" {
		t.Fatalf("pull request title = %q", got)
	}
}

func TestWhyAndTestedGoIntoTheCommit(t *testing.T) {
	var calls []string
	ws, _, _ := workspaceOf(deliverableGit("fix/crash", &calls))
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{
		Title: "fix: handle nil config", Branch: "fix/handle-nil-config",
		Why: "A missing file crashed the start.", Tested: "ran ship with no config file",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "commit 🐛 fix: handle nil config\n\nA missing file crashed the start.\n\nTested: ran ship with no config file\n"
	if got := calls[len(calls)-1]; got != want {
		t.Fatalf("commit = %q, want %q", got, want)
	}
}

func TestDeliveryBranchCheckKnowsTheTrackerKeys(t *testing.T) {
	git := deliverableGit("main", nil)
	ws, _, _ := workspaceOf(git)
	ws.Config = &testkit.FakeConfigFiles{User: `{"contexts": [{"match": {"owner": "acme"}, "ship": {"issues": {"trackerKeys": ["ACME"]}}}]}`}
	check := DeliveryBranchCheck(context.Background(), ws, nil)
	if c := check("feat/ACME-42-add-login"); !c.Valid() {
		t.Fatalf("a ticket of a known key is valid: %+v", c.Fault)
	}
	if c := check("Feature/Add_Login"); c.Valid() {
		t.Fatal("an invalid branch passed")
	}
	if CheckBranch("feat/ACME-42-add-login").Valid() {
		t.Fatal("without tracker keys the upper-case ticket is refused")
	}
}

func TestDeliverTreatsTheRemoteDefaultBranchAsATrunk(t *testing.T) {
	var calls []string
	git := deliverableGit("production", &calls)
	git.DefaultBranch = "production"
	ws, _, _ := workspaceOf(git)
	result, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if err != nil || !result.CreatedBranch || result.Branch == "production" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

// readCounter counts the reads a plan and its delivery make.
type readCounter struct {
	ports.Git
	reads map[string]int
}

func (c *readCounter) Status(ctx context.Context) (ports.WorkingTreeStatus, error) {
	c.reads["status"]++
	return c.Git.Status(ctx)
}

func (c *readCounter) DiffFromHead(ctx context.Context) (string, error) {
	c.reads["diff"]++
	return c.Git.DiffFromHead(ctx)
}

func (c *readCounter) RemoteDefaultBranch(ctx context.Context, remote string) (string, error) {
	c.reads["default branch"]++
	return c.Git.RemoteDefaultBranch(ctx, remote)
}

func (c *readCounter) RecentSubjects(ctx context.Context, limit int) ([]string, error) {
	c.reads["history"]++
	return c.Git.RecentSubjects(ctx, limit)
}

// TestADeliveryReusesWhatThePlanRead: right after the plan (--yes, an
// agent), the delivery and its doctor ask git nothing the plan knows; after
// a review (no Reads) they read the repository again, once for both.
func TestADeliveryReusesWhatThePlanRead(t *testing.T) {
	for _, tt := range []struct {
		name  string
		reuse bool
		want  map[string]int
	}{
		{"at once", true, map[string]int{"status": 1, "diff": 1, "default branch": 1, "history": 1}},
		{"after a review", false, map[string]int{"status": 2, "diff": 2, "default branch": 2, "history": 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			fake := deliverableGit("main", &calls)
			fake.DiffText = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
			git := &readCounter{Git: fake, reads: map[string]int{}}
			ws, _, _ := workspaceOf(git)
			view, err := ProposePlan(context.Background(), ws, PlanRequest{})
			if err != nil {
				t.Fatal(err)
			}
			req := DeliveryRequest{Title: view.Title, Branch: view.Branch}
			if tt.reuse {
				req.Reads = view.Reads()
			}
			if _, err := DeliverLocally(context.Background(), ws, req); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(git.reads, tt.want) {
				t.Errorf("reads = %v, want %v", git.reads, tt.want)
			}
		})
	}
}
