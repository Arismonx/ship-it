// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const nodePackage = `{"packageManager": "pnpm@9.12.0", "scripts": {"lint": "eslint .", "test": "vitest"}}`

func nodeProject(calls *[]string) testkit.FakeGit {
	git := publishableGit(calls)
	git.WorkingFiles = map[string]string{"package.json": nodePackage, "pnpm-lock.yaml": "lockfileVersion: 9"}
	git.RemoteBranches = map[string]bool{"origin/main": true}
	return git
}

type recordedGates struct{ events []string }

func (r *recordedGates) GateStarted(name, command string) { r.events = append(r.events, "start "+name) }
func (r *recordedGates) GateFinished(g GateResult) {
	r.events = append(r.events, string(g.Status)+" "+g.Name)
}

func TestPublishRunsTheGatesBeforePushing(t *testing.T) {
	var calls []string
	runner := &testkit.FakeRunner{}
	ws, _ := publishWorkspace(nodeProject(&calls), &testkit.FakeForge{})
	ws.Runner = runner
	observer := &recordedGates{}

	result, err := Publish(context.Background(), ws, PublishRequest{GateObserver: observer})
	if err != nil {
		t.Fatal(err)
	}
	want := "pnpm run lint;pnpm run test --changed origin/main --passWithNoTests"
	if strings.Join(runner.Calls, ";") != want {
		t.Fatalf("gates = %v", runner.Calls)
	}
	if strings.Join(observer.events, ",") != "start lint,passed lint,start test,passed test" {
		t.Errorf("events = %v", observer.events)
	}
	if len(result.Gates) != 3 || result.Gates[0].Status != GateSkipped || result.Gates[0].Name != "typecheck" {
		t.Errorf("results = %+v", result.Gates)
	}
	if !testkit.Pushed(calls) {
		t.Errorf("push expected after green gates: %v", calls)
	}
}

func TestRedGateStopsBeforeThePush(t *testing.T) {
	var calls []string
	runner := &testkit.FakeRunner{Failing: map[string]string{"pnpm run lint": "src/a.ts:3 no-unused-vars"}}
	forge := &testkit.FakeForge{}
	ws, log := publishWorkspace(nodeProject(&calls), forge)
	ws.Runner = runner

	_, err := Publish(context.Background(), ws, PublishRequest{})
	results, red := GatesIn(err)
	if !red || !errors.Is(err, &fault.Fault{Code: fault.GateFailed}) || !IsGateRefusal(err) {
		t.Fatalf("error = %v", err)
	}
	if last := results[len(results)-1]; last.Name != "lint" || !strings.Contains(last.Output, "no-unused-vars") {
		t.Errorf("results = %+v", results)
	}
	if len(runner.Calls) != 1 || testkit.Pushed(calls) || len(forge.Created) != 0 || len(log.Operations) != 0 {
		t.Fatalf("nothing after a red gate: runs %v, git %v", runner.Calls, calls)
	}
}

func TestMissingToolIsReported(t *testing.T) {
	ws, _ := publishWorkspace(nodeProject(nil), &testkit.FakeForge{})
	ws.Runner = &testkit.FakeRunner{Missing: map[string]bool{"pnpm": true}}
	_, err := Publish(context.Background(), ws, PublishRequest{})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != fault.GateToolMissing || f.Params["tool"] != "pnpm" {
		t.Fatalf("error = %v", err)
	}
}

func TestSkippingGatesIsWrittenInThePullRequest(t *testing.T) {
	runner := &testkit.FakeRunner{Failing: map[string]string{"pnpm run lint": "red"}}
	forge := &testkit.FakeForge{}
	ws, _ := publishWorkspace(nodeProject(nil), forge)
	ws.Runner = runner
	if _, err := Publish(context.Background(), ws, PublishRequest{SkipGates: true}); err != nil {
		t.Fatal(err)
	}
	if len(runner.Calls) != 0 || !strings.Contains(forge.Created[0].Body, "--skip-gates") {
		t.Fatalf("runs %v, body:\n%s", runner.Calls, forge.Created[0].Body)
	}
}

func TestGatesCanBeTurnedOff(t *testing.T) {
	runner := &testkit.FakeRunner{}
	ws, _ := publishWorkspace(nodeProject(nil), &testkit.FakeForge{})
	ws.Runner = runner
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"gates": {"enabled": false}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); err != nil || len(runner.Calls) != 0 {
		t.Fatalf("gates off: %v, runs %v", err, runner.Calls)
	}
}

// gatesFirst is a node project on main whose settings run the gates before
// the commit.
func gatesFirst(calls *[]string, runner *testkit.FakeRunner) (Workspace, *testkit.FakeCheckpointStore, *testkit.FakeOperationLog) {
	git := deliverableGit("main", calls)
	git.WorkingFiles = map[string]string{"package.json": nodePackage, "pnpm-lock.yaml": "lockfileVersion: 9"}
	ws, store, log := workspaceOf(git)
	ws.Runner = runner
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"gates": {"beforeCommit": true}}}`}
	return ws, store, log
}

func TestRedGateBeforeCommitRefusesTheCommit(t *testing.T) {
	var calls []string
	runner := &testkit.FakeRunner{Failing: map[string]string{"pnpm run lint": "src/a.ts:3 no-unused-vars"}}
	ws, store, log := gatesFirst(&calls, runner)
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if _, red := GatesIn(err); !red || !IsGateRefusal(err) {
		t.Fatalf("error = %v", err)
	}
	if len(calls) != 0 || len(store.Checkpoints) != 0 || len(log.Operations) != 0 {
		t.Fatalf("nothing may change after a red gate: git %q, checkpoints %d, log %d", calls, len(store.Checkpoints), len(log.Operations))
	}
}

func TestGreenGatesBeforeCommitAreNotRunTwice(t *testing.T) {
	var calls []string
	runner := &testkit.FakeRunner{}
	ws, _, _ := gatesFirst(&calls, runner)
	result, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if err != nil || len(result.Gates) == 0 || result.GatesDeferred || result.Commit == "" {
		t.Fatalf("result %+v, err %v", result, err)
	}
	// Off by default: the gates wait for the publication.
	ws, _, _ = gatesFirst(nil, runner)
	ws.Config = nil
	runner.Calls = nil
	result, err = DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"})
	if err != nil || len(runner.Calls) != 0 || !result.GatesDeferred {
		t.Fatalf("default: gates %v, result %+v, err %v", runner.Calls, result, err)
	}
}

func TestCheckGatesRunsWithoutCommitting(t *testing.T) {
	var calls []string
	runner := &testkit.FakeRunner{Failing: map[string]string{"pnpm run test": "1 failed"}}
	ws, _ := publishWorkspace(nodeProject(&calls), &testkit.FakeForge{})
	ws.Runner = runner
	result, err := CheckGates(context.Background(), ws, nil)
	if !IsGateRefusal(err) || len(result.Gates) != 3 || len(runner.Calls) != 2 {
		t.Fatalf("result %+v, err %v, runs %v", result, err, runner.Calls)
	}
	if testkit.Pushed(calls) {
		t.Errorf("ship check gates pushed: %q", calls)
	}
	for _, call := range calls {
		if strings.HasPrefix(call, "commit") || strings.HasPrefix(call, "stage") {
			t.Errorf("ship check gates changed the repository: %q", calls)
		}
	}
}
