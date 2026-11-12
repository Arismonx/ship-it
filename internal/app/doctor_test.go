// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestDoctorGathersFacts(t *testing.T) {
	git := testkit.HealthyRepository("")
	git.WorkingTree.Branch.Detached = true
	git.RemoteNames = []string{"origin", "backup"}
	git.MatchingValues = map[string]string{"remote.backup.mirror": "true", "remote.origin.mirror": "false"}

	report := Doctor(context.Background(), git, "linux")
	if report.Worst != "warning" || report.Blocking() {
		t.Fatalf("worst = %q, findings %+v", report.Worst, report.Findings)
	}
	var mirror *FindingView
	for i := range report.Findings {
		if report.Findings[i].Key == "doctor.remote.mirror" {
			mirror = &report.Findings[i]
		}
	}
	if mirror == nil || mirror.Params["remote"] != "backup" {
		t.Fatalf("mirror finding missing or wrong: %+v", report.Findings)
	}
}

type tokenFrom string

func (s tokenFrom) Token(context.Context, string) (ports.Token, error) {
	if s == "" {
		return ports.Token{}, ports.ErrNotAuthenticated
	}
	return ports.NewToken("t", string(s)), nil
}

func TestDoctorChecksGitHub(t *testing.T) {
	tests := []struct {
		name     string
		token    tokenFrom
		forgeErr error
		key      string
		worst    string
	}{
		{"granted", "gh", nil, "doctor.github.ok", "ok"},
		{"no token", "", nil, "doctor.github.no_token", "warning"},
		{"rejected", "GH_TOKEN", ports.ErrTokenRejected, "doctor.github.token_rejected", "warning"},
		{"hidden", "gh", ports.ErrNotFound, "doctor.github.repository_hidden", "warning"},
		{"offline", "gh", errors.New("dial tcp: no route"), "doctor.github.unreachable", "warning"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
			ws.Forge, ws.Credentials = &testkit.FakeForge{Err: tt.forgeErr}, tt.token
			report := CheckUp(context.Background(), ws)
			last := report.Findings[len(report.Findings)-1]
			if last.Key != tt.key || report.Worst != tt.worst || last.Params["repository"] != "acme/shop" {
				t.Fatalf("last finding = %+v, worst %s", last, report.Worst)
			}
		})
	}
}

// TestCheckUpDeep: #54. The token's scopes come from the repository read,
// and without a runner the project's checks are reported as not run.
func TestCheckUpDeep(t *testing.T) {
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	ws.Forge, ws.Credentials = &testkit.FakeForge{Private: true, TokenScopes: []string{"read:org"}}, tokenFrom("gh")
	ws.Runner = nil
	report := CheckUpDeep(context.Background(), ws, nil)
	if !report.Blocking() || !hasFinding(report, "doctor.deep.scope_missing") ||
		!hasFinding(report, "doctor.deep.rulesets_ok") || !hasFinding(report, "doctor.deep.unchecked_gates") {
		t.Fatalf("report = %+v", report.Findings)
	}

	// GitHub unreachable: the scopes and rulesets are said unchecked.
	ws.Forge = &testkit.FakeForge{Err: errors.New("dial tcp: no route")}
	report = CheckUpDeep(context.Background(), ws, nil)
	if !hasFinding(report, "doctor.deep.unchecked_scopes") || !hasFinding(report, "doctor.deep.unchecked_rulesets") {
		t.Fatalf("offline report = %+v", report.Findings)
	}
}

func TestDoctorWithoutGitHubRemoteSkipsTheNetwork(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.RemoteURLs = map[string]string{"origin": "/srv/git/shop.git"}
	ws, _, _ := workspaceOf(git)
	ws.Forge, ws.Credentials = &testkit.FakeForge{Err: errors.New("must not be called")}, tokenFrom("")
	for _, f := range CheckUp(context.Background(), ws).Findings {
		if strings.HasPrefix(f.Key, "doctor.github.") {
			t.Fatalf("unexpected GitHub finding %+v", f)
		}
	}
}

func TestDoctorBlocksWithoutIdentity(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.ConfigValues = nil
	if report := Doctor(context.Background(), git, "linux"); !report.Blocking() {
		t.Fatalf("missing identity must block: %+v", report)
	}
}
