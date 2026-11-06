// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"errors"
	"time"
)

// CommandResult is how a project command ended.
type CommandResult struct {
	ExitCode int
	// Output is the end of stdout and stderr, interleaved: where errors are.
	Output   []byte
	Duration time.Duration
}

// CommandRunner runs commands of the project (gates, commitlint) at the root
// of the working tree, never through a shell.
type CommandRunner interface {
	// Run runs command with no standard input. It returns an error only
	// when the command could not run at all; a failing command is a
	// non-zero ExitCode.
	Run(ctx context.Context, command []string) (CommandResult, error)
	// RunWithInput is Run with stdin as standard input (a commit message
	// for commitlint).
	RunWithInput(ctx context.Context, command []string, stdin []byte) (CommandResult, error)
}

// ErrCommandNotFound is returned by Run when the program is not installed.
var ErrCommandNotFound = errors.New("ports: program not found")
