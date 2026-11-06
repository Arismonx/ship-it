// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// benchRepository is the repository of the end-to-end benchmarks (cmd/ship):
// 2,000 files, 35 of them changed or new.
func benchRepository(b *testing.B) *gitexec.Client {
	b.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git unavailable")
	}
	b.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(b.TempDir(), "gitconfig"))
	dir := filepath.Join(b.TempDir(), "widget")
	if err := benchdata.NewRepository(dir, benchdata.RepoOptions{Files: 2000, Commits: 300, Modified: 30, Untracked: 5}); err != nil {
		b.Fatal(err)
	}
	readAll(b, dir)
	return gitexec.New(dir)
}

// readAll reads every file of the working tree once. The repository is
// generated a moment before the timing starts, and on Windows the first
// reading of a newly written file by the test binary is slow (measured: 9 s
// for 1,847 files, 75 ms the second time; most likely the antivirus scanning
// each file). A real repository's files were not written a second ago:
// without this, BenchmarkCreateCold measures that scan, not the checkpoint.
func readAll(b *testing.B, dir string) {
	b.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case d.Type().IsRegular():
			_, err = os.ReadFile(path)
		}
		return err
	})
	if err != nil {
		b.Fatal(err)
	}
}

// benchCreate takes checkpoints; cold removes the store before each one
// (outside the timing), as before the first ship in a repository.
func benchCreate(b *testing.B, cold bool) {
	git := benchRepository(b)
	var journal bytes.Buffer
	git.Log = slog.New(slog.NewTextHandler(&journal, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()
	gitDir, err := git.GitDir(ctx)
	if err != nil {
		b.Fatal(err)
	}
	store := New(git)
	if !cold {
		if _, err := store.Create(ctx, "warm-up"); err != nil {
			b.Fatal(err)
		}
	}
	journal.Reset()
	runs := 0
	for b.Loop() {
		if cold {
			b.StopTimer()
			if err := os.RemoveAll(filepath.Join(gitDir, "ship")); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
		}
		if _, err := store.Create(ctx, "bench"); err != nil {
			b.Fatal(err)
		}
		runs++
	}
	b.ReportMetric(float64(bytes.Count(journal.Bytes(), []byte("msg=git.exec")))/float64(runs), "git/op")
}

// BenchmarkCreateCold is the first checkpoint: every file is written to the
// new store.
func BenchmarkCreateCold(b *testing.B) { benchCreate(b, true) }

// BenchmarkCreateWarm is any later one: only the 35 changes are hashed.
func BenchmarkCreateWarm(b *testing.B) { benchCreate(b, false) }
