// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestLabel(t *testing.T) {
	forge := &testkit.FakeForge{
		RepositoryLabels: testkit.LabelNames("kind/bug", "size/XS"),
		PullRequests: map[int]ports.PullRequestContent{5: {
			PullRequest:    ports.PullRequest{Number: 5},
			CommitMessages: []string{"🐛 fix: round vat"},
			Files:          []ports.FileStat{{Path: "vat.go", Added: 2, Deleted: 1}},
		}},
	}
	labelRun := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		deps := Deps{Git: testkit.HealthyRepository("main"), Checkpoints: &testkit.FakeCheckpointStore{},
			Operations: &testkit.FakeOperationLog{}, Forge: forge, OperatingSystem: "linux"}
		code := Run(context.Background(), args, Streams{In: strings.NewReader(""), Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
		return code, out.String(), errOut.String()
	}
	code, _, stderr := labelRun("label", "--pr", "5")
	if code != ExitOK || !strings.Contains(stderr, "Pull request #5: added kind/bug, size/XS; removed -.") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if code, _, _ := labelRun("label"); code != ExitUsage {
		t.Errorf("without --pr: exit %d", code)
	}
	if code, _, stderr := labelRun("label", "--pr", "6"); code != ExitError || !strings.Contains(stderr, "SHIP-E0605") {
		t.Errorf("unknown pull request: exit %d, %q", code, stderr)
	}
}
