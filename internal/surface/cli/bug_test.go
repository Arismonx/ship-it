// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/obs"
)

// TestBugReportHoldsOnlyTheBlackBox: #53. A run that logged a path, an
// email and a token: the report and its address carry none of them, only
// what the scrubbed black box holds.
func TestBugReportHoldsOnlyTheBlackBox(t *testing.T) {
	const home, email, token = "/home/ada", "ada@example.com", "ghp_0123456789abcdefghijABCDEFGHIJ012345"
	j := obs.NewJournal(obs.JournalOptions{Scrubber: obs.NewScrubber(map[string]string{home: "~"})})
	j.Logger.Info("run.start", "command", "publish", "version", "v0.2.0", "os", "linux", "arch", "amd64")
	j.Logger.Info("git.run", obs.KeyStep, "git/push", "cmd", "git push "+home+"/shop "+email, obs.KeyDurationMS, 12)
	j.Logger.Error("run.error", "error", errors.New("push to https://x:"+token+"@github.com failed in "+home+"/shop"))
	j.Logger.Info("run.end", "command", "publish", "exit_code", 1)
	var box bytes.Buffer
	if err := j.DumpBlackBox(&box); err != nil {
		t.Fatal(err)
	}
	events, err := obs.ReadEvents(&box)
	if err != nil {
		t.Fatal(err)
	}
	report := buildBugReport("01JRUN", events, "git version 2.52.0")
	address, _ := url.QueryUnescape(report.IssueURL("01JRUN.report.md"))
	for _, text := range []string{report.Markdown(), address} {
		for _, secret := range []string{home, email, token} {
			if strings.Contains(text, secret) {
				t.Errorf("%q leaked into:\n%s", secret, text)
			}
		}
	}
	if !strings.Contains(report.Problem, "failed in ~/shop") {
		t.Errorf("the error must be there, scrubbed: %q", report.Problem)
	}
	md := report.Markdown()
	for _, want := range []string{"ship publish", "v0.2.0", "linux/amd64", "Exit code: `1`", "git/push", "git version 2.52.0"} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	if !strings.HasPrefix(report.IssueURL("f.md"), bugIssueURL+"?") || !strings.Contains(address, "template=1-bug-report.yml") ||
		!strings.Contains(address, "ship-version=v0.2.0") || !strings.Contains(address, "attached: 01JRUN.report.md") {
		t.Errorf("address = %s", address)
	}
}

func TestChooseBugRun(t *testing.T) {
	runs := []obs.RunFiles{{ID: "03CURRENT"}, {ID: "02OK"}, {ID: "01FAILED", BlackBox: true}}
	if got := chooseBugRun(runs, "03CURRENT", ""); got == nil || got.ID != "01FAILED" {
		t.Errorf("default: %+v, want the last failure", got)
	}
	if got := chooseBugRun(runs[:2], "03CURRENT", ""); got == nil || got.ID != "02OK" {
		t.Errorf("no failure: %+v, want the newest", got)
	}
	if got := chooseBugRun(runs, "03CURRENT", "02ok"); got == nil || got.ID != "02OK" {
		t.Errorf("by id: %+v", got)
	}
	if got := chooseBugRun(runs, "03CURRENT", "03CURRENT"); got != nil {
		t.Errorf("the current run is no history: %+v", got)
	}
}
