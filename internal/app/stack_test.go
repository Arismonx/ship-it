// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// stackWork is new work in three areas, on main.
func stackWork(calls *[]string) testkit.FakeGit {
	git := testkit.HealthyRepository("main")
	git.WorkingFiles = map[string]string{
		"api/cart.go": "package api\n\nfunc Cart() {}\n", "web/cart.ts": "export {}\nlet x = 1\n", "docs/cart.md": "# Cart\n",
	}
	for _, p := range []string{"api/cart.go", "web/cart.ts", "docs/cart.md"} {
		git.WorkingTree.Files = append(git.WorkingTree.Files, ports.FileChange{Path: p, State: ports.Untracked})
	}
	git.RemoteBranches = map[string]bool{"origin/main": true}
	git.Calls = calls
	return git
}

func stackRecordOf(branches ...string) oplog.Operation {
	rec := &oplog.StackRecord{Base: "main"}
	for _, b := range branches {
		rec.Layers = append(rec.Layers, oplog.StackLayer{Branch: b, Commit: b + "-1", Title: "✨ feat: add " + strings.TrimPrefix(b, "feat/")})
	}
	top := branches[len(branches)-1]
	return oplog.Operation{ID: "s1", Kind: oplog.Stack, Checkpoint: "cp", Before: oplog.RepositoryView{Branch: "main", Head: "abc"},
		After: oplog.RepositoryView{Branch: top, Head: top + "-1"}, CreatedBranch: true, Stack: rec}
}

// onLayer is a repository on a branch of a published stack.
func onLayer(branch string, calls *[]string, branches ...string) testkit.FakeGit {
	git := testkit.HealthyRepository(branch)
	git.WorkingTree.Branch.Commit = branch + "-1"
	git.ExistingBranches = map[string]bool{}
	git.Refs = map[string]string{"refs/remotes/origin/main": "m1"}
	git.RemoteBranches = map[string]bool{"origin/main": true}
	for _, b := range branches {
		git.ExistingBranches[b] = true
		git.Refs["refs/heads/"+b] = b + "-1"
	}
	git.Range = []ports.CommitRecord{{Message: "✨ feat: add a layer"}}
	git.Calls = calls
	return git
}

func TestProposeAndDeliverAStack(t *testing.T) {
	var calls []string
	ws, store, log := workspaceOf(stackWork(&calls))
	view, err := ProposeStack(context.Background(), ws, StackPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Layers) != 3 || view.Base != "main" || view.MaxLayers != 5 {
		t.Fatalf("plan = %+v", view)
	}
	if view.Layers[0].Area != "api/" || !reflect.DeepEqual(view.Layers[0].Files, []string{"api/cart.go"}) {
		t.Errorf("bottom layer = %+v: the biggest area", view.Layers[0])
	}
	var req StackDeliveryRequest
	for _, l := range view.Layers {
		req.Layers = append(req.Layers, StackLayerSpec{Title: l.Title, Branch: l.Branch, Files: l.Files})
	}
	result, err := DeliverStack(context.Background(), ws, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Layers) != 3 || result.Operation == "" {
		t.Fatalf("result = %+v", result)
	}
	var switches []string
	for _, c := range calls {
		if b, ok := strings.CutPrefix(c, "switch "); ok {
			switches = append(switches, b)
		}
	}
	if !reflect.DeepEqual(switches, []string{result.Layers[0].Branch, result.Layers[1].Branch, result.Layers[2].Branch}) ||
		!strings.HasPrefix(calls[1], "commit-only api/cart.go ") {
		t.Errorf("calls = %q", calls)
	}
	op := log.Operations[0]
	if op.Kind != oplog.Stack || len(op.Stack.Layers) != 3 || op.Before.Branch != "main" || op.After.Branch != result.Layers[2].Branch {
		t.Errorf("logged = %+v", op)
	}

	// Undone like a delivery: the work goes back uncommitted on main.
	calls = nil
	if _, err := Undo(context.Background(), ws, UndoRequest{}); err != nil {
		t.Fatal(err)
	}
	if calls[0] != "attach main" || calls[1] != "reset-index abc" || len(calls) != 5 ||
		calls[2] != "delete "+result.Layers[2].Branch+" at "+testkit.FakeCommitID {
		t.Errorf("undo calls = %q", calls)
	}
	if store.Restored != result.Checkpoint.ID {
		t.Errorf("restored %q", store.Restored)
	}
}

func TestProposeStackRefusesOneArea(t *testing.T) {
	git := stackWork(nil)
	git.WorkingTree.Files = git.WorkingTree.Files[:1]
	ws, _, _ := workspaceOf(git)
	if _, err := ProposeStack(context.Background(), ws, StackPlanRequest{}); !errors.Is(err, &fault.Fault{Code: fault.StackNeedsLayers}) {
		t.Errorf("err = %v", err)
	}
	ws, _, _ = workspaceOf(onLayer("feat/a", nil, "feat/a"))
	if _, err := ProposeStack(context.Background(), ws, StackPlanRequest{}); !errors.Is(err, &fault.Fault{Code: fault.StackNotFromTrunk}) {
		t.Errorf("err = %v", err)
	}
}

func TestChosenLayers(t *testing.T) {
	ws, _, _ := workspaceOf(stackWork(nil))
	view, err := ProposeStack(context.Background(), ws, StackPlanRequest{Layers: [][]string{{"docs/"}, {"api/", "web/"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Layers) != 2 || view.Layers[0].Files[0] != "docs/cart.md" || len(view.Layers[1].Files) != 2 {
		t.Errorf("layers = %+v", view.Layers)
	}
}

func TestPublishANativeStackThenUndoIt(t *testing.T) {
	var calls []string
	forge := &testkit.FakeForge{}
	stacks := &testkit.FakeStacks{}
	ws, log := publishWorkspace(onLayer("feat/b", &calls, "feat/a", "feat/b"), forge)
	ws.Stacks = stacks
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}

	result, err := PublishStack(context.Background(), ws, StackPublishRequest{Unattended: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"fetch origin", "push origin feat/a", "push origin feat/b"}) {
		t.Errorf("calls = %q", calls)
	}
	if len(forge.Created) != 2 || forge.Created[0].Base != "main" || forge.Created[1].Base != "feat/a" || !forge.Created[1].Draft {
		t.Fatalf("pull requests = %+v", forge.Created)
	}
	if strings.Contains(forge.Created[1].Body, "Part ") {
		t.Error("GitHub shows a native stack: no chain line")
	}
	if !reflect.DeepEqual(stacks.Calls, []string{"create 1,2"}) || !result.Native || result.Number != 1 {
		t.Errorf("stack calls = %q, result %+v", stacks.Calls, result)
	}
	op := log.Operations[0]
	if op.Kind != oplog.StackPublish || op.Stack.Number != 1 || !op.Stack.Layers[1].Created || op.Stack.Layers[1].Commit != "feat/b-1" {
		t.Errorf("logged = %+v", op.Stack)
	}

	// Undo needs consent, then closes every pull request and dissolves the stack.
	if _, err := Undo(context.Background(), ws, UndoRequest{}); !NeedsConsent(err) {
		t.Fatalf("undo without consent = %v", err)
	}
	calls = nil
	undone, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forge.Closed, []int{2, 1}) || stacks.Calls[1] != "unstack 1" || undone.Unstacked != 1 {
		t.Errorf("closed %v, stack calls %q", forge.Closed, stacks.Calls)
	}
	if !reflect.DeepEqual(calls, []string{"delete-remote origin feat/b at feat/b-1", "delete-remote origin feat/a at feat/a-1"}) {
		t.Errorf("calls = %q", calls)
	}
}

func TestUndoKeepsTheMergedLayers(t *testing.T) {
	forge := &testkit.FakeForge{Latest: map[string]ports.PullRequest{"feat/a": {Number: 1, Merged: true, State: "closed"}}}
	ws, log := publishWorkspace(onLayer("feat/b", nil, "feat/a", "feat/b"), forge)
	ws.Stacks = &testkit.FakeStacks{}
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	if _, err := PublishStack(context.Background(), ws, StackPublishRequest{}); err != nil {
		t.Fatal(err)
	}
	forge.Latest["feat/a"] = ports.PullRequest{Number: 1, Merged: true, State: "closed"}
	if _, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forge.Closed, []int{2}) {
		t.Errorf("closed = %v: a merged layer stays", forge.Closed)
	}
}

func TestPublishAChainWithoutStacks(t *testing.T) {
	forge := &testkit.FakeForge{}
	ws, log := publishWorkspace(onLayer("feat/b", nil, "feat/a", "feat/b"), forge)
	ws.Stacks = &testkit.FakeStacks{Unavailable: true} // GHES
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	result, err := PublishStack(context.Background(), ws, StackPublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Native || !strings.HasSuffix(forge.Created[0].Body, "\nPart 1 of 2.\n") ||
		!strings.HasSuffix(forge.Created[1].Body, "\nPart 2 of 2, after #1.\n") || forge.Created[1].Base != "feat/a" {
		t.Errorf("bodies:\n%s\n---\n%s", forge.Created[0].Body, forge.Created[1].Body)
	}
	// No stacks port at all is a chain too.
	ws.Stacks = nil
	if _, err := PublishStack(context.Background(), ws, StackPublishRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestPublishStackRenamesItsLayersForARuleset(t *testing.T) {
	var calls []string
	feature := []rules.Rule{namePattern(rules.StartsWith, "feature/")}
	forge := &testkit.FakeForge{Rules: map[string][]rules.Rule{
		"feat/a": feature, "feat/b": feature, "feature/a": feature, "feature/b": feature,
	}}
	ws, log := publishWorkspace(onLayer("feat/b", &calls, "feat/a", "feat/b"), forge)
	ws.Stacks = &testkit.FakeStacks{}
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	result, err := PublishStack(context.Background(), ws, StackPublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fetch origin", "rename feat/a feature/a at feat/a-1", "rename feat/b feature/b at feat/b-1",
		"push origin feature/a", "push origin feature/b"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls =\n%q\nwant\n%q", calls, want)
	}
	if forge.Created[0].Head != "feature/a" || forge.Created[1].Base != "feature/a" || len(result.Warnings) != 2 {
		t.Fatalf("pull requests %+v, warnings %+v", forge.Created, result.Warnings)
	}
	// The stack follows the new names; undo gives the old ones back.
	if c, err := currentChain(context.Background(), ws, "feature/b"); err != nil || !reflect.DeepEqual(c.Branches, []string{"feature/a", "feature/b"}) {
		t.Fatalf("chain = %+v, %v", c, err)
	}
	calls = nil
	undone, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"delete-remote origin feature/b at feat/b-1", "delete-remote origin feature/a at feat/a-1",
		"rename feature/b feat/b at feat/b-1", "rename feature/a feat/a at feat/a-1"}
	if !reflect.DeepEqual(calls, want) || undone.Branch != "feat/b" {
		t.Fatalf("undo calls =\n%q\nwant\n%q (on %q)", calls, want, undone.Branch)
	}
}

func TestPublishStackPushesNothingWhenALayerIsRefused(t *testing.T) {
	var calls []string
	git := onLayer("feat/b", &calls, "feat/a", "feat/b")
	git.Range = []ports.CommitRecord{{ID: "c2", Message: "wip"}}
	message := rules.Rule{Type: rules.CommitMessagePattern, Pattern: &rules.Pattern{Name: "Conventional", Operator: rules.StartsWith, Pattern: "✨"}}
	forge := &testkit.FakeForge{Rules: map[string][]rules.Rule{"feat/b": {message}}}
	ws, log := publishWorkspace(git, forge)
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	_, err := PublishStack(context.Background(), ws, StackPublishRequest{})
	if !errors.Is(err, &fault.Fault{Code: fault.CommitMessageRefused}) {
		t.Fatalf("err = %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"fetch origin"}) || len(forge.Created) != 0 {
		t.Fatalf("nothing may be pushed: %q", calls)
	}
}

func TestLayersGetLabelsAndTheLinkedIssue(t *testing.T) {
	git := onLayer("feat/b", nil, "feat/a", "feat/b")
	git.LayerStats = map[string][]ports.FileStat{
		"refs/heads/feat/a": {{Path: "api/cart.go", Added: 300}}, "refs/heads/feat/b": {{Path: "web/cart.ts", Added: 5}},
	}
	forge := &testkit.FakeForge{RepositoryLabels: testkit.LabelNames("kind/feature", "size/XS", "size/L"),
		IssueTitles: map[int]string{12: "Add a cart"}}
	ws, log := publishWorkspace(git, forge)
	ws.Stacks = &testkit.FakeStacks{}
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	result, err := PublishStack(context.Background(), ws, StackPublishRequest{Issue: 12})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forge.Added[1], []string{"kind/feature", "size/L"}) || !reflect.DeepEqual(forge.Added[2], []string{"kind/feature", "size/XS"}) {
		t.Errorf("labels = %v", forge.Added)
	}
	if !reflect.DeepEqual(result.Layers[0].Labels, []string{"kind/feature", "size/L"}) {
		t.Errorf("layer labels = %+v", result.Layers[0])
	}
	// Only the bottom layer targets the default branch: it alone closes.
	if !strings.Contains(forge.Created[0].Body, "Closes #12") || strings.Contains(forge.Created[1].Body, "Closes #12") ||
		!strings.Contains(forge.Created[1].Body, "Refs #12") {
		t.Errorf("bodies:\n%s\n---\n%s", forge.Created[0].Body, forge.Created[1].Body)
	}
}

func TestAChainSaysWhereEachLayerSits(t *testing.T) {
	// A layer added on top of a chain of two: the two below say "of 3".
	var calls []string
	git := onLayer("feat/c", &calls, "feat/a", "feat/b", "feat/c")
	git.RemoteBranches["origin/feat/b"] = true
	git.CommitMessages = []string{"✨ feat: add c"}
	forge := &testkit.FakeForge{Open: map[string]ports.PullRequest{"feat/a": {Number: 10}, "feat/b": {Number: 11}}}
	stacks := &testkit.FakeStacks{Unavailable: true, PullRequests: map[int]ports.StackPullRequest{
		10: {Number: 10, Body: "## Summary\na\n\nPart 1 of 2.\n"}, 11: {Number: 11, Body: "## Summary\nb\n\nPart 2 of 2, after #10.\n"},
	}}
	ws, log := publishWorkspace(git, forge)
	ws.Stacks = stacks
	log.Operations = []oplog.Operation{
		{ID: "d1", Kind: oplog.Deliver, StackParent: "feat/b", CreatedBranch: true, After: oplog.RepositoryView{Branch: "feat/c"}},
		stackRecordOf("feat/a", "feat/b"),
	}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(forge.Created[0].Body, "\nPart 3 of 3, after #11.\n") {
		t.Errorf("new layer body:\n%s", forge.Created[0].Body)
	}
	if stacks.PullRequests[10].Body != "## Summary\na\n\nPart 1 of 3.\n" || stacks.PullRequests[11].Body != "## Summary\nb\n\nPart 2 of 3, after #10.\n" {
		t.Errorf("bodies below: %q, %q", stacks.PullRequests[10].Body, stacks.PullRequests[11].Body)
	}

	// The bottom layer merged: a sync says "Part 1 of 2." on the next one.
	ws2, log2, stacks2, _ := syncedStack(nil)
	stacks2.Unavailable = true
	stacks2.PullRequests = map[int]ports.StackPullRequest{
		2: {Number: 2, Body: "b\n\nPart 2 of 3, after #1.\n"}, 3: {Number: 3, Body: "c\n\nPart 3 of 3, after #2.\n"},
	}
	ws2.Forge.(*testkit.FakeForge).Open = map[string]ports.PullRequest{"feat/b": {Number: 2}, "feat/c": {Number: 3}}
	if _, err := SyncStack(context.Background(), ws2); err != nil {
		t.Fatal(err)
	}
	if stacks2.PullRequests[2].Body != "b\n\nPart 1 of 2.\n" || stacks2.PullRequests[3].Body != "c\n\nPart 2 of 2, after #2.\n" {
		t.Errorf("after the sync: %q, %q", stacks2.PullRequests[2].Body, stacks2.PullRequests[3].Body)
	}
	if notes := log2.Operations[0].Stack.Notes; len(notes) != 2 || notes[0].Before != "Part 2 of 3, after #1." || notes[0].After != "Part 1 of 2." {
		t.Errorf("notes = %+v", notes)
	}
}

func TestPublishStackFromTheTopOnly(t *testing.T) {
	ws, log := publishWorkspace(onLayer("feat/a", nil, "feat/a", "feat/b"), &testkit.FakeForge{})
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	if _, err := PublishStack(context.Background(), ws, StackPublishRequest{}); !errors.Is(err, &fault.Fault{Code: fault.StackNotOnTop}) {
		t.Errorf("err = %v", err)
	}
	ws, _ = publishWorkspace(onLayer("fix/other", nil, "fix/other"), &testkit.FakeForge{})
	if _, err := PublishStack(context.Background(), ws, StackPublishRequest{}); !errors.Is(err, &fault.Fault{Code: fault.NotInStack}) {
		t.Errorf("err = %v", err)
	}
}

func TestShipOnTheTopAddsALayer(t *testing.T) {
	var calls []string
	git := onLayer("feat/b", &calls, "feat/a", "feat/b")
	git.WorkingTree.Files = []ports.FileChange{{Path: "web/x.ts", State: ports.Tracked, Unstaged: ports.Modified}}
	git.DiffText = "diff --git a/web/x.ts b/web/x.ts\n--- a/web/x.ts\n+++ b/web/x.ts\n@@ -1 +1 @@\n-a\n+b\n"
	forge := &testkit.FakeForge{Open: map[string]ports.PullRequest{"feat/b": {Number: 11}}}
	stacks := &testkit.FakeStacks{Stacks: []ports.Stack{{Number: 7, PullRequests: []ports.StackPullRequest{{Number: 10}, {Number: 11}}}}}
	ws, log := publishWorkspace(git, forge)
	ws.Stacks = stacks
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}

	view, err := ProposePlan(context.Background(), ws, PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !view.CreatesBranch || !slices.ContainsFunc(view.Reasons, func(r ReasonView) bool { return r.Key == "plan.reason.stack_layer" }) {
		t.Errorf("plan = %+v", view)
	}
	delivery, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "✨ feat(web): add x", Branch: "feat/add-x"})
	if err != nil {
		t.Fatal(err)
	}
	if !delivery.CreatedBranch || calls[0] != "switch feat/add-x" || log.Operations[0].StackParent != "feat/b" {
		t.Fatalf("delivery = %+v, calls %q", delivery, calls)
	}

	git.ExistingBranches["feat/add-x"] = true
	git.RemoteBranches["origin/feat/b"] = true
	git.CommitMessages = []string{"✨ feat(web): add x"}
	ws.Git = git
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Base != "feat/b" || forge.Created[0].Base != "feat/b" || result.Stack != 7 || !reflect.DeepEqual(stacks.Calls, []string{"add 7 1"}) {
		t.Errorf("result = %+v, stacks %q", result, stacks.Calls)
	}
}

func TestANewLayerNeedsItsParentOnGitHub(t *testing.T) {
	git := onLayer("feat/c", nil, "feat/a", "feat/b", "feat/c")
	git.CommitMessages = []string{"✨ feat: c"}
	ws, log := publishWorkspace(git, &testkit.FakeForge{})
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b", "feat/c")}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); !errors.Is(err, &fault.Fault{Code: fault.StackParentUnpublished}) {
		t.Errorf("err = %v", err)
	}
}

// syncedStack is a stack of three layers whose bottom one was merged.
func syncedStack(calls *[]string) (Workspace, *testkit.FakeOperationLog, *testkit.FakeStacks, testkit.FakeGit) {
	git := onLayer("feat/b", calls, "feat/a", "feat/b", "feat/c")
	for _, b := range []string{"feat/a", "feat/b", "feat/c"} {
		git.Refs["refs/remotes/origin/"+b] = b + "-1"
	}
	git.Refs["refs/remotes/origin/main"] = "m2"
	git.Rebased = "r"
	forge := &testkit.FakeForge{Latest: map[string]ports.PullRequest{
		"feat/a": {Number: 1, Merged: true, State: "closed"}, "feat/b": {Number: 2, State: "open"}, "feat/c": {Number: 3, State: "open"},
	}}
	stacks := &testkit.FakeStacks{}
	ws, log := publishWorkspace(git, forge)
	ws.Stacks = stacks
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b", "feat/c")}
	return ws, log, stacks, git
}

func TestSyncRebasesLocallyAfterAMerge(t *testing.T) {
	var calls []string
	ws, log, stacks, _ := syncedStack(&calls)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"stack": {"rebase": "local"}}}`}
	result, err := SyncStack(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"fetch origin", "switch-to main",
		"rebase-onto feat/b origin/main feat/a-1", "push origin feat/b lease feat/b-1",
		"rebase-onto feat/c r-feat/b feat/b-1", "push origin feat/c lease feat/c-1",
		"delete feat/a at feat/a-1", "delete-remote origin feat/a at feat/a-1",
		"switch-to feat/b",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls =\n%q\nwant\n%q", calls, want)
	}
	if result.Mode != "local" || !reflect.DeepEqual(result.Deleted, []string{"feat/a"}) || len(result.Rebased) != 2 || len(stacks.Calls) != 0 {
		t.Errorf("result = %+v; a native stack is retargeted by GitHub: %q", result, stacks.Calls)
	}
	op := log.Operations[0]
	if op.Kind != oplog.StackSync || len(op.Stack.Layers) != 3 || op.Checkpoint == "" {
		t.Fatalf("logged = %+v", op)
	}

	// ship undo puts the local branches back and, with consent, GitHub.
	calls = nil
	if _, err := Undo(context.Background(), ws, UndoRequest{}); !NeedsConsent(err) {
		t.Fatalf("undo without consent = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("nothing changes without consent: %q", calls)
	}
	undone, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"move feat/b r-feat/b..feat/b-1", "move feat/c r-feat/c..feat/c-1", "recreate feat/a at feat/a-1",
		"attach feat/b", "reset-index feat/b-1",
		"push-back origin feat/b feat/b-1 lease r-feat/b", "push-back origin feat/c feat/c-1 lease r-feat/c",
		"push-back origin feat/a feat/a-1 lease "}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("undo calls =\n%q\nwant\n%q", calls, want)
	}
	if !reflect.DeepEqual(undone.RestoredRemote, []string{"feat/b", "feat/c", "feat/a"}) || len(undone.Warnings) != 0 {
		t.Errorf("undone = %+v", undone)
	}
}

func TestUndoOfASyncKeepsWhatOthersPushedAndRetargetsBack(t *testing.T) {
	var calls []string
	ws, log, stacks, git := syncedStack(&calls)
	stacks.Unavailable = true // a chain: the sync retargets #2 to main
	if _, err := SyncStack(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	layers := log.Operations[0].Stack.Layers
	if layers[0].PullRequest != 2 || layers[0].PreviousBase != "feat/a" || layers[0].Base != "main" {
		t.Fatalf("retarget not recorded: %+v", layers[0])
	}
	// Since the sync, someone pushed to feat/c; #2 still targets main.
	git.PushErr = map[string]error{"feat/c": errors.New("! [rejected] (stale info)")}
	ws.Git = git
	stacks.PullRequests = map[int]ports.StackPullRequest{2: {Number: 2, State: "open", BaseBranch: "main"}}
	stacks.Calls = nil
	undone, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(undone.RestoredRemote, []string{"feat/b", "feat/a"}) || !reflect.DeepEqual(undone.Retargeted, []int{2}) {
		t.Errorf("undone = %+v", undone)
	}
	if len(undone.Warnings) != 1 || undone.Warnings[0].Code != string(fault.StackChangedSince) || undone.Warnings[0].Params["branch"] != "feat/c" {
		t.Errorf("warnings = %+v", undone.Warnings)
	}
	if !reflect.DeepEqual(stacks.Calls, []string{"retarget 2 feat/a"}) {
		t.Errorf("stack calls = %q", stacks.Calls)
	}
	// Retargeted by someone since: kept.
	stacks.PullRequests[3] = ports.StackPullRequest{Number: 3, State: "open", BaseBranch: "main"}
	if err := undoStackSyncStep(context.Background(), ws, oplog.Step{Kind: oplog.RetargetPullRequest, Number: 3, Target: "feat/b", Expected: "feat/a"}); !errors.Is(err, &fault.Fault{Code: fault.StackChangedSince}) {
		t.Errorf("err = %v", err)
	}
}

func TestSyncFollowsGitHubsRebase(t *testing.T) {
	var calls []string
	ws, _, _, git := syncedStack(&calls)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"stack": {"rebase": "github"}}}`}
	// GitHub rebased feat/b; feat/c holds a commit never pushed.
	git.Fetched = map[string]string{"refs/remotes/origin/feat/b": "gh-b"}
	git.Refs["refs/heads/feat/c"] = "c-local"
	ws.Git = git
	result, err := SyncStack(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "github" || !reflect.DeepEqual(result.Followed, []string{"feat/b"}) || len(result.Rebased) != 0 {
		t.Errorf("result = %+v", result)
	}
	if !slices.Contains(calls, "move feat/b feat/b-1..gh-b") || len(result.Warnings) != 1 || result.Warnings[0].Code != string(fault.StackLayerUnpushed) {
		t.Errorf("calls %q, warnings %+v", calls, result.Warnings)
	}
}

func TestSyncRetargetsAChain(t *testing.T) {
	var calls []string
	ws, _, stacks, _ := syncedStack(&calls)
	stacks.Unavailable = true
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"stack": {"rebase": "github"}}}`}
	result, err := SyncStack(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "local" || !reflect.DeepEqual(stacks.Calls, []string{"retarget 2 main"}) {
		t.Errorf("a chain is rebased locally and retargeted: %+v, %q", result, stacks.Calls)
	}
}

func TestSyncNeedsAMerge(t *testing.T) {
	ws, _, _, git := syncedStack(nil)
	ws.Forge.(*testkit.FakeForge).Latest["feat/a"] = ports.PullRequest{Number: 1, State: "open"}
	if _, err := SyncStack(context.Background(), ws); !errors.Is(err, &fault.Fault{Code: fault.NothingToSync}) {
		t.Errorf("err = %v", err)
	}
	git.WorkingTree.Files = []ports.FileChange{{Path: "x", State: ports.Untracked}}
	ws.Git = git
	if _, err := SyncStack(context.Background(), ws); !errors.Is(err, &fault.Fault{Code: fault.StackNeedsCleanTree}) {
		t.Errorf("err = %v", err)
	}
}

func mergeableStack() (Workspace, *testkit.FakeStacks, *testkit.FakeOperationLog) {
	forge := &testkit.FakeForge{Open: map[string]ports.PullRequest{
		"feat/a": {Number: 1, HeadSHA: "a1", URL: "u1"}, "feat/b": {Number: 2, HeadSHA: "b1", URL: "u2"},
	}}
	stacks := &testkit.FakeStacks{Merges: []ports.MergeStatus{{Status: "pending", UUID: "u-1"}, {Status: "merged", SHA: "m"}}}
	ws, log := publishWorkspace(onLayer("feat/b", nil, "feat/a", "feat/b"), forge)
	ws.Stacks = stacks
	log.Operations = []oplog.Operation{stackRecordOf("feat/a", "feat/b")}
	return ws, stacks, log
}

func TestMergeNeedsAHuman(t *testing.T) {
	ws, stacks, _ := mergeableStack()
	for _, req := range []StackMergeRequest{{}, {Confirmed: true, Agent: true}} {
		_, err := MergeStack(context.Background(), ws, req)
		if !errors.Is(err, &fault.Fault{Code: fault.MergeNeedsConfirmation}) || !NeedsHuman(err) {
			t.Errorf("%+v: err = %v", req, err)
		}
	}
	if len(stacks.Calls) != 0 {
		t.Errorf("nothing may be asked: %q", stacks.Calls)
	}
}

func TestMergeTheStackAsynchronously(t *testing.T) {
	ws, stacks, log := mergeableStack()
	slept := 0
	result, err := MergeStack(context.Background(), ws, StackMergeRequest{Confirmed: true,
		Sleep: func(context.Context, time.Duration) error { slept++; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stacks.Calls, []string{"merge 2 at b1"}) || result.Status.Status != "merged" || slept != 2 {
		t.Errorf("calls %q, result %+v, slept %d", stacks.Calls, result, slept)
	}
	if len(result.Plan.Merges) != 2 || log.Operations[0].Kind != oplog.StackMerge {
		t.Errorf("plan %+v, log %+v", result.Plan, log.Operations[0])
	}
}

func TestMergeAChainMergesItsBottom(t *testing.T) {
	ws, stacks, _ := mergeableStack()
	stacks.Unavailable = true
	stacks.Merges = []ports.MergeStatus{{Status: "failed", Message: "Required status check is failing"}}
	_, err := MergeStack(context.Background(), ws, StackMergeRequest{Confirmed: true,
		Sleep: func(context.Context, time.Duration) error { return nil }})
	if !errors.Is(err, &fault.Fault{Code: fault.MergeFailed}) || stacks.Calls[0] != "merge 1 at a1" {
		t.Errorf("err %v, calls %q", err, stacks.Calls)
	}
}

func TestStackStatus(t *testing.T) {
	ws, _, _ := mergeableStack()
	view, err := StackStatus(context.Background(), ws, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Layers) != 2 || !view.Layers[1].Current || view.Layers[0].PullRequest.Number != 1 || view.Layers[1].Base != "feat/a" {
		t.Errorf("view = %+v", view)
	}
}

func TestADryRunOfAStackChangesNothing(t *testing.T) {
	git := stackWork(nil)
	forge := &testkit.FakeForge{}
	ws, _, _ := workspaceOf(git)
	ws.Forge, ws.Stacks = forge, &testkit.FakeStacks{}
	ws = rehearsedWorkspace(t, ws)
	view, err := ProposeStack(context.Background(), ws, StackPlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var req StackDeliveryRequest
	for _, l := range view.Layers {
		req.Layers = append(req.Layers, StackLayerSpec{Title: l.Title, Branch: l.Branch, Files: l.Files})
	}
	delivery, err := DeliverStack(context.Background(), ws, req)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := PublishStack(context.Background(), ws, StackPublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	keys := append(plannedKeys(delivery.Rehearsal), plannedKeys(publication.Rehearsal)...)
	opened := 0
	for _, k := range keys {
		if k == "open_pull_request" {
			opened++
		}
	}
	if opened != 3 || !slices.Contains(keys, "create_stack") || !slices.Contains(keys, "commit_only") {
		t.Errorf("planned = %q", keys)
	}
}
