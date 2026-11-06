// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestParseGlobalFlags(t *testing.T) {
	g, rest := parseGlobalFlags([]string{"-d", "publish", "--draft", "-q", "--trace", "--", "-d"})
	if !g.debug || !g.quiet || !g.trace || !slices.Equal(rest, []string{"publish", "--draft", "--", "-d"}) {
		t.Fatalf("flags %+v, rest %q", g, rest)
	}
	if g, rest := parseGlobalFlags([]string{"--yes", "--plain"}); !g.plain || !slices.Equal(rest, []string{"--yes"}) {
		t.Errorf("--plain is global: %+v, %q", g, rest)
	}
	if g, rest := parseGlobalFlags([]string{"--debug", "--yes"}); !g.debug || !slices.Equal(rest, []string{"--yes"}) {
		t.Fatalf("flags %+v, rest %q", g, rest)
	}
}

func TestCommandNameHidesArguments(t *testing.T) {
	for args, want := range map[string]string{
		"":                           "ship",
		"--yes":                      "ship",
		"publish --draft":            "publish",
		"check title feat: secret x": "check title",
		"branch feat secret-subject": "branch",
	} {
		if got := commandName(strings.Fields(args)); got != want {
			t.Errorf("commandName(%q) = %q, want %q", args, got, want)
		}
	}
}

// runJournaled runs the CLI with a journal attached to a temporary directory.
func runJournaled(t *testing.T, deps Deps, args ...string) (code int, stdout, stderr string, j *obs.Journal) {
	t.Helper()
	j = obs.NewJournal(obs.JournalOptions{})
	if err := j.Attach(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, Streams{In: strings.NewReader(""), Out: &out, Err: &errOut, Journal: j}, i18n.English, testBuild, deps)
	return code, out.String(), errOut.String(), j
}

func healthyDeps() Deps {
	return Deps{Git: testkit.HealthyRepository("main"), Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{}, Forge: &testkit.FakeForge{}, OperatingSystem: "linux"}
}

func TestQuietKeepsOnlyWhatMatters(t *testing.T) {
	if code, _, stderr := run("checkpoint"); code != ExitOK || !strings.Contains(stderr, "Checkpoint") {
		t.Fatalf("checkpoint: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := run("-q", "checkpoint"); code != ExitOK || stderr != "" {
		t.Fatalf("checkpoint -q: exit %d, stderr %q", code, stderr)
	}
	// Errors are never silenced.
	if code, _, stderr := run("--quiet", "nope"); code != ExitUsage || !strings.Contains(stderr, "nope") {
		t.Fatalf("unknown command -q: exit %d, stderr %q", code, stderr)
	}
}

func TestDebugEchoesTheJournalOnStderr(t *testing.T) {
	code, stdout, stderr := run("--debug", "version")
	if code != ExitOK || !strings.Contains(stderr, "msg=run.start") || !strings.Contains(stderr, "msg=run.end") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if strings.Contains(stdout, "run.") {
		t.Fatalf("the journal reached stdout: %q", stdout)
	}
}

func TestTracePrintsTheStepsOnStderr(t *testing.T) {
	code, stdout, stderr := run("--trace", "undo")
	if code != ExitError || !strings.Contains(stderr, "steps") || !strings.Contains(stderr, "undo") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if strings.Contains(stdout, "steps") {
		t.Fatalf("the timeline reached stdout: %q", stdout)
	}
}

func TestUnexpectedErrorSavesTheBlackBox(t *testing.T) {
	git := brokenGit{testkit.HealthyRepository("main")}
	code, _, stderr, j := runJournaled(t, Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{}, Forge: &testkit.FakeForge{}, OperatingSystem: "linux"}, "status", "--offline")
	if code != ExitError {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	path := filepath.Join(j.Dir(), string(j.RunID)+obs.BlackBoxSuffix)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("black box not saved: %v\n%s", err, stderr)
	}
	if !strings.Contains(string(data), "run.error") || !strings.Contains(string(data), "run.end") {
		t.Errorf("black box content:\n%s", data)
	}
	if !strings.Contains(stderr, string(j.RunID)+obs.BlackBoxSuffix) {
		t.Errorf("the error must point to the black box: %q", stderr)
	}
}

// brokenGit fails in a way ship-it does not expect.
type brokenGit struct{ testkit.FakeGit }

func (brokenGit) Status(context.Context) (ports.WorkingTreeStatus, error) {
	return ports.WorkingTreeStatus{}, os.ErrPermission
}

// outsideGit is git run outside a repository.
type outsideGit struct{ testkit.FakeGit }

func (outsideGit) Status(context.Context) (ports.WorkingTreeStatus, error) {
	return ports.WorkingTreeStatus{}, fmt.Errorf("git status: exit 128: %w", ports.ErrNotARepository)
}

func TestOutsideARepositoryIsAnAnswer(t *testing.T) {
	git := outsideGit{testkit.HealthyRepository("main")}
	code, _, stderr, j := runJournaled(t, Deps{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{}, Forge: &testkit.FakeForge{}, OperatingSystem: "linux"}, "status", "--offline")
	if code != ExitError || !strings.Contains(stderr, "SHIP-E0316  Not inside a git repository: cd into your project, or run git init.") ||
		strings.Contains(stderr, "exit 128") || strings.Contains(stderr, obs.BlackBoxSuffix) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(j.Dir(), string(j.RunID)+obs.BlackBoxSuffix)); !os.IsNotExist(err) {
		t.Fatal("being outside a repository must not leave a black box")
	}
}

func TestGitHubWithoutAnswerIsAnAnswer(t *testing.T) {
	deps := healthyDeps()
	deps.Forge = &testkit.FakeForge{Err: &ports.UnreachableError{Host: "github.com", Err: errors.New("proxyconnect tcp: dial tcp 127.0.0.1:9: connection refused")}}
	deps.Git = testkit.HealthyRepository("feat/x")
	code, _, stderr, j := runJournaled(t, deps, "publish")
	if code != ExitError || !strings.Contains(stderr, "SHIP-E0618  GitHub (github.com) does not answer: see https://www.githubstatus.com and your proxy (HTTPS_PROXY).") ||
		strings.Contains(stderr, "proxyconnect") {
		t.Fatalf("publish: exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(j.Dir(), string(j.RunID)+obs.BlackBoxSuffix)); !os.IsNotExist(err) {
		t.Fatal("GitHub without answer must not leave a black box")
	}
}

func TestExpectedRefusalKeepsNoBlackBox(t *testing.T) {
	code, _, _, j := runJournaled(t, healthyDeps(), "nope")
	if code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(j.Dir(), string(j.RunID)+obs.BlackBoxSuffix)); !os.IsNotExist(err) {
		t.Fatal("a usage error must not leave a black box")
	}
}

func TestAKnownFaultKeepsNoBlackBox(t *testing.T) {
	// An invalid title is an answer, not a failure of ship-it: no black box,
	// no invitation to report a bug.
	code, _, stderr, j := runJournaled(t, healthyDeps(), "check", "title", "--", "Fixed the bug.")
	if code != ExitError || strings.Contains(stderr, "bug report") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(j.Dir(), string(j.RunID)+obs.BlackBoxSuffix)); !os.IsNotExist(err) {
		t.Fatal("a known fault must not leave a black box")
	}
}

func TestPanicIsRecoveredAndReported(t *testing.T) {
	// No Git adapter at all: the use case dereferences a nil interface.
	code, _, stderr, j := runJournaled(t, Deps{}, "status", "--offline")
	if code != ExitError || !strings.Contains(stderr, "internal error") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(j.Dir(), string(j.RunID)+obs.BlackBoxSuffix))
	if err != nil || !strings.Contains(string(data), "run.panic") {
		t.Fatalf("black box: %v\n%s", err, data)
	}
}

func TestTraceCommandReadsAPastRun(t *testing.T) {
	dir := t.TempDir()
	past := obs.NewJournal(obs.JournalOptions{})
	if err := past.Attach(dir); err != nil {
		t.Fatal(err)
	}
	past.Logger.Info("step.done", "step", "publish/push", "duration_ms", 812)
	past.Close()

	now := obs.NewJournal(obs.JournalOptions{})
	if err := now.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer now.Close()
	var out, errOut bytes.Buffer
	streams := Streams{Out: &out, Err: &errOut, Journal: now}
	if code := Run(context.Background(), []string{"trace"}, streams, i18n.English, testBuild, healthyDeps()); code != ExitOK {
		t.Fatalf("trace: exit %d, %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), string(past.RunID)) || !strings.Contains(out.String(), "publish/push") || !strings.Contains(out.String(), "812ms") {
		t.Fatalf("trace output:\n%s", out.String())
	}
	out.Reset()
	if code := Run(context.Background(), []string{"trace", "--list"}, streams, i18n.English, testBuild, healthyDeps()); code != ExitOK ||
		strings.Count(out.String(), "\n") != 1 || !strings.Contains(out.String(), string(past.RunID)) {
		t.Fatalf("trace --list: exit %d\n%s", code, out.String())
	}
	if code := Run(context.Background(), []string{"trace", "NOPE00"}, streams, i18n.English, testBuild, healthyDeps()); code != ExitError {
		t.Fatalf("unknown run: exit %d", code)
	}
}

func TestGcPrunesRunLogs(t *testing.T) {
	dir := t.TempDir()
	for range 22 {
		old := obs.NewJournal(obs.JournalOptions{})
		if err := old.Attach(dir); err != nil {
			t.Fatal(err)
		}
		old.Logger.Info("run.end")
		old.Close()
	}
	now := obs.NewJournal(obs.JournalOptions{})
	if err := now.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer now.Close()
	var out, errOut bytes.Buffer
	streams := Streams{Out: &out, Err: &errOut, Journal: now}
	if code := Run(context.Background(), []string{"gc", "--dry-run"}, streams, i18n.English, testBuild, healthyDeps()); code != ExitOK ||
		!strings.Contains(errOut.String(), "Would remove 3 old run logs") {
		t.Fatalf("gc --dry-run: exit %d, %s", code, errOut.String())
	}
	if runs, _ := obs.ListRuns(dir); len(runs) != 23 {
		t.Fatalf("a dry run removed logs: %d runs left", len(runs))
	}
	errOut.Reset()
	if code := Run(context.Background(), []string{"gc"}, streams, i18n.English, testBuild, healthyDeps()); code != ExitOK ||
		!strings.Contains(errOut.String(), "Removed 3 old run logs") {
		t.Fatalf("gc: exit %d, %s", code, errOut.String())
	}
	if runs, _ := obs.ListRuns(dir); len(runs) != 20 {
		t.Fatalf("%d runs left, want 20", len(runs))
	}
}

// TestConfiguredLogLimits applies ship.logs: the retention of ship gc and of
// the post-run pruning, and the black box size.
func TestConfiguredLogLimits(t *testing.T) {
	dir := t.TempDir()
	for range 6 {
		old := obs.NewJournal(obs.JournalOptions{})
		if err := old.Attach(dir); err != nil {
			t.Fatal(err)
		}
		old.Logger.Info("run.end")
		old.Close()
	}
	deps := healthyDeps()
	deps.Config = &testkit.FakeConfigFiles{User: `{"ship": {"logs": {"keepRuns": 3, "blackBoxBytes": 65536}}}`}
	now := obs.NewJournal(obs.JournalOptions{})
	if err := now.Attach(dir); err != nil {
		t.Fatal(err)
	}
	defer now.Close()
	var out, errOut bytes.Buffer
	streams := Streams{Out: &out, Err: &errOut, Journal: now}
	if code := Run(context.Background(), []string{"gc", "--dry-run"}, streams, i18n.English, testBuild, deps); code != ExitOK ||
		!strings.Contains(errOut.String(), "Would remove 4 old run logs") {
		t.Fatalf("gc --dry-run: exit %d, %s", code, errOut.String())
	}
	if got := now.Retention(); got.Runs != 3 || got.Bytes != 10<<20 {
		t.Fatalf("retention = %+v", got)
	}
	// Post-run pruning (any command but gc) uses the same limits.
	if code := Run(context.Background(), []string{"version"}, streams, i18n.English, testBuild, deps); code != ExitOK {
		t.Fatalf("version: exit %d", code)
	}
	if runs, _ := obs.ListRuns(dir); len(runs) != 3 {
		t.Fatalf("%d runs left, want 3", len(runs))
	}
	// The black box now holds at most 64 KiB.
	for range 2000 {
		now.Logger.Debug("git.exec", "cmd", strings.Repeat("x", 100))
	}
	var box bytes.Buffer
	now.DumpBlackBox(&box)
	if box.Len() > 65536 || box.Len() < 60000 {
		t.Fatalf("black box holds %d bytes, want about 64 KiB", box.Len())
	}
}
