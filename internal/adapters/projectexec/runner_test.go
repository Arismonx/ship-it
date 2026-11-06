// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package projectexec

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

type dir string

func (d dir) TopLevel(context.Context) (string, error) { return string(d), nil }

// git is on every machine that runs ship-it: it stands in for a project tool.
func TestRunReportsExitCodeAndOutput(t *testing.T) {
	r := New(dir(t.TempDir()))
	ctx := context.Background()
	ok, err := r.Run(ctx, []string{"git", "--version"})
	if err != nil || ok.ExitCode != 0 || !strings.HasPrefix(string(ok.Output), "git version") {
		t.Fatalf("Run = %+v, %v", ok, err)
	}
	failed, err := r.Run(ctx, []string{"git", "no-such-subcommand"})
	if err != nil || failed.ExitCode == 0 || !strings.Contains(string(failed.Output), "no-such-subcommand") {
		t.Fatalf("failing command = %+v, %v", failed, err)
	}
}

// git hash-object --stdin proves the input reached the program: the blob id
// of "hello\n" is well known.
func TestRunWithInput(t *testing.T) {
	r := New(dir(t.TempDir()))
	got, err := r.RunWithInput(context.Background(), []string{"git", "hash-object", "--stdin"}, []byte("hello\n"))
	if err != nil || got.ExitCode != 0 || strings.TrimSpace(string(got.Output)) != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Fatalf("RunWithInput = %+v (%s), %v", got, got.Output, err)
	}
}

func TestMissingProgram(t *testing.T) {
	_, err := New(dir(t.TempDir())).Run(context.Background(), []string{"ship-it-no-such-program-1f3a"})
	if !errors.Is(err, ports.ErrCommandNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	tail := &tailBuffer{max: 8}
	tail.Write([]byte("0123456789"))
	tail.Write([]byte("ab"))
	if got := tail.bytes(); !bytes.Equal(got, []byte("456789ab")) {
		t.Fatalf("tail = %q", got)
	}
}

// A gate stopped by its deadline reports the deadline, on every OS (Windows
// kills with exit code 1, which must not read as a red gate).
func TestATimedOutGateIsNotARedGate(t *testing.T) {
	r := New(dir(t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Run(ctx, []string{"git", "--version"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the context's error", err)
	}
}
