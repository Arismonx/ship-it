// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets this test binary stand in for git: with GITEXEC_FAKE set, it
// fails the first FAKE_FAILURES calls with the given stderr, then succeeds.
func TestMain(m *testing.M) {
	if counter := os.Getenv("GITEXEC_FAKE_COUNTER"); counter != "" {
		data, _ := os.ReadFile(counter)
		calls := len(data)
		_ = os.WriteFile(counter, append(data, '.'), 0o644)
		failures := 0
		fmt.Sscan(os.Getenv("GITEXEC_FAKE_FAILURES"), &failures)
		if calls < failures {
			fmt.Fprintln(os.Stderr, os.Getenv("GITEXEC_FAKE_STDERR"))
			os.Exit(128)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeGit(t *testing.T, failures int, stderr string) (*Client, func() int) {
	t.Helper()
	counter := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GITEXEC_FAKE_COUNTER", counter)
	t.Setenv("GITEXEC_FAKE_FAILURES", fmt.Sprint(failures))
	t.Setenv("GITEXEC_FAKE_STDERR", stderr)
	saved := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = saved })
	calls := func() int {
		data, _ := os.ReadFile(counter)
		return len(data)
	}
	return &Client{Dir: t.TempDir(), Binary: os.Args[0]}, calls
}

func TestFetchRetriesADroppedConnection(t *testing.T) {
	c, calls := fakeGit(t, 2, "fatal: Could not read from remote repository.")
	if err := c.Fetch(context.Background(), "origin"); err != nil || calls() != 3 {
		t.Fatalf("Fetch = %v after %d calls", err, calls())
	}
}

func TestPushRetriesOnlyWhenNothingWasSent(t *testing.T) {
	c, calls := fakeGit(t, 1, "fatal: The remote end hung up unexpectedly")
	if err := c.Push(context.Background(), "origin", "feat/x", ""); err == nil || calls() != 1 {
		t.Fatalf("a push that may have sent data is not retried: %v after %d calls", err, calls())
	}
	c, calls = fakeGit(t, 1, "fatal: Could not read from remote repository.")
	if err := c.Push(context.Background(), "origin", "feat/x", ""); err != nil || calls() != 2 {
		t.Fatalf("Push = %v after %d calls", err, calls())
	}
}

func TestOtherFailuresAreNotRetried(t *testing.T) {
	c, calls := fakeGit(t, 5, "fatal: couldn't find remote ref main")
	err := c.Fetch(context.Background(), "origin")
	if err == nil || calls() != 1 || !strings.Contains(err.Error(), "couldn't find remote ref") {
		t.Fatalf("Fetch = %v after %d calls", err, calls())
	}
}
