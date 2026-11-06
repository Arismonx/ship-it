// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package projectexec implements ports.CommandRunner: it runs the project's
// own commands (lint, tests) for the gates. Arguments go to exec.Command as
// they are, never through a shell (AGENTS.md).
package projectexec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GregoireF/ship-it/internal/ports"
)

// waitDelay is how long Run waits for the output pipes once the gate has
// exited or been killed.
const waitDelay = 5 * time.Second

// TailSize is how much of the output is kept: enough for the failing test
// and its stack, little enough to print.
const TailSize = 8 << 10

// TopLevelLocator returns the root of the working tree.
type TopLevelLocator interface {
	TopLevel(ctx context.Context) (string, error)
}

// Runner implements ports.CommandRunner.
type Runner struct {
	Git TopLevelLocator
}

var _ ports.CommandRunner = (*Runner)(nil)

// New returns a Runner working at the root of git's working tree.
func New(git TopLevelLocator) *Runner { return &Runner{Git: git} }

// Run implements ports.CommandRunner.
func (r *Runner) Run(ctx context.Context, command []string) (ports.CommandResult, error) {
	return r.run(ctx, command, nil)
}

// RunWithInput implements ports.CommandRunner.
func (r *Runner) RunWithInput(ctx context.Context, command []string, stdin []byte) (ports.CommandResult, error) {
	return r.run(ctx, command, bytes.NewReader(stdin))
}

func (r *Runner) run(ctx context.Context, command []string, stdin io.Reader) (ports.CommandResult, error) {
	if len(command) == 0 {
		return ports.CommandResult{}, errors.New("projectexec: empty command")
	}
	dir, err := r.Git.TopLevel(ctx)
	if err != nil {
		return ports.CommandResult{}, err
	}
	name := command[0]
	if strings.HasPrefix(name, "./") {
		// A wrapper of the project (./gradlew, ./mvnw) is relative to the
		// repository root, not to where ship was started; an absolute path
		// also avoids LookPath's refusal of relative results (exec.ErrDot).
		name = filepath.Join(dir, filepath.FromSlash(name))
	}
	program, err := exec.LookPath(name)
	if err != nil {
		return ports.CommandResult{}, errors.Join(ports.ErrCommandNotFound, err)
	}
	cmd := exec.CommandContext(ctx, program, command[1:]...)
	cmd.Dir = dir
	// Without stdin, test runners such as vitest run once instead of watching.
	cmd.Stdin = stdin
	tail := &tailBuffer{max: TailSize}
	cmd.Stdout, cmd.Stderr = tail, tail
	// Killing the gate (timeout, interrupt) stops its direct process only;
	// children such as node under pnpm may keep the output pipe open, and
	// Wait would block on it forever. WaitDelay closes it after a grace.
	cmd.WaitDelay = waitDelay

	start := time.Now()
	err = cmd.Run()
	result := ports.CommandResult{Output: tail.bytes(), Duration: time.Since(start)}
	if ctx.Err() != nil {
		// Timed out or interrupted: on Windows the killed process exits 1,
		// which must not read as a red gate.
		return result, ctx.Err()
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	case errors.Is(err, exec.ErrWaitDelay):
		return result, nil // the gate ended; a child it left behind kept the pipe
	case err != nil:
		return result, err
	}
	return result, nil
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu   sync.Mutex
	max  int
	data []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if len(t.data) > t.max {
		t.data = append(t.data[:0], t.data[len(t.data)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]byte(nil), t.data...)
}
