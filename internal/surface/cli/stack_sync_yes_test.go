// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strings"
	"testing"
)

// TestStackSyncAcceptsYes guards issue #57: `ship stack sync` must accept
// --yes/-y like the other writing commands, so scripts and agents can pass it
// uniformly. Without the flag, parsing fails with ExitUsage before the
// command runs.
func TestStackSyncAcceptsYes(t *testing.T) {
	code, stdout, stderr := run("stack", "sync", "--help")
	if code != ExitOK || stderr != "" {
		t.Fatalf("stack sync --help: exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "--yes") {
		t.Fatalf("stack sync --help missing --yes:\n%s", stdout)
	}
	if code, _, stderr := run("stack", "sync", "--yes"); code == ExitUsage {
		t.Fatalf("stack sync --yes rejected as usage: %s", stderr)
	}
}
