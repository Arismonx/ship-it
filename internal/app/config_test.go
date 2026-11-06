// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestShowConfigNamesTheSources(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	ws.Config = &testkit.FakeConfigFiles{User: `{"ship":{"issues":{"keyword":"Fixes"}}}`, Repository: `{"ship":{"publish":{"mode":"never"}}}`}
	view, err := ShowConfig(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, s := range view.Settings {
		sources[s.Key] = s.Source + "=" + s.Value
	}
	if sources["ship.publish.mode"] != `repository="never"` || sources["ship.issues.keyword"] != `user="Fixes"` ||
		sources["ship.pullRequest.labels"] != "default=true" || len(view.Files) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if publishes, err := PublishesAfterShip(context.Background(), ws); publishes || err != nil {
		t.Fatalf("ship.publish never: PublishesAfterShip = %v, %v", publishes, err)
	}
}

func TestPolicyInConfigAndDoctor(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{Policy: `{"organization":"Acme","enforce":{"ship":{"publish":{"mode":"never"}}}}`}
	ws.Config = files

	view, err := ShowConfig(context.Background(), ws)
	if err != nil || view.Policy == nil || view.Policy.Contact != "Acme" || strings.Join(view.Policy.Locked, ",") != "ship.publish.mode" {
		t.Fatalf("view = %+v, %v", view, err)
	}
	report := CheckUp(context.Background(), ws)
	if !hasFinding(report, "doctor.policy.active") || report.Blocking() {
		t.Fatalf("report = %+v", report.Findings)
	}

	// The repository contradicts the policy: refused, and the doctor blocks.
	files.Repository = `{"ship":{"publish":{"mode":"auto"}}}`
	if _, err := PublishesAfterShip(context.Background(), ws); !IsPolicyRefusal(err) {
		t.Fatalf("error = %v", err)
	}
	report = CheckUp(context.Background(), ws)
	if !report.Blocking() || !hasFinding(report, "config.policy_conflict") {
		t.Fatalf("report = %+v", report.Findings)
	}
}

func hasFinding(report DoctorReport, key string) bool {
	for _, f := range report.Findings {
		if f.Key == key {
			return true
		}
	}
	return false
}

func TestInitWritesAreasFromTheHistory(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.History = []string{"feat(api): a", "fix(api): b", "feat(api): c", "feat(ui): d", "docs: e"}
	ws, _, _ := workspaceOf(git)
	files := &testkit.FakeConfigFiles{}
	ws.Config = files
	result, err := Init(context.Background(), ws)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Areas, ",") != "api" || !strings.Contains(files.Repository, `"areas": ["api"]`) {
		t.Fatalf("result = %+v, file:\n%s", result, files.Repository)
	}
	if _, err := Init(context.Background(), ws); !errors.Is(err, &fault.Fault{Code: fault.ConfigAlreadyThere}) {
		t.Fatalf("second init: %v", err)
	}
}

func TestPlanAndApplySync(t *testing.T) {
	ws, _, log := workspaceOf(testkit.HealthyRepository("main"))
	forge := &testkit.FakeForge{RepositoryLabels: []settings.Label{{Name: "bug"}, {Name: "help wanted"}}}
	ws.Forge, ws.Config = forge, &testkit.FakeConfigFiles{}

	plan, err := PlanSync(context.Background(), ws)
	if err != nil || plan.Empty() || plan.ReadOnly || plan.Labels[0].From != "bug" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	result, err := ApplySync(context.Background(), ws, plan)
	if err != nil || result.Applied != len(plan.Labels) || forge.LabelCalls[0] != "update bug → kind/bug" {
		t.Fatalf("result = %+v, %v, calls %v", result, err, forge.LabelCalls)
	}
	if op := log.Operations[0]; op.Kind != "sync" || len(op.LabelChanges) != result.Applied {
		t.Errorf("operation = %+v", op)
	}
	if again, _ := PlanSync(context.Background(), ws); !again.Empty() {
		t.Fatalf("a second sync must find nothing to do: %+v", again.Labels)
	}
}

func TestSyncIsReadOnlyWhenAnotherToolManagesTheRepository(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	forge := &testkit.FakeForge{}
	ws.Forge, ws.Config = forge, &testkit.FakeConfigFiles{Repository: `{"github":{"managedBy":"opentofu"}}`}
	plan, err := PlanSync(context.Background(), ws)
	if err != nil || !plan.ReadOnly || plan.ManagedBy != "opentofu" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if _, err := ApplySync(context.Background(), ws, plan); !errors.Is(err, &fault.Fault{Code: fault.SyncReadOnly}) || len(forge.LabelCalls) != 0 {
		t.Fatalf("apply on a managed repository: %v, calls %v", err, forge.LabelCalls)
	}
}

func TestPublishFollowsTheSettings(t *testing.T) {
	forge := &testkit.FakeForge{RepositoryLabels: testkit.LabelNames("kind/feature", "size/S")}
	ws, _ := publishWorkspace(publishableGit(nil), forge)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship":{"pullRequest":{"draft":"never","labels":false},"issues":{"keyword":"Refs"}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{Unattended: true}); err != nil {
		t.Fatal(err)
	}
	spec := forge.Created[0]
	if spec.Draft || !strings.HasPrefix(spec.Body, "Refs #42") || len(forge.Added) != 0 {
		t.Fatalf("spec = %+v, labels %v", spec, forge.Added)
	}
}

func TestBrokenConfigStopsBeforeAnyChange(t *testing.T) {
	var calls []string
	ws, _ := publishWorkspace(publishableGit(&calls), &testkit.FakeForge{})
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship":{"publish":{"mode":"sometimes"}}}`}
	if _, err := Publish(context.Background(), ws, PublishRequest{}); !errors.Is(err, &fault.Fault{Code: fault.ConfigInvalidValue}) || len(calls) != 0 {
		t.Fatalf("error = %v, calls %v", err, calls)
	}
}
