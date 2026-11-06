// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestRoadmapPlanShowsTheProject(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.WorkingFiles = map[string]string{"ROADMAP.md": "## Phase 1\n**Due:** 2026-12-15\n- [ ] CLI #1\n"}
	deps := healthyDeps()
	deps.Git = git
	deps.Tracker = &testkit.FakeTracker{IssueList: []roadmap.Issue{{Number: 1, ID: 1001, Title: "CLI", Open: true}}}
	deps.Projects = &testkit.FakeProjects{Fields: []roadmap.ProjectField{
		{ID: 2, Name: "Status", Type: roadmap.SingleSelectField, Options: []roadmap.FieldOption{{ID: "t", Name: "Todo"}, {ID: "d", Name: "Done"}}},
		{ID: 3, Name: "Target date", Type: roadmap.DateField},
	}}
	deps.Config = &testkit.FakeConfigFiles{Repository: `{"github": {"project": {"url": "acme/7", "dateField": "Target date", "phaseField": "Phase"}}}`}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"roadmap", "sync"}, Streams{In: strings.NewReader(""), Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	if code != ExitNeedsHuman {
		t.Fatalf("code = %d, stderr:\n%s", code, errOut.String())
	}
	for _, want := range []string{
		"▦ add #1 to project acme/7",
		"▦ #1: Status → Todo",
		"▦ #1: Target date → 2026-12-15",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout misses %q:\n%s", want, out.String())
		}
	}
	if want := `SHIP-W0714  Project acme/7 has no field "Phase"`; !strings.Contains(errOut.String(), want) {
		t.Errorf("stderr misses %q:\n%s", want, errOut.String())
	}
}
