// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strings"
	"testing"
)

// TestStackPublishAcceptYes: issue #109: ship stack publish accepts --yes
// Check that both -y and --yes flags are recognized and shown in help output
// if the ExitUsage is a usage error it means the program rejected the flag.

func TestStackPublishAcceptYes(t *testing.T) {
	for _, sub := range []string{"-y", "--yes"} {
		code, stdout, stderr := run("stack", "publish", sub, "--help")
		if code != ExitOK || stderr != "" {
			t.Errorf("stack publish %s failed: exit %d\n%s", sub, code, stderr)
		}

		if !strings.Contains(stdout, "\n  "+sub+" ") {
			t.Errorf("stack publish --help misses %s:\n%s", sub, stdout)
		}

		if code, _, stderr := run("stack", "publish", sub); code == ExitUsage {
			t.Errorf("stack publish %s rejected as usage: %s", sub, stderr)
		}
	}
}
