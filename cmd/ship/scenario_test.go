// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/internal/adapters/checkpointstore"
	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
	"github.com/GregoireF/ship-it/internal/adapters/oplogstore"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// scenario is a real repository driven through the CLI with the real
// adapters; only GitHub's API is replaced.
type scenario struct {
	t    *testing.T
	dir  string
	git  *gitexec.Client
	deps cli.Deps
}

func newScenario(t *testing.T, forge ports.Forge) *scenario {
	t.Helper()
	dir := t.TempDir()
	git := gitexec.New(dir)
	if _, err := git.Run(context.Background(), "init", "--quiet", "--initial-branch=main"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	s := &scenario{t: t, dir: dir, git: git, deps: cli.Deps{
		Git: git, Checkpoints: checkpointstore.New(git), Operations: oplogstore.New(git), Forge: forge, OperatingSystem: "linux",
	}}
	s.sh("config", "user.name", "ship-it test")
	s.sh("config", "user.email", "test@example.invalid")
	s.sh("config", "commit.gpgsign", "false")
	return s
}

// sh runs git in the repository and returns its trimmed output.
func (s *scenario) sh(args ...string) string {
	s.t.Helper()
	out, err := s.git.Run(context.Background(), args...)
	// Under heavy parallel load, git for Windows sometimes fails to start
	// the local transport ("Could not read from remote repository"): the
	// harness retries like ship-it does (gitexec.Client.retry).
	for attempt := 0; err != nil && attempt < 2 && strings.Contains(err.Error(), "Could not read from remote repository"); attempt++ {
		time.Sleep(500 * time.Millisecond)
		out, err = s.git.Run(context.Background(), args...)
	}
	if err != nil {
		s.t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func (s *scenario) write(name, content string) {
	s.t.Helper()
	if err := os.WriteFile(filepath.Join(s.dir, name), []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// ship runs the CLI and returns its exit code, stdout and stderr.
func (s *scenario) ship(args ...string) (int, string, string) {
	return s.shipWith(cli.BuildInfo{}, args...)
}

// shipWith runs the CLI with a build identity, for the version command.
func (s *scenario) shipWith(build cli.BuildInfo, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Streams{Out: &out, Err: &errOut}, i18n.English, build, s.deps)
	return code, out.String(), errOut.String()
}
