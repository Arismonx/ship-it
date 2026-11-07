// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestStatusOnTheDefaultBranch: the default branch has no pull request of its
// own and status does not ask GitHub for one, so it says nothing about it
// rather than "unknown (offline)".
func TestStatusOnTheDefaultBranch(t *testing.T) {
	s := newScenario(t, nil)
	s.write("main.go", "package main\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("update-ref", "refs/remotes/origin/main", "HEAD")
	s.sh("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	code, stdout, stderr := s.ship("status")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "Pull request") || strings.Contains(stdout, "offline") {
		t.Errorf("status on main talks about a pull request:\n%s", stdout)
	}
}

// TestStatusWithABaseNeverFetched: origin/HEAD names a branch that was never
// fetched, so status cannot compare with it and says no base rather than
// "0 ahead, 0 behind" (one git process, the failing comparison, tells).
func TestStatusWithABaseNeverFetched(t *testing.T) {
	s := newScenario(t, nil)
	s.write("main.go", "package main\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("switch", "--quiet", "--create", "feat/x")
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	code, stdout, stderr := s.ship("status", "--offline", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var view struct {
		Base        string `json:"base"`
		AheadOfBase int    `json:"ahead_of_base"`
	}
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.Base != "" {
		t.Errorf("base = %q, want none (origin/main was never fetched)", view.Base)
	}

	s.sh("update-ref", "refs/remotes/origin/main", "HEAD")
	s.write("b.go", "package main\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "✨ feat: b")
	_, stdout, _ = s.ship("status", "--offline", "--json")
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatal(err)
	}
	if view.Base != "main" || view.AheadOfBase != 1 {
		t.Errorf("base %q, %d ahead; want main, 1 ahead", view.Base, view.AheadOfBase)
	}
}
