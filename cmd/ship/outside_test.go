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

	"github.com/GregoireF/ship-it/internal/adapters/checkpointstore"
	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
	"github.com/GregoireF/ship-it/internal/adapters/oplogstore"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestOutsideARepository runs the real git in a folder that is no
// repository: an expected situation, said in a sentence, with no black box
// and no invitation to report a bug.
func TestOutsideARepository(t *testing.T) {
	dir := t.TempDir()
	// git must not find a repository in a parent folder either.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	git := gitexec.New(dir)
	if _, err := git.Version(context.Background()); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	deps := cli.Deps{Git: git, Checkpoints: checkpointstore.New(git), Operations: oplogstore.New(git), OperatingSystem: "linux"}
	for _, args := range [][]string{{}, {"--yes"}, {"plan"}, {"status"}, {"stack"}, {"log"}} {
		logs := t.TempDir()
		journal := obs.NewJournal(obs.JournalOptions{})
		if err := journal.Attach(logs); err != nil {
			t.Fatal(err)
		}
		var out, errOut bytes.Buffer
		code := cli.Run(context.Background(), args, cli.Streams{Out: &out, Err: &errOut, Journal: journal}, i18n.English, cli.BuildInfo{}, deps)
		journal.Close()
		stderr := errOut.String()
		if code != cli.ExitError || !strings.Contains(stderr, "SHIP-E0316  Not inside a git repository: cd into your project, or run git init.") ||
			strings.Contains(stderr, "exit 128") || strings.Contains(stderr, "bug") {
			t.Errorf("ship %q: exit %d\n%s", args, code, stderr)
		}
		boxes, _ := filepath.Glob(filepath.Join(logs, "*"+obs.BlackBoxSuffix))
		if len(boxes) > 0 {
			t.Errorf("ship %q left a black box: %v", args, boxes)
		}
	}

	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), []string{"doctor"}, cli.Streams{Out: &out, Err: &errOut}, i18n.English, cli.BuildInfo{}, deps)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "Not ready") || code != cli.ExitError {
		t.Errorf("ship doctor outside a repository: exit %d, ends with %q:\n%s", code, last, out.String())
	}
	if strings.Contains(out.String(), "soon") {
		t.Errorf("ship doctor promises a future:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Error("ship created a repository")
	}
}
