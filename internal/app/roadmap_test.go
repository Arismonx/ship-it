// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const sampleRoadmap = "# Roadmap\n\n## Phase 1 — Foundations\n**Due:** 2026-12-15\n" +
	"- [ ] feat: CLI skeleton\n  - [ ] git layer\n- [x] ADRs\n**Exit:** a junior ships a PR.\n"

func roadmapWorkspace(t *testing.T) (Workspace, *testkit.FakeTracker, *testkit.FakeRoadmapMemory, testkit.FakeGit) {
	t.Helper()
	git := testkit.HealthyRepository("main")
	git.WorkingFiles = map[string]string{RoadmapFile: sampleRoadmap}
	tracker, memory := &testkit.FakeTracker{}, &testkit.FakeRoadmapMemory{}
	ws, _, _ := workspaceOf(git)
	ws.Forge = &testkit.FakeForge{RepositoryLabels: testkit.LabelNames("kind/feature")}
	ws.Tracker, ws.RoadmapMemory = tracker, memory
	return ws, tracker, memory, git
}

func TestRoadmapSyncCreatesWritesBackAndUndoes(t *testing.T) {
	ws, tracker, memory, git := roadmapWorkspace(t)
	ctx := context.Background()
	plan, err := PlanRoadmap(ctx, ws, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyRoadmap(ctx, ws, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FileChanged || result.Operation == "" {
		t.Fatalf("result = %+v", result)
	}
	// Milestone, three issues (one a sub-issue of the first, one closed), numbers written back.
	if len(tracker.MilestoneList) != 1 || tracker.MilestoneList[0].Due != "2026-12-15" || len(tracker.IssueList) != 3 {
		t.Fatalf("milestones %+v, issues %+v", tracker.MilestoneList, tracker.IssueList)
	}
	if got := tracker.Children[1]; len(got) != 1 || got[0] != 2 {
		t.Fatalf("sub-issues of #1 = %v", got)
	}
	if adr := tracker.IssueList[2]; adr.Open || adr.Title != "ADRs" {
		t.Fatalf("ADRs = %+v", adr)
	}
	if cli := tracker.IssueList[0]; cli.Title != "CLI skeleton" || len(cli.Labels) != 1 || !strings.Contains(cli.Body, roadmap.Marker) {
		t.Fatalf("CLI = %+v", cli)
	}
	file := git.WorkingFiles[RoadmapFile]
	for _, want := range []string{"- [ ] feat: CLI skeleton #1\n", "  - [ ] git layer #2\n", "- [x] ADRs #3\n"} {
		if !strings.Contains(file, want) {
			t.Errorf("file misses %q:\n%s", want, file)
		}
	}
	if memory.Memory[3] != true || memory.Memory[1] != false {
		t.Errorf("memory = %v", memory.Memory)
	}

	// A second sync has nothing to do.
	again, err := PlanRoadmap(ctx, ws, "")
	if err != nil || !again.Empty() {
		t.Fatalf("second plan = %+v, %v", again.Changes, err)
	}

	// Undo needs consent, then closes what was created and restores the file.
	if _, err := Undo(ctx, ws, UndoRequest{}); !errors.Is(err, &fault.Fault{Code: fault.UndoNeedsConsent}) {
		t.Fatalf("undo without consent: %v", err)
	}
	if _, err := Undo(ctx, ws, UndoRequest{AllowRemote: true}); err != nil {
		t.Fatal(err)
	}
	for _, is := range tracker.IssueList {
		if is.Open {
			t.Errorf("issue #%d still open after undo", is.Number)
		}
	}
	if len(tracker.MilestoneList) != 0 {
		t.Errorf("the created milestone holds only created issues: deleted, got %+v", tracker.MilestoneList)
	}
	if git.WorkingFiles[RoadmapFile] != sampleRoadmap {
		t.Errorf("file not restored:\n%s", git.WorkingFiles[RoadmapFile])
	}
}

func TestRoadmapSyncFollowsGitHubClosing(t *testing.T) {
	ws, tracker, _, git := roadmapWorkspace(t)
	ctx := context.Background()
	plan, _ := PlanRoadmap(ctx, ws, "")
	if _, err := ApplyRoadmap(ctx, ws, plan); err != nil {
		t.Fatal(err)
	}
	// A pull request closed "git layer" (#2) on GitHub.
	tracker.IssueList[1].Open = false
	plan, err := PlanRoadmap(ctx, ws, "")
	if err != nil || len(plan.Changes) != 1 || plan.Changes[0].Kind != roadmap.TickBox {
		t.Fatalf("plan = %+v, %v", plan.Changes, err)
	}
	if _, err := ApplyRoadmap(ctx, ws, plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(git.WorkingFiles[RoadmapFile], "  - [x] git layer #2\n") {
		t.Fatalf("box not ticked:\n%s", git.WorkingFiles[RoadmapFile])
	}
}

func TestRoadmapMissing(t *testing.T) {
	ws, _, _, git := roadmapWorkspace(t)
	delete(git.WorkingFiles, RoadmapFile)
	if _, err := PlanRoadmap(context.Background(), ws, ""); !errors.Is(err, &fault.Fault{Code: fault.RoadmapMissing}) {
		t.Fatalf("err = %v", err)
	}
}
