// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// testEnvironment places the user directories under a temporary home.
func testEnvironment(t *testing.T) platform.Environment {
	t.Helper()
	home := t.TempDir()
	vars := map[string]string{
		"XDG_STATE_HOME":  filepath.Join(home, "state"),
		"LocalAppData":    filepath.Join(home, "AppData", "Local"),
		"AppData":         filepath.Join(home, "AppData", "Roaming"),
		"XDG_CONFIG_HOME": filepath.Join(home, "config"),
		"TMPDIR":          os.TempDir(), "TMP": os.TempDir(),
	}
	return platform.Environment{GOOS: runtime.GOOS, Home: home, Getenv: func(k string) string { return vars[k] }}
}

// journaled runs the CLI like main does: one journal per run.
func (s *scenario) journaled(env platform.Environment, args ...string) (code int, stdout, stderr string, j *obs.Journal) {
	s.t.Helper()
	git := gitexec.New(s.dir)
	j = openJournal(context.Background(), git, env)
	defer j.Close()
	deps := s.deps
	deps.Git, deps.Log, deps.RunLogs = git, j.Logger, j
	var out, errOut bytes.Buffer
	code = cli.Run(context.Background(), args, cli.Streams{Out: &out, Err: &errOut, Journal: j}, i18n.English, cli.BuildInfo{Version: "test"}, deps)
	return code, out.String(), errOut.String(), j
}

// TestRunJournal ships work in a real repository and reads the journal back:
// where it is, what it says, and what it never says.
func TestRunJournal(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	env := testEnvironment(t)
	s := newScenario(t, nil)
	s.sh("remote", "add", "origin", "https://bob:hunter2hunter2@example.invalid/acme/shop.git")
	s.write("main.go", "package main\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.write("main.go", "package main\n\n// confidential content\nfunc main() {}\n")

	code, stdout, stderr, j := s.journaled(env, "--yes", "--local", "--trace")
	if code != cli.ExitOK {
		t.Fatalf("ship --yes --local: exit %d\n%s", code, stderr)
	}
	// --trace: the timeline on stderr, never on stdout.
	if !strings.Contains(stderr, "deliver") || !strings.Contains(stderr, "git") || strings.Contains(stdout, "ms  ") {
		t.Errorf("--trace timeline:\nstdout %q\nstderr %s", stdout, stderr)
	}

	gitDir := s.sh("rev-parse", "--absolute-git-dir")
	logs := platform.RepositoryLogDir(filepath.Clean(gitDir))
	if j.Dir() != logs {
		t.Fatalf("journal in %q, want %q", j.Dir(), logs)
	}
	data, err := os.ReadFile(filepath.Join(logs, string(j.RunID)+obs.JournalSuffix))
	if err != nil {
		t.Fatal(err)
	}
	events, _ := obs.ReadEvents(bytes.NewReader(data))
	var deliver *obs.Event
	for i, e := range events {
		if e.Step == "deliver" {
			deliver = &events[i]
		}
	}
	if len(events) == 0 || events[0].Msg != "run.start" || events[len(events)-1].Msg != "run.end" {
		t.Fatalf("journal must open with run.start and close with run.end:\n%s", data)
	}
	if deliver == nil || deliver.Msg != "step.done" || deliver.Attrs["op_id"] == nil {
		t.Fatalf("no deliver step with its op_id:\n%s", data)
	}
	var first map[string]any
	json.Unmarshal(bytes.SplitN(data, []byte("\n"), 2)[0], &first)
	if first["run_id"] != string(j.RunID) || !strings.HasPrefix(fmt.Sprint(first["repo_hash"]), "hmac-sha256:") || first["severity_number"] != float64(9) {
		t.Errorf("first record = %v", first)
	}
	// The salt keying repo_hash lives in the (temporary) user config dir.
	if saltFile, _ := env.SaltFile(); !strings.HasPrefix(saltFile, env.Home) {
		t.Fatalf("salt file %q outside the test home", saltFile)
	} else if info, err := os.Stat(saltFile); err != nil || info.Size() != obs.SaltSize {
		t.Fatalf("salt file: %v", err)
	}
	// DEBUG (git commands) stays in the black box: no file on success.
	if strings.Contains(string(data), "git.exec") {
		t.Error("DEBUG records reached the INFO journal")
	}
	if _, err := os.Stat(filepath.Join(logs, string(j.RunID)+obs.BlackBoxSuffix)); !os.IsNotExist(err) {
		t.Error("a successful run must not leave a black box")
	}

	// A failing command keeps its black box, scrubbed like the journal: a
	// stale index.lock makes `git add` fail, which ship-it does not expect.
	s.write("main.go", "package main\n// more confidential content\n")
	if err := os.WriteFile(filepath.Join(gitDir, "index.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr, failed := s.journaled(env, "--yes", "--local")
	os.Remove(filepath.Join(gitDir, "index.lock"))
	if code != cli.ExitError {
		t.Fatalf("ship with a locked index: exit %d\n%s", code, stderr)
	}
	boxPath := filepath.Join(logs, string(failed.RunID)+obs.BlackBoxSuffix)
	box, err := os.ReadFile(boxPath)
	if err != nil {
		t.Fatalf("no black box after a failure: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, string(failed.RunID)+obs.BlackBoxSuffix) {
		t.Errorf("the error must point to the black box:\n%s", stderr)
	}
	if !strings.Contains(string(box), "git.exec") {
		t.Errorf("the black box misses the git commands:\n%s", box)
	}

	// Privacy: no absolute path, e-mail, credential or file content anywhere.
	all := string(data) + string(box)
	for _, leak := range []string{s.dir, filepath.ToSlash(s.dir), env.Home, "test@example.invalid", "hunter2hunter2", "confidential content"} {
		if strings.Contains(all, leak) {
			t.Errorf("%q leaked into the logs", leak)
		}
	}

	// ship trace reads the failed run back from its black box.
	code, stdout, _, _ = s.journaled(env, "trace", string(failed.RunID))
	if code != cli.ExitOK || !strings.Contains(stdout, "git") {
		t.Fatalf("ship trace: exit %d\n%s", code, stdout)
	}
}

// TestJournalOutsideARepository goes to the user log directory.
func TestJournalOutsideARepository(t *testing.T) {
	env := testEnvironment(t)
	j := openJournal(context.Background(), gitexec.New(t.TempDir()), env)
	defer j.Close()
	want, err := env.LogDir()
	if err != nil {
		t.Fatal(err)
	}
	if j.Dir() != want {
		t.Fatalf("journal in %q, want %q", j.Dir(), want)
	}
	j.Logger.Info("run.start")
	if _, err := os.Stat(filepath.Join(want, string(j.RunID)+obs.JournalSuffix)); err != nil {
		t.Fatal(err)
	}
}
