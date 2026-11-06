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

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func publishableGit(calls *[]string) testkit.FakeGit {
	git := testkit.HealthyRepository("feat/42-add-vat")
	git.CommitMessages = []string{"✨ feat: add vat"}
	git.DiffStats = []ports.FileStat{
		{Path: "billing/vat.go", Added: 20, Deleted: 5},
		{Path: "go.sum", Added: 400},              // lockfile: not counted
		{Path: "api/generated.pb.go", Added: 900}, // linguist-generated: not counted
	}
	git.Generated = map[string]bool{"api/generated.pb.go": true}
	git.Calls = calls
	return git
}

func TestPublishWritesTheBodyFromFacts(t *testing.T) {
	git := publishableGit(nil)
	git.CommitMessages = []string{"✨ feat: add vat\n\nThe EU changed its VAT rules in July.\n\nSigned-off-by: Ada <ada@example.invalid>"}
	forge := &testkit.FakeForge{IssueTitles: map[int]string{42: "Invoices ignore VAT"}}
	ws, _ := publishWorkspace(git, forge)
	ws.Runner = &testkit.FakeRunner{}
	ws.Config = &testkit.FakeConfigFiles{}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	want := "Closes #42 (Invoices ignore VAT)\n\nThe EU changed its VAT rules in July.\n\n" + pullrequest.Marker + "\n"
	if body := forge.Created[0].Body; body != want {
		t.Fatalf("body =\n%s\nwant\n%s", body, want)
	}
}

func TestPublishFillsTheRepositoryTemplate(t *testing.T) {
	for name, setting := range map[string]string{
		"found as GitHub does": "",
		"named by the config":  `{"ship": {"pullRequest": {"template": ".github/PULL_REQUEST_TEMPLATE/feature.md"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			git := publishableGit(nil)
			git.CommitMessages = []string{"✨ feat: add vat\n\nThe EU changed its VAT rules."}
			git.Files = map[string]string{
				".github/PULL_REQUEST_TEMPLATE.md":         "## Why\n\n<!-- the motivation -->\n\n## Checklist\n\n- [ ] Docs updated\n",
				".github/PULL_REQUEST_TEMPLATE/feature.md": "## Why\n\n<!-- the motivation -->\n\n## Checklist\n\n- [ ] Docs updated\n",
				"docs/pull_request_template.md":            "## Ignored: .github comes first\n",
				"README.md":                                "# shop\n",
			}
			forge := &testkit.FakeForge{}
			ws, _ := publishWorkspace(git, forge)
			ws.Config = &testkit.FakeConfigFiles{Repository: setting}
			if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
				t.Fatal(err)
			}
			want := "Closes #42\n\n## Why\n\n<!-- the motivation -->\n\nThe EU changed its VAT rules.\n\n## Checklist\n\n- [ ] Docs updated\n\n" + pullrequest.Marker + "\n"
			if body := forge.Created[0].Body; body != want {
				t.Fatalf("body =\n%s\nwant\n%s", body, want)
			}
		})
	}
}

func publishWorkspace(git testkit.FakeGit, forge *testkit.FakeForge) (Workspace, *testkit.FakeOperationLog) {
	ws, _, log := workspaceOf(git)
	ws.Forge = forge
	return ws, log
}

func TestPublishOpensAPullRequest(t *testing.T) {
	var calls []string
	forge := &testkit.FakeForge{}
	ws, log := publishWorkspace(publishableGit(&calls), forge)

	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"fetch origin", "push origin feat/42-add-vat"}) {
		t.Fatalf("calls = %q", calls)
	}
	spec := forge.Created[0]
	if spec.Title != "✨ feat: add vat" || spec.Head != "feat/42-add-vat" || spec.Base != "main" || spec.Draft {
		t.Errorf("spec = %+v", spec)
	}
	if spec.Body != "Closes #42\n\n"+pullrequest.Marker+"\n" {
		t.Errorf("body must link the issue from the branch name, and say nothing it does not know:\n%s", spec.Body)
	}
	if !result.Created || result.Repository.Slug() != "acme/shop" || result.PullRequest.Number != 1 {
		t.Errorf("result = %+v", result)
	}
	if op := log.Operations[0]; op.Kind != oplog.Publish || op.PullRequest != 1 || op.Remote != "origin" {
		t.Errorf("operation = %+v", op)
	}
}

func TestPublishUpdatesTheSizeOfAnOpenPullRequest(t *testing.T) {
	open := ports.PullRequest{Number: 9, Labels: []string{"size/XS", "kind/bug", "priority/p1"}}
	forge := &testkit.FakeForge{
		Open:             map[string]ports.PullRequest{"feat/42-add-vat": open},
		RepositoryLabels: testkit.LabelNames("kind/feature", "kind/bug", "size/XS", "size/S"),
	}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// 25 changed lines: size/S replaces size/XS; kind/feature is not added to
	// an existing pull request, priority/p1 and kind/bug are left alone.
	if strings.Join(forge.LabelCalls, ";") != "remove size/XS from #9" || !reflect.DeepEqual(forge.Added[9], []string{"size/S"}) {
		t.Fatalf("calls %v, added %v", forge.LabelCalls, forge.Added)
	}
	if !reflect.DeepEqual(result.Labels, []string{"size/S"}) {
		t.Errorf("result labels = %v", result.Labels)
	}
}

func TestPublishLabelsWithTheRepositoryOwnLabels(t *testing.T) {
	forge := &testkit.FakeForge{RepositoryLabels: testkit.LabelNames("kind/feature", "size/S", "size/XXL", "enhancement")}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// 25 lines once go.sum and the generated file are left out: size/S.
	want := []string{"kind/feature", "size/S"}
	if !reflect.DeepEqual(forge.Added[1], want) || !reflect.DeepEqual(result.Labels, want) {
		t.Fatalf("labels = %v, result %v", forge.Added[1], result.Labels)
	}
}

func TestPublishMentionsAnAgentWrittenPlan(t *testing.T) {
	forge := &testkit.FakeForge{}
	ws, log := publishWorkspace(publishableGit(nil), forge)
	log.Operations = []oplog.Operation{{ID: "op1", Kind: oplog.Deliver, AIAssistant: "host agent",
		After: oplog.RepositoryView{Branch: "feat/42-add-vat"}}}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(forge.Created[0].Body, "\nThe commit message was drafted with AI assistance (host agent).\n") {
		t.Fatalf("body:\n%s", forge.Created[0].Body)
	}
}

func TestPublishDisclosesAIWithTheRepositoryLabel(t *testing.T) {
	forge := &testkit.FakeForge{RepositoryLabels: testkit.LabelNames("kind/feature", "AI-Assisted")}
	ws, log := publishWorkspace(publishableGit(nil), forge)
	log.Operations = []oplog.Operation{{ID: "op1", Kind: oplog.Deliver, AIAssistant: "host agent",
		After: oplog.RepositoryView{Branch: "feat/42-add-vat"}}}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(forge.Created[0].Body, "AI assistance") {
		t.Errorf("the label discloses it; no sentence:\n%s", forge.Created[0].Body)
	}
	if !slices.Contains(forge.Added[1], "AI-Assisted") || !slices.Contains(result.Labels, "AI-Assisted") {
		t.Fatalf("labels = %v, result %v", forge.Added, result.Labels)
	}
}

func TestPublishUnattendedOpensADraft(t *testing.T) {
	forge := &testkit.FakeForge{}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	if _, err := Publish(context.Background(), ws, PublishRequest{Unattended: true}); err != nil {
		t.Fatal(err)
	}
	if !forge.Created[0].Draft {
		t.Fatal("an unattended publication must open a draft (ADR-0010)")
	}
}

func TestPublishUpdatesTheOpenPullRequest(t *testing.T) {
	var calls []string
	open := ports.PullRequest{Number: 9, URL: "https://github.com/acme/shop/pull/9"}
	forge := &testkit.FakeForge{Open: map[string]ports.PullRequest{"feat/42-add-vat": open}}
	ws, _ := publishWorkspace(publishableGit(&calls), forge)
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created || !reflect.DeepEqual(result.PullRequest, open) || len(forge.Created) != 0 || !testkit.Pushed(calls) {
		t.Fatalf("result = %+v, created %d, calls %q", result, len(forge.Created), calls)
	}
}

func TestPublishRebasesABranchBehindTheBase(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.RemoteBranches = map[string]bool{"origin/main": true}
	git.Comparisons = map[string][2]int{"origin/main": {1, 3}}
	git.WorkingTree.Branch.Commit = "c1"
	git.Rebased = "c1r"
	git.Refs = map[string]string{"refs/remotes/origin/feat/42-add-vat": "c1"} // pushed before
	ws, log := publishWorkspace(git, &testkit.FakeForge{})

	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || !result.Rebased {
		t.Fatalf("result = %+v, %v", result, err)
	}
	want := []string{"fetch origin", "rebase origin/main", "push origin feat/42-add-vat lease c1"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q", calls)
	}
	if log.Operations[1].Kind != oplog.Rebase || log.Operations[1].After.Head != "c1r" || log.Operations[0].Commit != "c1r" {
		t.Errorf("operations = %+v", log.Operations)
	}
}

func TestPublishWithoutRebase(t *testing.T) {
	tests := []struct {
		name     string
		change   func(*testkit.FakeGit)
		warning  string
		wantPush string
	}{
		{"conflict: published as it is", func(g *testkit.FakeGit) { g.RebaseConflicts = true }, "SHIP-W0610", "push origin feat/42-add-vat"},
		{"someone else pushed: their commits first", func(g *testkit.FakeGit) {
			g.Refs = map[string]string{"refs/remotes/origin/feat/42-add-vat": "theirs"}
		}, "SHIP-W0611", "push origin feat/42-add-vat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			git := publishableGit(&calls)
			git.RemoteBranches = map[string]bool{"origin/main": true}
			git.Comparisons = map[string][2]int{"origin/main": {1, 3}}
			git.WorkingTree.Branch.Commit = "c1"
			tt.change(&git)
			ws, _ := publishWorkspace(git, &testkit.FakeForge{})
			result, err := Publish(context.Background(), ws, PublishRequest{})
			if err != nil || result.Rebased || len(result.Warnings) != 1 || result.Warnings[0].Code != tt.warning {
				t.Fatalf("result = %+v, %v", result, err)
			}
			if calls[len(calls)-1] != tt.wantPush {
				t.Fatalf("calls = %q", calls)
			}
		})
	}
}

func TestPublishRefusals(t *testing.T) {
	trunk := publishableGit(nil)
	trunk.WorkingTree.Branch.Name = "main"
	gitlab := publishableGit(nil)
	gitlab.RemoteURLs = map[string]string{"origin": "/srv/git/shop.git"}
	noRemote := publishableGit(nil)
	noRemote.RemoteURLs = nil
	nothing := publishableGit(nil)
	nothing.CommitMessages = nil
	cases := []struct {
		name  string
		git   testkit.FakeGit
		forge *testkit.FakeForge
		code  fault.Code
	}{
		{"trunk", trunk, &testkit.FakeForge{}, fault.PublishFromTrunk},
		{"local remote", gitlab, &testkit.FakeForge{}, fault.NoGitHubRemote},
		{"no origin", noRemote, &testkit.FakeForge{}, fault.NoGitHubRemote},
		{"no token", publishableGit(nil), &testkit.FakeForge{Err: ports.ErrNotAuthenticated}, fault.NotAuthenticated},
		{"bad token", publishableGit(nil), &testkit.FakeForge{Err: ports.ErrTokenRejected}, fault.TokenRejected},
		{"no access", publishableGit(nil), &testkit.FakeForge{Err: ports.ErrNotFound}, fault.GitHubRepositoryNotFound},
		{"nothing ahead", nothing, &testkit.FakeForge{}, fault.NothingToPublish},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			tt.git.Calls = &calls
			ws, log := publishWorkspace(tt.git, tt.forge)
			_, err := Publish(context.Background(), ws, PublishRequest{})
			if !errors.Is(err, &fault.Fault{Code: tt.code}) {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			if testkit.Pushed(calls) || len(log.Operations) != 0 {
				t.Fatalf("a refused publication must not push: calls %q", calls)
			}
		})
	}
}

func TestTheTestedTrailerReachesThePullRequest(t *testing.T) {
	git := publishableGit(nil)
	git.CommitMessages = []string{"✨ feat: add vat\n\nThe EU changed its VAT rules.\n\nTested: three invoices by hand\nSigned-off-by: Ada <ada@example.invalid>"}
	forge := &testkit.FakeForge{}
	ws, _ := publishWorkspace(git, forge)
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	body := forge.Created[0].Body
	if !strings.Contains(body, "The EU changed its VAT rules.") || !strings.Contains(body, "### Testing\n\nTested by the author: three invoices by hand.") || strings.Contains(body, "Signed-off-by") {
		t.Fatalf("body:\n%s", body)
	}
}

func TestPublishScansTheCommitsItWouldPush(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	// Assembled at run time: ship-it's own scan must not flag this file.
	token := strings.Join([]string{"gh", "p_", strings.Repeat("a1B2", 9)}, "")
	git.PushedDiff = "diff --git a/deploy.sh b/deploy.sh\nnew file mode 100644\n--- /dev/null\n+++ b/deploy.sh\n@@ -0,0 +1 @@\n+export GH_TOKEN=" + token + "\n"
	ws, _ := publishWorkspace(git, &testkit.FakeForge{})
	_, err := Publish(context.Background(), ws, PublishRequest{})
	findings, ok := SecretsIn(err)
	if !ok || len(findings) != 1 || findings[0].Path != "deploy.sh" || testkit.Pushed(calls) {
		t.Fatalf("err = %v, findings %+v, calls %q", err, findings, calls)
	}
	var f *fault.Fault
	if !errors.As(err, &f) || f.Key != "secrets.unpushed" || !IsGateRefusal(err) {
		t.Errorf("fault = %+v", f)
	}
}

func TestPublishRefusesTheRealDefaultBranch(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.WorkingTree.Branch.Name = "production"
	ws, _ := publishWorkspace(git, &testkit.FakeForge{DefaultBranch: "production"})
	_, err := Publish(context.Background(), ws, PublishRequest{})
	if !errors.Is(err, &fault.Fault{Code: fault.PublishFromTrunk}) || testkit.Pushed(calls) {
		t.Fatalf("err = %v, calls %q", err, calls)
	}
}

func TestPublishKeepsTheLeaseOfAnEarlierRebase(t *testing.T) {
	newGit := func(calls *[]string) testkit.FakeGit {
		git := publishableGit(calls)
		git.WorkingTree.Branch.Commit = "rebased"
		git.RemoteBranches = map[string]bool{"origin/main": true, "origin/feat/42-add-vat": true}
		git.Refs = map[string]string{"refs/remotes/origin/feat/42-add-vat": "old"}
		return git
	}
	// The earlier publish rebased from "old" (the pushed tip), then stopped.
	var calls []string
	ws, log := publishWorkspace(newGit(&calls), &testkit.FakeForge{})
	log.Operations = []oplog.Operation{{ID: "op1", Kind: oplog.Rebase,
		Before: oplog.RepositoryView{Branch: "feat/42-add-vat", Head: "old"}, After: oplog.RepositoryView{Branch: "feat/42-add-vat", Head: "rebased"}}}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "push origin feat/42-add-vat lease old") {
		t.Fatalf("calls = %q", calls)
	}
	// Without ship-it's rebase in the log, the remote branch may hold
	// someone else's commits: no lease, nothing overwritten.
	calls = nil
	ws, _ = publishWorkspace(newGit(&calls), &testkit.FakeForge{})
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "push origin feat/42-add-vat") {
		t.Fatalf("calls = %q", calls)
	}
}

func TestPublishRebasesAgainAfterItsOwnUnpushedRebase(t *testing.T) {
	var calls []string
	git := publishableGit(&calls)
	git.WorkingTree.Branch.Commit = "rebased"
	git.RemoteBranches = map[string]bool{"origin/main": true, "origin/feat/42-add-vat": true}
	git.Refs = map[string]string{"refs/remotes/origin/feat/42-add-vat": "old"}
	git.Comparisons = map[string][2]int{"origin/main": {1, 2}} // main moved again
	git.Rebased = "rebased-again"
	ws, log := publishWorkspace(git, &testkit.FakeForge{})
	log.Operations = []oplog.Operation{{ID: "op1", Kind: oplog.Rebase,
		Before: oplog.RepositoryView{Branch: "feat/42-add-vat", Head: "old"}, After: oplog.RepositoryView{Branch: "feat/42-add-vat", Head: "rebased"}}}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "rebase origin/main") || !slices.Contains(calls, "push origin feat/42-add-vat lease old") || len(result.Warnings) != 0 {
		t.Fatalf("calls = %q, warnings %+v", calls, result.Warnings)
	}
}
