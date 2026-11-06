// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"sync/atomic"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// benchRepo is the repository of the end-to-end benchmarks
// (docs/design/performance.md): 2,000 files, 300 commits, on a feature branch
// with 30 changed and 5 new files. GitHub is never reached.
var benchRepo = benchdata.RepoOptions{Files: 2000, Commits: 300, Modified: 30, Untracked: 5}

// gitCounter counts the git.exec records of the journal: one per git
// process (gitexec.Client.log), the cost that dominates on Windows.
type gitCounter struct {
	slog.Handler
	n *atomic.Int64
}

func (h gitCounter) Enabled(context.Context, slog.Level) bool { return true }

func (h gitCounter) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "git.exec" {
		h.n.Add(1)
	}
	if h.Handler.Enabled(ctx, r.Level) {
		return h.Handler.Handle(ctx, r)
	}
	return nil
}

// benchEnvironment isolates the run from the user's configuration: a
// temporary home, user directories and global git configuration.
func benchEnvironment(b *testing.B) platform.Environment {
	b.Helper()
	home := b.TempDir()
	vars := map[string]string{
		"XDG_STATE_HOME": filepath.Join(home, "state"), "XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"XDG_CACHE_HOME": filepath.Join(home, "cache"), "LocalAppData": filepath.Join(home, "AppData", "Local"),
		"AppData": filepath.Join(home, "AppData", "Roaming"), "TMPDIR": os.TempDir(), "TMP": os.TempDir(),
	}
	for k, v := range vars {
		b.Setenv(k, v)
	}
	b.Setenv("HOME", home)
	b.Setenv("USERPROFILE", home)
	b.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	b.Setenv("SHIP_LANG", "en")
	return platform.Environment{GOOS: runtime.GOOS, Home: home, Getenv: func(k string) string { return vars[k] }}
}

func newBenchRepo(b *testing.B) string {
	b.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git unavailable")
	}
	dir := filepath.Join(b.TempDir(), "widget")
	if err := benchdata.NewRepository(dir, benchRepo); err != nil {
		b.Fatal(err)
	}
	return dir
}

// benchCLI runs `ship args…` in dir as main does: a fresh git client (its
// caches live for one run), the journal, every real adapter. after, when
// set, puts the repository back outside the timing; such a command also runs
// once before the timing, as the first run in a repository creates the
// checkpoint store (BenchmarkCreateCold measures that). It reports the git
// processes per run as git/op.
func benchCLI(b *testing.B, dir string, after func(), args ...string) {
	env := benchEnvironment(b)
	b.Chdir(dir)
	if after != nil {
		runCLI(b, env, nil, args)
		after()
	}
	var processes atomic.Int64
	runs := 0
	for b.Loop() {
		runCLI(b, env, &processes, args)
		runs++
		if after != nil {
			b.StopTimer()
			after()
			b.StartTimer()
		}
	}
	b.ReportMetric(float64(processes.Load())/float64(runs), "git/op")
}

// runCLI runs ship once in the current directory, adding its git processes
// to processes when not nil.
func runCLI(b *testing.B, env platform.Environment, processes *atomic.Int64, args []string) {
	if processes == nil {
		processes = new(atomic.Int64)
	}
	ctx := context.Background()
	git := gitexec.New(".")
	// The rev-parse of openJournal runs before the journal exists.
	git.Log = slog.New(gitCounter{Handler: slog.DiscardHandler, n: processes})
	journal := openJournal(ctx, git, env)
	git.Log = slog.New(gitCounter{Handler: journal.Logger.Handler(), n: processes})
	ws := workspace(git, journal)
	var out, errOut bytes.Buffer
	code := cli.Run(ctx, args, cli.Streams{Out: &out, Err: &errOut, Journal: journal}, i18n.English, cli.BuildInfo{Version: "bench"}, ws)
	journal.Close()
	if code != cli.ExitOK {
		b.Fatalf("ship %s: exit %d\n%s%s", strings.Join(args, " "), code, out.String(), errOut.String())
	}
}

func BenchmarkStatusOffline(b *testing.B) {
	benchCLI(b, newBenchRepo(b), nil, "status", "--offline")
}

func BenchmarkPlan(b *testing.B) {
	benchCLI(b, newBenchRepo(b), nil, "plan")
}

// BenchmarkShipLocal commits the work in progress (`ship --local --yes`);
// a mixed reset brings it back for the next run.
func BenchmarkShipLocal(b *testing.B) {
	dir := newBenchRepo(b)
	reset := func() {
		cmd := exec.Command("git", "reset", "--quiet", "--mixed", "HEAD~1")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("reset: %v: %s", err, out)
		}
	}
	benchCLI(b, dir, reset, "--local", "--yes")
}

// BenchmarkBinaryStatusOffline runs the real executable, built from this
// package (the go build cache makes it quick): what a user waits for,
// process start included. --debug echoes the git.exec records, counted as
// git/op, plus the rev-parse that runs before the journal starts.
func BenchmarkBinaryStatusOffline(b *testing.B) {
	exe := filepath.Join(b.TempDir(), "ship")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		b.Skipf("go build: %v\n%s", err, out)
	}
	dir := newBenchRepo(b)
	benchEnvironment(b)
	processes, runs := 0, 0
	for b.Loop() {
		cmd := exec.Command(exe, "--debug", "status", "--offline")
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			b.Fatalf("ship status: %v\n%s", err, stderr.String())
		}
		processes += strings.Count(stderr.String(), "msg=git.exec") + 1
		runs++
	}
	b.ReportMetric(float64(processes)/float64(runs), "git/op")
}
