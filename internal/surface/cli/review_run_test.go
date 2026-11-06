// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// runReview runs `ship` with a terminal on stdin (answered by stdin) and on
// stderr when terminalUI, in the environment env.
func runReview(t *testing.T, stdin string, terminalUI bool, env []string, calls *[]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	git.Calls = calls
	var out, errOut bytes.Buffer
	streams := Streams{In: strings.NewReader(stdin), Out: &out, Err: &errOut, Interactive: true, TerminalUI: terminalUI}
	deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{},
		Forge: &testkit.FakeForge{}, OperatingSystem: "linux", Environ: env}
	code = Run(context.Background(), append([]string{"--local"}, args...), streams, i18n.English, testBuild, deps)
	return code, out.String(), errOut.String()
}

func TestPlainReviewAsksLineByLine(t *testing.T) {
	var calls []string
	// Enter keeps the title and the branch; why and tested are typed; Enter ships.
	code, stdout, stderr := runReview(t, "\n\nThe run hung on Windows.\ngo test ./...\n\n", true, nil, &calls, "--plain")
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	prompts := "Title [🐛 fix: update run]: Branch [fix/update-run]: Why? (optional, Enter to skip): " +
		"Tested? (optional, Enter to skip): Ship it? [Y/n] "
	if !strings.HasPrefix(stderr, prompts) {
		t.Errorf("prompts, exactly, on stderr:\n%q\nwant prefix\n%q", stderr, prompts)
	}
	if !strings.Contains(stdout, "🐛 fix: update run") || strings.Contains(stdout, "Title [") {
		t.Errorf("stdout keeps the plan, not the prompts: %q", stdout)
	}
	want := "commit 🐛 fix: update run\n\nThe run hung on Windows.\n\nTested: go test ./...\n"
	if got := calls[len(calls)-1]; got != want {
		t.Errorf("commit = %q, want %q", got, want)
	}
}

// TestPlainReviewAsksForTheType: the project's commitlint refuses the
// planned type; the prompt lists the allowed ones, by number or name, with
// no default, and the title then proposed carries the pick.
func TestPlainReviewAsksForTheType(t *testing.T) {
	var errOut bytes.Buffer
	r := runner{ctx: context.Background(), lang: i18n.English,
		streams: Streams{In: strings.NewReader("\nfeature\n2\n\n\n\n\n\n"), Err: &errOut}}
	cfg := reviewConfig{lang: i18n.English, plan: perfPlan(),
		checkBranch: func(name string) app.BranchCheck { return app.CheckBranch(name) },
		branchFor: func(title string) (string, bool) {
			b, err := app.BranchForTitle(title, 0)
			return b, err == nil
		}}
	outcome := r.plainReview(cfg)
	if outcome.action != reviewShip || outcome.title != "⚡️ fix(cart): cache vat rates" || outcome.branch != "fix/cache-vat-rates" {
		t.Fatalf("outcome = %+v\n%s", outcome, errOut.String())
	}
	prompt := "The project's commitlint refuses this type. Type (1) refactor  2) fix): "
	want := prompt + "Answer with a number or one of: refactor, fix.\n" + prompt + "Answer with a number or one of: refactor, fix.\n" +
		prompt + "Title [⚡️ fix(cart): cache vat rates]: Branch [fix/cache-vat-rates]: "
	if !strings.HasPrefix(errOut.String(), want) {
		t.Fatalf("prompts:\n%q\nwant prefix\n%q", errOut.String(), want)
	}
	// The end of the input cancels: no type is picked for the human.
	r.streams.In = strings.NewReader("")
	if outcome := r.plainReview(cfg); outcome.action != reviewCancel {
		t.Fatalf("end of input: %+v", outcome)
	}
}

func TestAccessibleReviewAsksAgainAndCanBeDeclined(t *testing.T) {
	var calls []string
	stdin := "Fix: Broken.\n✨ feat: add retries\nfeat/add-retries\n\n\nn\n"
	code, _, stderr := runReview(t, stdin, true, []string{"SHIP_ACCESSIBLE=1"}, &calls, "--why", "Flaky network.")
	if code != ExitAborted || len(calls) != 0 {
		t.Fatalf("declined: exit %d, calls %q\n%s", code, calls, stderr)
	}
	for _, want := range []string{"SHIP-E01", "Title [🐛 fix: update run]: SHIP-E0106  Start the subject with a lowercase letter.\nTitle [🐛 fix: update run]: Branch [feat/add-retries]: ", "Why? [Flaky network.]: ", "Nothing changed."} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr misses %q:\n%s", want, stderr)
		}
	}
	// The end of the input cancels: nothing is decided for the human.
	if code, _, _ := runReview(t, "\n", true, []string{"TERM=dumb"}, &calls); code != ExitAborted || len(calls) != 0 {
		t.Fatalf("end of input: exit %d, calls %q", code, calls)
	}
}

func TestReviewScreenEndToEnd(t *testing.T) {
	var calls []string
	// Shift+Tab twice to Why?, type "x", then F2 ships. Bubble
	// Tea reads the keys from a plain reader as from a terminal.
	code, stdout, stderr := runReview(t, "\x1b[Z\x1b[Zx\x1bOQ", true, []string{"TERM=xterm-256color"}, &calls)
	if code != ExitOK {
		t.Fatalf("exit %d\nstderr %q", code, stderr)
	}
	if got := calls[len(calls)-1]; got != "commit 🐛 fix: update run\n\nx\n" {
		t.Errorf("commit = %q (the why typed on the screen is the body)", got)
	}
	if stdout != "" {
		t.Errorf("the screen draws on stderr only; stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "Committed c0ffee on the new branch fix/update-run") {
		t.Errorf("stderr = %q", stderr)
	}
	// Esc cancels and changes nothing.
	calls = nil
	if code, _, _ := runReview(t, "\x1b", true, []string{"TERM=xterm-256color"}, &calls); code != ExitAborted || len(calls) != 0 {
		t.Fatalf("esc: exit %d, calls %q", code, calls)
	}
}
