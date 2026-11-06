// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestCommandHelp(t *testing.T) {
	code, stdout, _ := run("plan", "--help")
	if code != ExitOK || !strings.HasPrefix(stdout, "Usage: ship plan [flags]\n") || !strings.Contains(stdout, "  --issue int ") {
		t.Fatalf("plan --help: exit %d\n%s", code, stdout)
	}
	code, stdout, _ = run("stack", "--help")
	for _, sub := range []string{"status", "sync", "merge", "publish"} {
		if code != ExitOK || !strings.Contains(stdout, "\n  "+sub+" ") {
			t.Errorf("stack --help misses %s: exit %d\n%s", sub, code, stdout)
		}
	}
	if code, _, stderr := run("check", "title", "--help"); code != ExitOK || stderr != "" {
		t.Errorf("check title --help: exit %d, %q", code, stderr)
	}
	// A wrong flag is still a usage error, pointing to the help.
	code, stdout, stderr := run("plan", "--nope")
	if code != ExitUsage || stdout != "" || !strings.Contains(stderr, `Run "ship plan --help" for its flags.`) {
		t.Errorf("plan --nope: exit %d\n%s%s", code, stdout, stderr)
	}
}

// TestFlagsGoAnywhere: a flag after the arguments counts, and a command that
// takes no argument refuses one instead of ignoring it.
func TestFlagsGoAnywhere(t *testing.T) {
	if code, stdout, _ := run("branch", "feat", "add", "login", "--issue", "42"); code != ExitOK || strings.TrimSpace(stdout) != "feat/42-add-login" {
		t.Errorf("branch: exit %d, %q", code, stdout)
	}
	if code, stdout, _ := run("check", "branch", "feat/x", "--json"); code != ExitOK || !strings.HasPrefix(stdout, "{") {
		t.Errorf("check branch --json: exit %d, %q", code, stdout)
	}
	if code, stdout, _ := run("check", "title", "feat: x", "--json"); code != ExitOK || !strings.HasPrefix(stdout, "{") {
		t.Errorf("check title --json: exit %d, %q", code, stdout)
	}
	// After "--", a word starting with a dash is an argument.
	if code, stdout, _ := run("check", "title", "--json", "--", "feat: x"); code != ExitOK || !strings.HasPrefix(stdout, "{") {
		t.Errorf("check title -- : exit %d, %q", code, stdout)
	}
	for _, args := range [][]string{{"undo", "now"}, {"checkpoint", "create", "--label", "exp2"}, {"status", "x"}, {"stack", "sync", "x"}} {
		code, stdout, stderr := run(args...)
		if code != ExitUsage || stdout != "" || !strings.Contains(stderr, `Unexpected argument "`+args[1]) && !strings.Contains(stderr, `Unexpected argument "x"`) {
			t.Errorf("%q: exit %d\n%s%s", args, code, stdout, stderr)
		}
	}
	store := &testkit.FakeCheckpointStore{}
	var out, errOut bytes.Buffer
	deps := Deps{Git: testkit.HealthyRepository("main"), Checkpoints: store, Operations: &testkit.FakeOperationLog{}, OperatingSystem: "linux"}
	if code := Run(context.Background(), []string{"checkpoint", "--json", "--label", "exp2"}, Streams{Out: &out, Err: &errOut}, i18n.English, testBuild, deps); code != ExitOK || !strings.Contains(out.String(), `"label": "exp2"`) {
		t.Errorf("checkpoint --label: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

func TestVersionJSON(t *testing.T) {
	code, stdout, _ := run("version", "--json")
	var v map[string]string
	if err := json.Unmarshal([]byte(stdout), &v); code != ExitOK || err != nil ||
		v["version"] != "1.2.3" || v["commit"] != "abc" || v["date"] != "2026-10-23" || !strings.HasPrefix(v["go"], "go") {
		t.Fatalf("exit %d, %v\n%s", code, err, stdout)
	}
}

// TestCheckpointDryRun: --dry-run says what a checkpoint or a restore would
// do and takes none.
func TestCheckpointDryRun(t *testing.T) {
	store := &testkit.FakeCheckpointStore{}
	var out, errOut bytes.Buffer
	deps := Deps{Git: testkit.HealthyRepository("main"), Checkpoints: store, Operations: &testkit.FakeOperationLog{}, OperatingSystem: "linux"}
	exec := func(args ...string) int {
		out.Reset()
		errOut.Reset()
		return Run(context.Background(), args, Streams{Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	}
	if code := exec("checkpoint", "--label", "try", "--dry-run"); code != ExitOK || !strings.Contains(out.String(), "try") {
		t.Fatalf("checkpoint --dry-run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if views, _ := store.List(context.Background(), 10); len(views) != 0 {
		t.Fatalf("a dry run took a checkpoint: %v", views)
	}
	if exec("checkpoint", "--label", "real") != ExitOK {
		t.Fatal(errOut.String())
	}
	views, _ := store.List(context.Background(), 10)
	if code := exec("checkpoint", "restore", views[0].ID, "--dry-run"); code != ExitOK || store.Restored != "" || !strings.Contains(out.String(), "Dry run") {
		t.Fatalf("restore --dry-run: exit %d, restored %q\n%s%s", code, store.Restored, out.String(), errOut.String())
	}
	if views, _ := store.List(context.Background(), 10); len(views) != 1 {
		t.Fatalf("a dry run took a safety checkpoint: %v", views)
	}
}

// TestGroupHelp: the commands made of subcommands answer --help with their
// usage, exit 0.
func TestGroupHelp(t *testing.T) {
	for _, args := range [][]string{{"mcp", "--help"}, {"check", "--help"}, {"roadmap", "--help"}, {"check", "-h"}} {
		code, stdout, stderr := run(args...)
		if code != ExitOK || stderr != "" || !strings.HasPrefix(stdout, "Usage: ship "+args[0]) || !strings.Contains(stdout, "Commands:") {
			t.Errorf("%q: exit %d\n%s%s", args, code, stdout, stderr)
		}
	}
	code, stdout, _ := run("checkpoint", "--help")
	if code != ExitOK || !strings.Contains(stdout, "ship checkpoint list") || !strings.Contains(stdout, "ship checkpoint restore <id>") {
		t.Errorf("checkpoint --help: exit %d\n%s", code, stdout)
	}
	for _, sub := range []string{"list", "restore"} {
		if _, stdout, _ := run("checkpoint", sub, "--help"); strings.Contains(stdout, "--label") {
			t.Errorf("checkpoint %s --help shows --label:\n%s", sub, stdout)
		}
	}
	if _, stdout, _ := run("undo", "--help"); !strings.Contains(stdout, "\n  -y ") {
		t.Errorf("undo has no -y:\n%s", stdout)
	}
	if _, stdout, _ := run("stack", "merge", "--help"); !strings.Contains(stdout, "\n  -y ") {
		t.Errorf("stack merge has no -y:\n%s", stdout)
	}
}

// TestUsageIsAligned: every row of ship --help starts its description in the
// same column, in every language, and names every stack subcommand.
func TestUsageIsAligned(t *testing.T) {
	for _, lang := range []i18n.Language{i18n.English, i18n.French} {
		usage := i18n.Render(lang, "cli.usage", nil)
		column := -1
		for _, line := range strings.Split(usage, "\n") {
			if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
				continue // not a row: the second usage line
			}
			gap := strings.Index(line[2:], "  ")
			if gap < 0 {
				t.Errorf("%s: row without a description: %q", lang, line)
				continue
			}
			rest := line[2+gap:]
			at := utf8.RuneCountInString(line[:2+gap]) + utf8.RuneCountInString(rest) - utf8.RuneCountInString(strings.TrimLeft(rest, " "))
			if column < 0 {
				column = at
			}
			if at != column {
				t.Errorf("%s: description at column %d, want %d: %q", lang, at, column, line)
			}
		}
		for _, sub := range []string{"stack status", "stack sync", "stack merge", "stack publish"} {
			if !strings.Contains(usage, "\n  "+sub) {
				t.Errorf("%s: usage misses %s", lang, sub)
			}
		}
		if first := strings.SplitN(usage, "\n", 2)[0]; !strings.Contains(first, "--issue N]") {
			t.Errorf("%s: the first usage line is cut: %q", lang, first)
		}
	}
}

func runGates(runner *testkit.FakeRunner, args ...string) (int, string, string) {
	git := testkit.HealthyRepository("main", "a.go")
	git.WorkingFiles = map[string]string{"package.json": `{"scripts": {"lint": "eslint .", "test": "vitest"}}`, "package-lock.json": "{}"}
	git.DiffText = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-a\n+b\n"
	git.CommitMessages = []string{"🐛 fix: update a"}
	var out, errOut bytes.Buffer
	deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{}, Forge: &testkit.FakeForge{}, Runner: runner, OperatingSystem: "linux"}
	code := Run(context.Background(), args, Streams{In: strings.NewReader(""), Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	return code, out.String(), errOut.String()
}

func TestCheckGates(t *testing.T) {
	if code, _, stderr := runGates(&testkit.FakeRunner{}, "check", "gates"); code != ExitOK || !strings.Contains(stderr, "Every check passed.") {
		t.Fatalf("green: exit %d\n%s", code, stderr)
	}
	red := &testkit.FakeRunner{Failing: map[string]string{"npm run lint": "a.ts:1 error"}}
	if code, _, stderr := runGates(red, "check", "gates"); code != ExitGate || !strings.Contains(stderr, "SHIP-E0902") {
		t.Fatalf("red: exit %d\n%s", code, stderr)
	}
	runner := &testkit.FakeRunner{}
	if code, stdout, _ := runGates(runner, "check", "gates", "--dry-run"); code != ExitOK || len(runner.Calls) != 0 || !strings.Contains(stdout, "lint") {
		t.Fatalf("dry run: exit %d, runs %v\n%s", code, runner.Calls, stdout)
	}
}

// TestLocalDeliverySaysWhenChecksRun: after a local commit, one line says the
// checks wait for the publication, and how to run them now.
func TestLocalDeliverySaysWhenChecksRun(t *testing.T) {
	runner := &testkit.FakeRunner{}
	code, _, stderr := runGates(runner, "--yes", "--local")
	if code != ExitOK || len(runner.Calls) != 0 || !strings.Contains(stderr, "ship check gates runs them now") {
		t.Fatalf("exit %d, runs %v\n%s", code, runner.Calls, stderr)
	}
}
