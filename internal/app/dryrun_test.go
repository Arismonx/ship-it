// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// rehearsedWorkspace wraps every port of ws in a guard that fails the test
// on any write, then makes it a dry run: the use cases must never reach a
// guard's write.
func rehearsedWorkspace(t *testing.T, ws Workspace) Workspace {
	t.Helper()
	if ws.Git != nil {
		ws.Git = testkit.GuardGit(t, ws.Git)
	}
	if ws.Forge != nil {
		ws.Forge = testkit.GuardForge(t, ws.Forge)
	}
	if ws.Checkpoints != nil {
		ws.Checkpoints = testkit.GuardCheckpoints(t, ws.Checkpoints)
	}
	if ws.Operations != nil {
		ws.Operations = testkit.GuardOperations(t, ws.Operations)
	}
	if ws.Config != nil {
		ws.Config = testkit.GuardConfig(t, ws.Config)
	}
	if ws.Tracker != nil {
		ws.Tracker = testkit.GuardTracker(t, ws.Tracker)
	}
	if ws.RoadmapMemory != nil {
		ws.RoadmapMemory = testkit.GuardRoadmapMemory(t, ws.RoadmapMemory)
	}
	if ws.Projects != nil {
		ws.Projects = testkit.GuardProjects(t, ws.Projects)
	}
	if ws.Stacks != nil {
		ws.Stacks = testkit.GuardStacks(t, ws.Stacks)
	}
	return DryRun(ws)
}

func plannedKeys(r Rehearsal) []string {
	keys := make([]string, len(r.Planned))
	for i, s := range r.Planned {
		keys[i] = strings.TrimPrefix(s.Key, "app.dryrun.")
	}
	return keys
}

func wantPlan(t *testing.T, r Rehearsal, want ...string) {
	t.Helper()
	if !r.DryRun {
		t.Error("the result does not say it is a dry run")
	}
	if got := plannedKeys(r); !slices.Equal(got, want) {
		t.Errorf("planned =\n%q\nwant\n%q", got, want)
	}
}

func TestDryRunOfShipDeliversThenPublishesNothing(t *testing.T) {
	var calls []string
	git := deliverableGit("main", &calls)
	git.WorkingFiles = map[string]string{"package.json": `{"scripts": {"lint": "eslint .", "test": "vitest"}}`}
	forge := &testkit.FakeForge{}
	real, store, log := workspaceOf(git)
	real.Forge, real.Runner, real.Config = forge, &testkit.FakeRunner{}, &testkit.FakeConfigFiles{}
	ws := rehearsedWorkspace(t, real)

	delivery, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, delivery.Rehearsal, "checkpoint", "create_branch", "commit", "log")
	if delivery.Branch != "feat/add-login-2" || !delivery.CreatedBranch || delivery.Commit != "" || delivery.Operation != "" {
		t.Errorf("delivery = %+v (the branch it would create, no commit id)", delivery)
	}

	// The publication sees the repository as the delivery would leave it.
	publication, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, publication.Rehearsal, "fetch", "gate", "gate", "push", "open_pull_request", "log")
	push := publication.Planned[3]
	if push.Params["branch"] != "feat/add-login-2" || !push.Remote {
		t.Errorf("push = %+v", push)
	}
	if pr := publication.Planned[4]; pr.Params["title"] != "✨ feat: add login" || pr.Params["base"] != "main" {
		t.Errorf("pull request = %+v", pr)
	}
	if publication.PullRequest.Number != 0 || publication.PullRequest.URL != "" || !publication.Created {
		t.Errorf("publication = %+v", publication)
	}

	if len(calls) != 0 || len(forge.Created) != 0 || len(store.Checkpoints) != 0 || len(log.Operations) != 0 {
		t.Errorf("something changed: git %q, pull requests %v, checkpoints %v, log %v", calls, forge.Created, store.Checkpoints, log.Operations)
	}
	if runner := real.Runner.(*testkit.FakeRunner); len(runner.Calls) != 0 {
		t.Errorf("gates ran: %q", runner.Calls)
	}
}

func TestDryRunRefusesWhatTheRealRunRefuses(t *testing.T) {
	git := deliverableGit("main", nil)
	// Assembled at run time so that this file never trips the scanner itself.
	key := strings.Join([]string{"AK", "IA", "IOSFODNN7ABCDEFG"}, "")
	git.DiffText = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,2 @@\n package a\n+aws_access_key_id = " + key + "\n"
	real, _, _ := workspaceOf(git)
	ws := rehearsedWorkspace(t, real)
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if _, ok := SecretsIn(err); !ok || !IsGateRefusal(err) {
		t.Fatalf("error = %v, want the secret refusal of the real run", err)
	}
}

func TestDryRunOfUndo(t *testing.T) {
	var calls []string
	real, store, log := workspaceOf(gitAfterDelivery(&calls))
	log.Operations = []oplog.Operation{delivered}
	store.Checkpoints = []ports.Checkpoint{{ID: "cp1", Label: "deliver"}}

	result, err := Undo(context.Background(), rehearsedWorkspace(t, real), UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, result.Rehearsal, "checkpoint", "attach_head", "reset_index", "restore_checkpoint", "delete_branch", "log")
	if result.Undone != "op1" || result.Branch != "main" || result.Operation != "" {
		t.Errorf("result = %+v", result)
	}
	if len(calls) != 0 || store.Restored != "" || len(log.Operations) != 1 {
		t.Errorf("something changed: %q, restored %q, log %v", calls, store.Restored, log.Operations)
	}
}

func TestDryRunOfARemoteUndoNeedsNoConsent(t *testing.T) {
	real, _, log := workspaceOf(gitAfterDelivery(nil))
	real.Forge = &testkit.FakeForge{}
	log.Operations = []oplog.Operation{{
		ID: "op2", Kind: oplog.Publish, Remote: "origin", PullRequest: 7, PullRequestCreated: true,
		Before: oplog.RepositoryView{Branch: "feat/add-vat", Head: "c1"}, After: oplog.RepositoryView{Branch: "feat/add-vat", Head: "c1"},
		Commit: "c1",
	}}
	result, err := Undo(context.Background(), rehearsedWorkspace(t, real), UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	keys := plannedKeys(result.Rehearsal)
	if keys[0] != "needs_consent" || !slices.Contains(keys, "close_pull_request") || !slices.Contains(keys, "delete_remote_branch") {
		t.Errorf("planned = %q", keys)
	}
	// Without a dry run, the same undo asks first.
	if _, err := Undo(context.Background(), real, UndoRequest{}); !errors.Is(err, &fault.Fault{Code: fault.UndoNeedsConsent}) {
		t.Fatalf("real undo error = %v", err)
	}
}

func TestDryRunOfTidy(t *testing.T) {
	var calls []string
	git, forge := afterMerge(&calls)
	real, store, log := workspaceOf(git)
	real.Forge = forge

	result, err := Tidy(context.Background(), rehearsedWorkspace(t, real))
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, result.Rehearsal, "fetch", "checkpoint", "stash", "conflicts_unchecked", "switch", "fast_forward",
		"stash_pop", "delete_branch", "delete_branch", "log")
	if result.LeftBranch != "feat/vat" || !slices.Equal(result.Deleted, []string{"feat/vat", "fix/old"}) {
		t.Errorf("result = %+v", result)
	}
	if len(calls) != 0 || len(store.Checkpoints) != 0 || len(log.Operations) != 0 {
		t.Errorf("something changed: %q", calls)
	}
}

func TestDryRunOfFix(t *testing.T) {
	var calls []string
	git, forge := brokenRepository(&calls)
	real, _, log := workspaceOf(git)
	real.Forge = forge

	report, err := Fix(context.Background(), rehearsedWorkspace(t, real), FixRequest{Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, report.Rehearsal, "checkpoint", "set_git_config", "set_git_config", "unset_git_config", "write_file", "untrack", "log")
	if len(calls) != 0 || len(log.Operations) != 0 {
		t.Errorf("something changed: %q", calls)
	}
}

func TestDryRunOfSync(t *testing.T) {
	forge := &testkit.FakeForge{}
	real, _, log := workspaceOf(testkit.HealthyRepository("main"))
	real.Forge, real.Config = forge, &testkit.FakeConfigFiles{}
	ws := rehearsedWorkspace(t, real)
	plan, err := PlanSync(context.Background(), ws)
	if err != nil || plan.Empty() {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	result, err := ApplySync(context.Background(), ws, plan)
	if err != nil {
		t.Fatal(err)
	}
	// One step per label, then the log entry.
	if !result.DryRun || len(result.Planned) != len(plan.Labels)+1 || result.Applied != len(plan.Labels) {
		t.Errorf("result = %+v for %d changes", result, len(plan.Labels))
	}
	if len(forge.LabelCalls) != 0 || len(log.Operations) != 0 {
		t.Errorf("something changed: %q", forge.LabelCalls)
	}
}

func TestDryRunOfInit(t *testing.T) {
	files := &testkit.FakeConfigFiles{}
	real, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	real.Config = files
	result, err := Init(context.Background(), rehearsedWorkspace(t, real))
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, result.Rehearsal, "create_config_file")
	if result.Path != testkit.FakeRepositoryConfigPath || files.Repository != "" {
		t.Errorf("result = %+v, file %q", result, files.Repository)
	}

	files.Repository = "{}"
	if _, err := Init(context.Background(), rehearsedWorkspace(t, real)); !errors.Is(err, &fault.Fault{Code: fault.ConfigAlreadyThere}) {
		t.Fatalf("existing file: %v", err)
	}
}

func TestDryRunOfConfigEdits(t *testing.T) {
	files := &testkit.FakeConfigFiles{Repository: "{\n  \"ship\": {\"pullRequest\": {\"maxLines\": 300}}\n}\n"}
	real, _, log := workspaceOf(testkit.HealthyRepository("main"))
	real.Config = files
	ws := rehearsedWorkspace(t, real)

	set, err := SetSetting(context.Background(), ws, ConfigEditRequest{Key: "ship.pullRequest.maxLines", Value: "400"})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, set.Rehearsal, "write_config_file", "log")
	unset, err := UnsetSetting(context.Background(), ws, ConfigEditRequest{Key: "ship.pullRequest.maxLines"})
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, unset.Rehearsal, "write_config_file", "log")
	if !set.Changed || set.Operation != "" || files.Writes != 0 || len(log.Operations) != 0 {
		t.Errorf("set = %+v, writes %d", set, files.Writes)
	}
	// A refusal stays a refusal.
	if _, err := SetSetting(context.Background(), ws, ConfigEditRequest{Key: "ship.pullRequest.maxLines", Value: "many"}); err == nil {
		t.Error("an invalid value must be refused on a dry run too")
	}
}

func TestDryRunOfLabel(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.Generated = map[string]bool{"api/generated.pb.go": true}
	forge := labelForge("help wanted")
	real, _ := publishWorkspace(git, forge)
	result, err := LabelPullRequest(context.Background(), rehearsedWorkspace(t, real), 12)
	if err != nil {
		t.Fatal(err)
	}
	wantPlan(t, result.Rehearsal, "add_labels")
	if len(result.Added) != 3 || len(forge.Added) != 0 {
		t.Errorf("result = %+v, added %v", result, forge.Added)
	}
}

func TestARealRunSaysNothingOfDryRuns(t *testing.T) {
	ws, _, _ := workspaceOf(deliverableGit("main", nil))
	if ws.IsDryRun() || !DryRun(ws).IsDryRun() {
		t.Fatal("IsDryRun")
	}
	if again := DryRun(DryRun(ws)); again.rehearsal == nil {
		t.Fatal("DryRun of a dry run must keep its rehearsal")
	}
	result, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "dry_run") || strings.Contains(string(data), "planned") {
		t.Errorf("a real result mentions the dry run: %s", data)
	}
}

func TestDryRunOfARoadmapUndo(t *testing.T) {
	ws, tracker, _, git := roadmapWorkspace(t)
	ctx := context.Background()
	plan, err := PlanRoadmap(ctx, ws, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRoadmap(ctx, ws, plan); err != nil {
		t.Fatal(err)
	}
	synced := git.WorkingFiles[RoadmapFile]

	result, err := Undo(ctx, rehearsedWorkspace(t, ws), UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	keys := plannedKeys(result.Rehearsal)
	for _, want := range []string{"needs_consent_roadmap", "close_issue", "remove_parent", "delete_milestone"} {
		if !slices.Contains(keys, want) {
			t.Errorf("planned %q, missing %s", keys, want)
		}
	}
	for _, is := range tracker.IssueList {
		if is.Number != 3 && !is.Open {
			t.Errorf("issue #%d closed by a dry run", is.Number)
		}
	}
	if len(tracker.MilestoneList) != 1 || git.WorkingFiles[RoadmapFile] != synced {
		t.Error("a dry run changed the milestones or the file")
	}
}
