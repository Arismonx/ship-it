// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const reviewSettings = `{"ship": {"pullRequest": {"reviewers": ["ada", "grace"], "teamReviewers": ["backend"], "assignSelf": true}}}`

func reviewWorkspace() (Workspace, *testkit.FakeForge, *testkit.FakeTracker, *testkit.FakeOperationLog) {
	forge := &testkit.FakeForge{User: ports.User{Login: "ada"}}
	tracker := &testkit.FakeTracker{IssueList: []roadmap.Issue{{Number: 1}, {Number: 42, Open: true, Milestone: 3}}}
	ws, log := publishWorkspace(publishableGit(nil), forge)
	ws.Tracker = tracker
	ws.Config = &testkit.FakeConfigFiles{Repository: reviewSettings}
	return ws, forge, tracker, log
}

func TestPublishRequestsReviewsAssignsAndFollowsTheMilestone(t *testing.T) {
	ws, forge, tracker, log := reviewWorkspace()
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// The author is never asked to review their own pull request.
	if !reflect.DeepEqual(forge.Reviewers, []string{"#1 grace,@backend"}) || !reflect.DeepEqual(result.Reviewers, []string{"grace", "@backend"}) {
		t.Errorf("reviewers = %q, result %q", forge.Reviewers, result.Reviewers)
	}
	if result.Assignee != "ada" || !slices.Contains(tracker.Calls, "assign 1 ada") {
		t.Errorf("assignee = %q, calls %q", result.Assignee, tracker.Calls)
	}
	if result.Milestone != 3 || tracker.IssueList[0].Milestone != 3 || log.Operations[0].PullRequestMilestone != 3 {
		t.Errorf("milestone = %d, pull request %+v, operation %+v", result.Milestone, tracker.IssueList[0], log.Operations[0])
	}

	// ship undo takes the pull request out of the milestone, then closes it.
	if _, err := Undo(context.Background(), ws, UndoRequest{AllowRemote: true}); err != nil {
		t.Fatal(err)
	}
	if tracker.IssueList[0].Milestone != 0 || !reflect.DeepEqual(forge.Closed, []int{1}) {
		t.Errorf("after undo: pull request %+v, closed %v", tracker.IssueList[0], forge.Closed)
	}
}

func TestPublishWithoutMilestone(t *testing.T) {
	ws, _, tracker, _ := reviewWorkspace()
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"pullRequest": {"milestone": "none"}}}`}
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || result.Milestone != 0 || tracker.IssueList[0].Milestone != 0 || result.Assignee != "" {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestRefusedReviewsAreWarnings(t *testing.T) {
	ws, forge, _, _ := reviewWorkspace()
	forge.ReviewersErr = errors.New("github: 422 Reviews may only be requested from collaborators")
	result, err := Publish(context.Background(), ws, PublishRequest{})
	if err != nil || !result.Created || len(result.Warnings) != 1 || len(result.Reviewers) != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestReviewsAreRehearsed(t *testing.T) {
	ws, _, _, _ := reviewWorkspace()
	ws.Forge = testkit.GuardForge(t, ws.Forge)
	ws.Tracker = testkit.GuardTracker(t, ws.Tracker)
	result, err := Publish(context.Background(), DryRun(ws), PublishRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range result.Planned {
		keys = append(keys, s.Key)
	}
	for _, want := range []string{"app.dryrun.request_reviewers_new", "app.dryrun.assign_new_pull_request", "app.dryrun.milestone_new_pull_request"} {
		if !slices.Contains(keys, want) {
			t.Errorf("planned %q lacks %s", keys, want)
		}
	}
}
