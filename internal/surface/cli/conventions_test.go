// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// runConfigured executes the CLI in a repository whose configuration file
// holds repository.
func runConfigured(repository string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	deps := Deps{Git: testkit.HealthyRepository("main"), Config: &testkit.FakeConfigFiles{Repository: repository}, OperatingSystem: "linux"}
	code = Run(context.Background(), args, Streams{In: strings.NewReader(""), Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
	return code, out.String(), errOut.String()
}

const depsType = `{"ship": {"commits": {"types": {"deps": {"emoji": "⬆️", "branchPrefix": "deps"}, "wip": false}, "scopeRequired": true},
	"branches": {"issue": "none"}}}`

func TestCommandsFollowTheRepositoryConventions(t *testing.T) {
	if code, _, stderr := runConfigured(depsType, "check", "title", "⬆️ deps(yaml): bump parser"); code != ExitOK {
		t.Errorf("check title: exit %d, %s", code, stderr)
	}
	if code, _, stderr := runConfigured(depsType, "check", "title", "⬆️ deps: bump parser"); code != ExitError || !strings.Contains(stderr, "SHIP-E0118") {
		t.Errorf("a scope is required: exit %d, %s", code, stderr)
	}
	if code, _, _ := runConfigured(depsType, "check", "branch", "deps/bump-parser"); code != ExitOK {
		t.Errorf("check branch: exit %d", code)
	}
	code, stdout, _ := runConfigured(depsType, "types")
	if code != ExitOK || !strings.Contains(stdout, "⬆️ deps") || !strings.Contains(stdout, "Type of this repository") || strings.Contains(stdout, " wip ") {
		t.Errorf("types: exit %d\n%s", code, stdout)
	}
	if code, stdout, _ := runConfigured(depsType, "branch", "--issue", "4", "deps", "bump the parser"); code != ExitOK || strings.TrimSpace(stdout) != "deps/bump-parser" {
		t.Errorf("branch = %q (exit %d)", stdout, code)
	}
	if code, _, stderr := runConfigured(`{"ship": {"commits": {"bodyWidth": 10}}}`, "check", "title", "✨ feat: x"); code == ExitOK || !strings.Contains(stderr, "SHIP-E0704") {
		t.Errorf("a broken file is reported: exit %d, %s", code, stderr)
	}
}
