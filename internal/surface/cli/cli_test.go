// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

var testBuild = BuildInfo{Version: "1.2.3", Commit: "abc", Date: "2026-10-23"}

// runWith executes the CLI against fakes; stdin answers prompts.
func runWith(git testkit.FakeGit, stdin string, interactive bool, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	streams := Streams{In: strings.NewReader(stdin), Out: &out, Err: &errOut, Interactive: interactive}
	deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{}, Forge: &testkit.FakeForge{}, OperatingSystem: "linux"}
	code = Run(context.Background(), args, streams, i18n.English, testBuild, deps)
	return code, out.String(), errOut.String()
}

func run(args ...string) (code int, stdout, stderr string) {
	return runWith(testkit.HealthyRepository("main"), "", false, args...)
}

func TestShipNeedsAHumanWhenNotInteractive(t *testing.T) {
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	if code, stdout, _ := runWith(git, "", false); code != ExitNeedsHuman || !strings.Contains(stdout, "fix: update run") {
		t.Fatalf("exit %d, plan %q", code, stdout)
	}
}

func TestShipAsksAndDelivers(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	git.Calls = &calls
	git.CommitMessages = []string{"🐛 fix: update run"}
	code, stdout, stderr := runWith(git, "\n", true) // Enter accepts
	// fetch (tidy), switch, stage, commit, fetch (publish), push.
	if code != ExitOK || len(calls) != 6 || calls[0] != "fetch origin" || calls[5] != "push origin fix/update-run" {
		t.Fatalf("exit %d, calls %q, stderr %q", code, calls, stderr)
	}
	for _, want := range []string{"new branch fix/update-run", "Pull request #1 opened against main"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr misses %q:\n%s", want, stderr)
		}
	}
	if !strings.HasSuffix(stdout, "https://github.com/acme/shop/pull/1\n") {
		t.Errorf("the pull request URL must end stdout: %q", stdout)
	}
	calls = nil
	// Declining changes nothing; only the read-only fetch of the tidy ran.
	if code, _, _ := runWith(git, "n\n", true); code != ExitAborted || strings.Join(calls, ",") != "fetch origin" {
		t.Fatalf("declined: exit %d, calls %q", code, calls)
	}
}

func TestShipLocalNeverPushes(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	git.Calls = &calls
	code, _, stderr := runWith(git, "", false, "--yes", "--local")
	if code != ExitOK || len(calls) != 3 || !strings.Contains(stderr, "ship publish") {
		t.Fatalf("exit %d, calls %q, stderr %q", code, calls, stderr)
	}
}

// TestNoTokenSaidOnce: without a token, ship --yes says SHIP-E0603 once.
func TestNoTokenSaidOnce(t *testing.T) {
	var out, errOut bytes.Buffer
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{},
		Forge: &testkit.FakeForge{Err: ports.ErrNotAuthenticated}, OperatingSystem: "linux"}
	Run(context.Background(), []string{"--yes"}, Streams{Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	if n := strings.Count(errOut.String(), "SHIP-E0603"); n != 1 {
		t.Errorf("SHIP-E0603 said %d times:\n%s", n, errOut.String())
	}
}

func TestShipWithoutTokenKeepsTheCommitAndAsksAHuman(t *testing.T) {
	var out, errOut bytes.Buffer
	var calls []string
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	git.Calls = &calls
	deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{},
		Forge: &testkit.FakeForge{Err: ports.ErrNotAuthenticated}, OperatingSystem: "linux"}
	code := Run(context.Background(), []string{"--yes"}, Streams{Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	// Tidy asks GitHub first, warns without a token and goes on; then
	// switch, stage, commit: no push.
	if code != ExitNeedsHuman || len(calls) != 3 || calls[2] != "commit 🐛 fix: update run\n" {
		t.Fatalf("exit %d, calls %q", code, calls)
	}
	for _, want := range []string{"SHIP-E0603", "gh auth login", "Committed locally"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr misses %q:\n%s", want, errOut.String())
		}
	}
}

func TestCheckTitle(t *testing.T) {
	if code, _, _ := run("check", "title", "✨ feat(ui): add dark mode"); code != ExitOK {
		t.Fatalf("valid title exit = %d", code)
	}
	code, stdout, stderr := run("check", "title", "Feat: Add dark mode.")
	if code != ExitError {
		t.Fatalf("invalid title exit = %d", code)
	}
	if stdout != "" {
		t.Errorf("diagnostics must go to stderr, stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "SHIP-E0106") || !strings.Contains(stderr, "SHIP-E0107") {
		t.Errorf("stderr misses codes: %q", stderr)
	}
}

func TestCheckTitleJSON(t *testing.T) {
	code, stdout, _ := run("check", "title", "--json", "feat: add dark mode")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var result struct {
		Canonical string `json:"canonical"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout)
	}
	if result.Canonical != "✨ feat: add dark mode" {
		t.Errorf("canonical = %q", result.Canonical)
	}
}

func TestBranchSuggestion(t *testing.T) {
	code, stdout, _ := run("branch", "--issue", "42", "feat", "add the login page")
	if code != ExitOK || strings.TrimSpace(stdout) != "feat/42-add-login-page" {
		t.Fatalf("branch = %q (exit %d)", stdout, code)
	}
	if code, _, _ := run("branch", "nope", "x"); code != ExitError {
		t.Fatalf("unknown type exit = %d", code)
	}
}

func TestCheckBranch(t *testing.T) {
	if code, _, _ := run("check", "branch", "feat/42-add-login"); code != ExitOK {
		t.Fatal("valid branch rejected")
	}
	if code, _, stderr := run("check", "branch", "feature/Add_Login"); code != ExitError || !strings.Contains(stderr, "SHIP-E0110") {
		t.Fatalf("invalid branch: exit %d, stderr %q", code, stderr)
	}
}

func TestUsageAndVersion(t *testing.T) {
	if code, _, _ := run(); code != ExitError {
		t.Fatalf("ship on a clean tree exit = %d, want nothing-to-ship error", code)
	}
	if code, _, _ := run("frobnicate"); code != ExitUsage {
		t.Fatalf("unknown command exit = %d", code)
	}
	if _, stdout, _ := run("version"); !strings.Contains(stdout, "1.2.3") {
		t.Fatalf("version output = %q", stdout)
	}
}

func TestDoctorCommand(t *testing.T) {
	code, stdout, _ := run("doctor")
	if code != ExitOK || !strings.Contains(stdout, "✓ ok") || !strings.Contains(stdout, "Ready to ship.") {
		t.Fatalf("healthy doctor: exit %d\n%s", code, stdout)
	}
	broken := testkit.HealthyRepository("main")
	broken.ConfigValues = nil
	code, stdout, _ = runWith(broken, "", false, "doctor")
	if code != ExitError || !strings.Contains(stdout, "✗ block") || !strings.Contains(stdout, "git config --global user.name") {
		t.Fatalf("blocked doctor: exit %d\n%s", code, stdout)
	}
	if code, stdout, _ := run("doctor", "--json"); code != ExitOK || !strings.Contains(stdout, `"worst": "ok"`) {
		t.Fatalf("doctor --json: exit %d\n%s", code, stdout)
	}
}

func TestCheckpointCommands(t *testing.T) {
	var out, errOut bytes.Buffer
	store := &testkit.FakeCheckpointStore{}
	deps := Deps{Git: testkit.HealthyRepository("main"), Checkpoints: store, OperatingSystem: "linux"}
	exec := func(args ...string) int {
		out.Reset()
		errOut.Reset()
		return Run(context.Background(), args, Streams{Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	}
	if exec("checkpoint", "list") != ExitOK || !strings.Contains(errOut.String(), "No checkpoint yet.") {
		t.Fatalf("empty list: %q", errOut.String())
	}
	if exec("checkpoint", "--label", "try") != ExitOK || !strings.Contains(errOut.String(), "created") {
		t.Fatalf("create: %q", errOut.String())
	}
	if exec("checkpoint", "list") != ExitOK || !strings.Contains(out.String(), "try") {
		t.Fatalf("list: %q", out.String())
	}
	if exec("checkpoint", "restore", "a000") != ExitOK || store.Restored == "" {
		t.Fatalf("restore: %q", errOut.String())
	}
	if exec("checkpoint", "restore") != ExitUsage {
		t.Fatal("restore without id must be a usage error")
	}
	if exec("checkpoint", "restore", "zzz") != ExitError || !strings.Contains(errOut.String(), "SHIP-E0401") {
		t.Fatalf("unknown id: %q", errOut.String())
	}
}

func TestUndoCommand(t *testing.T) {
	var calls []string
	var errOut bytes.Buffer
	git := testkit.HealthyRepository("feat/add-vat")
	git.WorkingTree.Branch.Commit = "c1"
	git.Calls = &calls
	log := &testkit.FakeOperationLog{Operations: []oplog.Operation{{
		ID: "op1", Kind: oplog.Deliver, Title: "✨ feat: add vat", CreatedBranch: true,
		Before: oplog.RepositoryView{Branch: "main", Head: "h0"},
		After:  oplog.RepositoryView{Branch: "feat/add-vat", Head: "c1"},
	}}}
	deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: log, OperatingSystem: "linux"}
	exec := func() int {
		errOut.Reset()
		return Run(context.Background(), []string{"undo"}, Streams{Out: io.Discard, Err: &errOut}, i18n.English, testBuild, deps)
	}
	if code := exec(); code != ExitOK || len(calls) != 3 {
		t.Fatalf("undo: exit %d, calls %q, stderr %q", code, calls, errOut.String())
	}
	for _, want := range []string{"Undone: ✨ feat: add vat", "on main", "Branch feat/add-vat removed", "ship checkpoint restore a000"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr misses %q:\n%s", want, errOut.String())
		}
	}
	if code := exec(); code != ExitError || !strings.Contains(errOut.String(), "SHIP-E0403") {
		t.Fatalf("second undo: exit %d, %q", code, errOut.String())
	}
}

// TestUndoSaysWhatItReverted: an operation without a title (mcp install, a
// fix) or without uncommitted changes to give back (a rename, a stack sync)
// gets its own line, never "Undone: . Your changes are uncommitted again".
func TestUndoSaysWhatItReverted(t *testing.T) {
	stack := &oplog.StackRecord{Base: "main", Layers: []oplog.StackLayer{{Branch: "feat/a", Commit: "a1"}, {Branch: "feat/b", Commit: "b1"}}}
	synced := &oplog.StackRecord{Base: "main", Layers: []oplog.StackLayer{
		{Branch: "feat/b", Previous: "b1", Commit: "b2"}, {Branch: "feat/a", Previous: "a1", Deleted: true}}}
	tests := []struct {
		name   string
		branch string
		op     oplog.Operation
		want   string
	}{
		{"mcp install", "main", oplog.Operation{ID: "op1", Kind: oplog.Fix,
			FilesBefore: []oplog.FileBefore{{Path: ".mcp.json"}}}, "↶ Undone: put back as before: .mcp.json.\n"},
		{"fix", "main", oplog.Operation{ID: "op1", Kind: oplog.Fix, Untracked: []string{".env"},
			ConfigChanges: []oplog.ConfigChange{{Key: "core.autocrlf"}}}, "↶ Undone: put back as before: .env, core.autocrlf.\n"},
		{"rename", "feat/42-add-vat", oplog.Operation{ID: "op1", Kind: oplog.Rename, Title: "feat/add-vat",
			Before: oplog.RepositoryView{Branch: "feat/add-vat", Head: "c1"}, After: oplog.RepositoryView{Branch: "feat/42-add-vat", Head: "c1"}},
			"↶ Undone: the branch feat/42-add-vat is called feat/add-vat again.\n"},
		{"roadmap", "main", oplog.Operation{ID: "op1", Kind: oplog.Roadmap, Title: "ROADMAP.md",
			FilesBefore: []oplog.FileBefore{{Path: "ROADMAP.md", Existed: true}}},
			"↶ Undone: the roadmap sync of ROADMAP.md. The file is back as it was, and what the sync did on GitHub is reverted.\n"},
		{"stack", "feat/b", oplog.Operation{ID: "op1", Kind: oplog.Stack, Title: "✨ feat: a", CreatedBranch: true, Stack: stack,
			Before: oplog.RepositoryView{Branch: "main", Head: "h0"}, After: oplog.RepositoryView{Branch: "feat/b", Head: "c1"}},
			"↶ Undone: the stack of 2 layers. Your changes are uncommitted again on main; branches removed: feat/b, feat/a.\n"},
		{"stack sync", "feat/b", oplog.Operation{ID: "op1", Kind: oplog.StackSync, Title: "main", Stack: synced,
			Before: oplog.RepositoryView{Branch: "feat/a", Head: "a1"}, After: oplog.RepositoryView{Branch: "feat/b", Head: "c1"}},
			"↶ Undone: the stack sync. The layers are back where they were (feat/a, feat/b); you are on feat/a.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			git := testkit.HealthyRepository(tt.branch)
			git.WorkingTree.Branch.Commit = "c1"
			deps := Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{Operations: []oplog.Operation{tt.op}},
				OperatingSystem: "linux"}
			code := Run(context.Background(), []string{"undo"}, Streams{Out: io.Discard, Err: &errOut}, i18n.English, testBuild, deps)
			if code != ExitOK || !strings.HasPrefix(errOut.String(), tt.want) {
				t.Fatalf("exit %d, stderr:\n%s\nwant prefix\n%s", code, errOut.String(), tt.want)
			}
		})
	}
}

func TestGarbageCollectionCommand(t *testing.T) {
	if code, _, stderr := run("gc", "--dry-run"); code != ExitOK || !strings.Contains(stderr, "Would remove 0 old checkpoints; 0 kept") {
		t.Fatalf("gc --dry-run: exit %d, %q", code, stderr)
	}
	if code, stdout, _ := run("gc", "--json"); code != ExitOK || !strings.Contains(stdout, `"removed": []`) {
		t.Fatalf("gc --json: exit %d, %s", code, stdout)
	}
}

func TestPlanCommand(t *testing.T) {
	git := testkit.HealthyRepository("main", "docs/guide.md")
	git.DiffText = "diff --git a/docs/guide.md b/docs/guide.md\n--- a/docs/guide.md\n+++ b/docs/guide.md\n@@ -1 +1 @@\n-a\n+b\n"
	code, stdout, _ := runWith(git, "", false, "plan")
	if code != ExitOK || !strings.Contains(stdout, "📝 docs: update guide docs") || !strings.Contains(stdout, "documentation") {
		t.Fatalf("plan: exit %d\n%s", code, stdout)
	}
	if code, _, stderr := run("plan"); code != ExitError || !strings.Contains(stderr, "SHIP-E0201") {
		t.Fatalf("plan on a clean tree: exit %d, %q", code, stderr)
	}
}

// TestAWordAfterTheFlagsDoesNothing: `ship --yes --local plan` once committed
// the work instead of planning it. A word left after ship's flags stops
// everything with a usage error.
func TestAWordAfterTheFlagsDoesNothing(t *testing.T) {
	for _, args := range [][]string{{"--yes", "--local", "plan"}, {"--yes", "undo"}, {"--json", "status"}} {
		code, _, stderr := run(args...)
		if code != ExitUsage || !strings.Contains(stderr, `Unexpected argument "`+args[len(args)-1]+`"`) {
			t.Errorf("ship %v: exit %d, %q", args, code, stderr)
		}
		// The hint is a command that works: `ship plan --yes` does not.
		if word := args[len(args)-1]; !strings.Contains(stderr, `"ship `+word+`".`) {
			t.Errorf("ship %v: the hint must suggest ship %s alone: %q", args, word, stderr)
		}
	}
}
