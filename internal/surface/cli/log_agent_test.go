// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// TestLogAgent: #49. ship log --agent keeps what agents did, --agent claude
// (or --agent=claude) what one agent did, and the plain log names the agent.
func TestLogAgent(t *testing.T) {
	ops := &testkit.FakeOperationLog{Operations: []oplog.Operation{
		{ID: "01AAAAAAAAAAAAAAAAAAAAAAA3", Kind: oplog.Deliver, Title: "✨ feat: by codex", Agent: "codex"},
		{ID: "01AAAAAAAAAAAAAAAAAAAAAAA2", Kind: oplog.Deliver, Title: "✨ feat: by claude", Agent: "claude"},
		{ID: "01AAAAAAAAAAAAAAAAAAAAAAA1", Kind: oplog.Deliver, Title: "✨ feat: by a person"},
	}}
	run := func(args ...string) (int, string) {
		var out, errOut bytes.Buffer
		deps := Deps{Git: testkit.HealthyRepository("main"), Operations: ops, OperatingSystem: "linux"}
		code := Run(context.Background(), append([]string{"log"}, args...), Streams{Out: &out, Err: &errOut}, i18n.English, testBuild, deps)
		return code, out.String()
	}
	for _, tt := range []struct {
		args       []string
		want, skip []string
	}{
		{nil, []string{"by codex on   (agent: codex)", "by claude", "by a person"}, nil},
		{[]string{"--agent"}, []string{"by codex", "by claude"}, []string{"by a person"}},
		{[]string{"--agent", "claude"}, []string{"by claude"}, []string{"by codex", "by a person"}},
		{[]string{"--agent=Codex"}, []string{"by codex"}, []string{"by claude", "by a person"}},
	} {
		code, out := run(tt.args...)
		if code != ExitOK {
			t.Fatalf("%q: exit %d", tt.args, code)
		}
		for _, w := range tt.want {
			if !strings.Contains(out, w) {
				t.Errorf("%q: missing %q\n%s", tt.args, w, out)
			}
		}
		for _, s := range tt.skip {
			if strings.Contains(out, s) {
				t.Errorf("%q: should not show %q\n%s", tt.args, s, out)
			}
		}
	}
	if code, _ := run("--agent", "claude", "extra"); code != ExitUsage {
		t.Errorf("two names: exit %d, want %d", code, ExitUsage)
	}
}
