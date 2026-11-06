// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const (
	printConfig = "npx --no -- commitlint --print-config json"
	lint        = "npx --no -- commitlint"
)

// conventionalRules is the shape of @commitlint/config-conventional's rules.
const conventionalRules = `{"rules":{"header-max-length":[2,"always",100],"subject-full-stop":[2,"never","."],
"subject-case":[2,"never",["sentence-case","start-case","pascal-case","upper-case"]],
"type-enum":[2,"always",["build","chore","ci","docs","feat","fix","perf","refactor","revert","style","test"]]}}`

// fakeCommitlint answers like commitlint with the conventional parser: a
// header that does not start with a type (a leading gitmoji) is refused.
func fakeCommitlint(config string) *testkit.FakeRunner {
	return &testkit.FakeRunner{Respond: func(line string, stdin []byte) (ports.CommandResult, bool) {
		switch line {
		case printConfig:
			return ports.CommandResult{Output: []byte(config)}, true
		case lint:
			if r, _ := utf8.DecodeRune(stdin); r >= utf8.RuneSelf {
				return ports.CommandResult{ExitCode: 1, Output: []byte("⧗   input: " + string(stdin) + "✖   type may not be empty [type-empty]\n")}, true
			}
			return ports.CommandResult{}, true
		}
		return ports.CommandResult{}, false
	}}
}

// withCommitlint installs commitlint in the fake project.
func withCommitlint(git testkit.FakeGit) testkit.FakeGit {
	files := map[string]string{"commitlint.config.js": "module.exports = {}\n", "node_modules/.bin/commitlint": ""}
	for path, content := range git.WorkingFiles {
		files[path] = content
	}
	git.WorkingFiles = files
	return git
}

func TestDeliverFollowsTheProjectsCommitlint(t *testing.T) {
	var calls []string
	git := withCommitlint(deliverableGit("fix/crash", &calls))
	ws, _, _ := workspaceOf(git)
	runner := fakeCommitlint(conventionalRules)
	ws.Runner = runner
	result, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🔒️ security: rotate keys", Branch: "fix/rotate-keys"})
	if err != nil {
		t.Fatal(err)
	}
	// security is not allowed, fix means the same; the gitmoji goes, since
	// commitlint rejects it first: rule zero beats ship-it's format.
	if result.Title != "fix: rotate keys" || !slices.Contains(calls, "commit fix: rotate keys\n") {
		t.Fatalf("title %q, calls %q", result.Title, calls)
	}
	if !slices.Equal(runner.Inputs, []string{"🔒️ fix: rotate keys\n", "fix: rotate keys\n"}) {
		t.Fatalf("the exact message is validated: %q", runner.Inputs)
	}
}

func TestCommitlintSeesTheWholeMessage(t *testing.T) {
	var calls []string
	git := withCommitlint(deliverableGit("fix/crash", &calls))
	ws, _, _ := workspaceOf(git)
	runner := fakeCommitlint(`{"rules":{}}`)
	ws.Runner = runner
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "fix: handle nil", Branch: "fix/x",
		Why: "The config may be absent.", Tested: "go test ./..."})
	if err != nil {
		t.Fatal(err)
	}
	last := runner.Inputs[len(runner.Inputs)-1]
	if !strings.Contains(last, "The config may be absent.") || !slices.Contains(calls, "commit "+last) {
		t.Fatalf("commitlint must check what is committed: %q, calls %q", runner.Inputs, calls)
	}
}

func TestDeliverStopsWhenCommitlintRefuses(t *testing.T) {
	var calls []string
	git := withCommitlint(deliverableGit("fix/crash", &calls))
	ws, store, _ := workspaceOf(git)
	runner := fakeCommitlint(`{"rules":{}}`)
	runner.Respond = func(line string, _ []byte) (ports.CommandResult, bool) {
		if line == lint {
			return ports.CommandResult{ExitCode: 1, Output: []byte("✖   subject must not be shouted [x]\n")}, true
		}
		return ports.CommandResult{Output: []byte(`{"rules":{}}`)}, true
	}
	ws.Runner = runner
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🐛 fix: handle nil", Branch: "fix/x"})
	output, ok := CommitlintOutputIn(err)
	if !ok || !strings.Contains(output, "shouted") || !errors.Is(err, &fault.Fault{Code: fault.CommitlintRefused}) || !IsGateRefusal(err) {
		t.Fatalf("err = %v", err)
	}
	if len(calls) != 0 || len(store.Checkpoints) != 0 {
		t.Fatalf("nothing changes: %q %+v", calls, store.Checkpoints)
	}
}

func TestCommitlintRulesBeatShipItsStyle(t *testing.T) {
	var calls []string
	git := withCommitlint(deliverableGit("fix/crash", &calls))
	ws, _, _ := workspaceOf(git)
	ws.Runner = fakeCommitlint(`{"rules":{"subject-case":[2,"always","sentence-case"]}}`)
	// ship-it alone refuses a capitalised subject; this project wants it.
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "fix: Handle nil", Branch: "fix/x"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "commit fix: Handle nil\n") {
		t.Fatalf("calls %q", calls)
	}
}

// An explicitly set length applies where the project's commitlint has no
// rule for it; commitlint's own rule wins, and a default gives way as before.
func TestExplicitLengthsApplyWhereCommitlintIsSilent(t *testing.T) {
	const long = "fix: handle a missing configuration file at startup" // subject of 46
	tests := []struct {
		name, settings, rules string
		refused               fault.Code
	}{
		{"explicit, no rule", `{"ship": {"commits": {"subjectMaxLength": 30}}}`, `{"rules":{}}`, fault.SubjectTooLong},
		{"explicit header, no rule", `{"ship": {"commits": {"headerMaxLength": 40}}}`, `{"rules":{}}`, fault.HeaderTooLong},
		{"explicit, commitlint rules it", `{"ship": {"commits": {"subjectMaxLength": 30}}}`,
			`{"rules":{"subject-max-length":[2,"always",100]}}`, ""},
		{"explicit, a warning only", `{"ship": {"commits": {"subjectMaxLength": 30}}}`,
			`{"rules":{"subject-max-length":[1,"always",100]}}`, fault.SubjectTooLong},
		{"explicit, within the limit", `{"ship": {"commits": {"subjectMaxLength": 50}}}`, `{"rules":{}}`, ""},
		{"no setting", "", `{"rules":{}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			git := withCommitlint(deliverableGit("fix/crash", nil))
			ws, _, _ := workspaceOf(git)
			ws.Config = &testkit.FakeConfigFiles{Repository: tt.settings}
			ws.Runner = fakeCommitlint(tt.rules)
			_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: long, Branch: "fix/x"})
			if tt.refused == "" && err != nil {
				t.Fatalf("delivered: %v", err)
			}
			if tt.refused != "" && !errors.Is(err, &fault.Fault{Code: tt.refused}) {
				t.Fatalf("err = %v, want %s", err, tt.refused)
			}
		})
	}
	// subjectMaxLength 50 set by a file is explicit, though equal to the
	// default: a longer subject is refused.
	git := withCommitlint(deliverableGit("fix/crash", nil))
	ws, _, _ := workspaceOf(git)
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"commits": {"subjectMaxLength": 50}}}`}
	ws.Runner = fakeCommitlint(`{"rules":{}}`)
	title := "fix: handle a missing configuration file at startup time now" // subject of 55
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: title, Branch: "fix/x"}); !errors.Is(err, &fault.Fault{Code: fault.SubjectTooLong}) {
		t.Fatalf("explicit 50: %v", err)
	}
}

// The body is wrapped within commitlint's body-max-line-length.
func TestBodyWidthWithCommitlint(t *testing.T) {
	why := strings.Repeat("the configuration may be absent on a fresh clone ", 4)
	for _, tt := range []struct {
		name, settings, rules string
		width                 int
	}{
		{"commitlint narrower", "", `{"rules":{"body-max-line-length":[2,"always",50]}}`, 50},
		{"ship-it narrower", `{"ship": {"commits": {"bodyWidth": 45}}}`, `{"rules":{"body-max-line-length":[2,"always",100]}}`, 45},
		{"explicit, no rule", `{"ship": {"commits": {"bodyWidth": 90}}}`, `{"rules":{}}`, 90},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			git := withCommitlint(deliverableGit("fix/crash", &calls))
			ws, _, _ := workspaceOf(git)
			ws.Config = &testkit.FakeConfigFiles{Repository: tt.settings}
			ws.Runner = fakeCommitlint(tt.rules)
			if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "fix: handle nil", Branch: "fix/x", Why: why}); err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(calls, func(c string) bool { return strings.HasPrefix(c, "commit ") })
			if i < 0 {
				t.Fatalf("calls %q", calls)
			}
			longest := 0
			for _, line := range strings.Split(calls[i], "\n")[1:] {
				longest = max(longest, utf8.RuneCountInString(line))
			}
			if longest > tt.width || longest < tt.width-12 {
				t.Fatalf("longest body line %d, want about %d:\n%s", longest, tt.width, calls[i])
			}
		})
	}
}

func TestCommitlintUnsatisfiableIsAGateRefusal(t *testing.T) {
	git := withCommitlint(deliverableGit("fix/crash", nil))
	ws, _, _ := workspaceOf(git)
	ws.Runner = fakeCommitlint(`{"rules":{"type-enum":[2,"always",["feat","fix","refactor"]]}}`)
	_, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "⚡️ perf: cache rates", Branch: "chore/x"})
	var f *fault.Fault
	if !errors.As(err, &f) || f.Key != "deliver.commitlint.type_not_allowed" || !IsGateRefusal(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithoutCommitlintNothingRuns(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"no configuration": {"node_modules/.bin/commitlint": ""},
		"not installed":    {".commitlintrc.json": "{}"},
	} {
		t.Run(name, func(t *testing.T) {
			git := deliverableGit("fix/crash", nil)
			git.WorkingFiles = files
			ws, _, _ := workspaceOf(git)
			runner := &testkit.FakeRunner{}
			ws.Runner = runner
			if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🐛 fix: handle nil", Branch: "fix/x"}); err != nil {
				t.Fatal(err)
			}
			if len(runner.Calls) != 0 {
				t.Fatalf("no command may run: %q", runner.Calls)
			}
		})
	}
}

func TestPackageJSONFieldDeclaresCommitlint(t *testing.T) {
	git := deliverableGit("fix/crash", nil)
	git.WorkingFiles = map[string]string{"package.json": `{"commitlint":{"extends":["@commitlint/config-conventional"]}}`, "node_modules/.bin/commitlint.cmd": ""}
	ws, _, _ := workspaceOf(git)
	runner := fakeCommitlint(conventionalRules)
	ws.Runner = runner
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🐛 fix: handle nil", Branch: "fix/x"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(runner.Calls, printConfig) {
		t.Fatalf("calls %q", runner.Calls)
	}
}

func TestUnreadableCommitlintConfigurationIsIgnored(t *testing.T) {
	git := withCommitlint(deliverableGit("fix/crash", nil))
	ws, _, _ := workspaceOf(git)
	ws.Runner = &testkit.FakeRunner{Failing: map[string]string{printConfig: "Error: Cannot find module"}}
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "🐛 fix: handle nil", Branch: "fix/x"}); err != nil {
		t.Fatal(err)
	}
}

// TestPlanAsksForTheTypeWhenSeveralFit: the project refuses "perf" and allows
// both "refactor" and "fix"; the plan offers them, the human picks at review.
// Delivered as is (--yes, an agent), the title is refused as before.
func TestPlanAsksForTheTypeWhenSeveralFit(t *testing.T) {
	git := withCommitlint(testkit.FakeGit{})
	runner := fakeCommitlint(`{"rules":{"type-enum":[2,"always",["feat","fix","refactor","chore"]]}}`)
	ws := Workspace{Git: git, Runner: runner}
	view, err := planWithCommitlint(context.Background(), ws, convention.Rules{}, convention.GitmojiFirst, PlanView{Title: "⚡️ perf: cache rates"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(view.TypeChoices, []string{"refactor", "fix"}) || view.Title != "⚡️ perf: cache rates" {
		t.Fatalf("view = %+v", view)
	}
	last := view.Reasons[len(view.Reasons)-1]
	if last.Key != "plan.reason.commitlint.type_choice" || last.Params["choices"] != "refactor, fix" || last.Params["type"] != "perf" {
		t.Fatalf("reason = %+v", last)
	}
	for _, line := range runner.Calls {
		if line == lint {
			t.Fatal("nothing is linted before the human picks")
		}
	}

	deliver := withCommitlint(deliverableGit("chore/cache", nil))
	dws, _, _ := workspaceOf(deliver)
	dws.Runner = runner
	_, err = DeliverLocally(context.Background(), dws, DeliveryRequest{Title: view.Title, Branch: "chore/x"})
	if !errors.Is(err, &fault.Fault{Code: fault.CommitlintUnsatisfiable}) || !IsGateRefusal(err) {
		t.Fatalf("without a pick: %v", err)
	}
	picked, ok := RetypeTitle(view.Title, view.TypeChoices[0])
	if !ok || picked != "⚡️ refactor: cache rates" {
		t.Fatalf("RetypeTitle = %q, %v", picked, ok)
	}
	if _, err := DeliverLocally(context.Background(), dws, DeliveryRequest{Title: picked, Branch: "chore/x"}); err != nil {
		t.Fatalf("after the pick: %v", err)
	}
}

func TestRetypeTitle(t *testing.T) {
	for title, want := range map[string]string{
		"⚡️ perf(cache): speed up perf: twice": "⚡️ refactor(cache): speed up perf: twice",
		"perf!: drop the old cache":            "refactor!: drop the old cache",
		"perf: ⚡️ cache":                       "refactor: ⚡️ cache",
	} {
		if got, ok := RetypeTitle(title, "refactor"); !ok || got != want {
			t.Errorf("RetypeTitle(%q) = %q, %v; want %q", title, got, ok, want)
		}
	}
	if _, ok := RetypeTitle("not a title", "fix"); ok {
		t.Error("an unparsable title")
	}
	if _, ok := RetypeTitle("perf: x", "feature"); ok {
		t.Error("a type ship-it does not know")
	}
}

func TestPlanSaysItFollowsCommitlint(t *testing.T) {
	git := withCommitlint(testkit.FakeGit{
		WorkingTree: ports.WorkingTreeStatus{
			Branch: ports.BranchState{Name: "fix/crash", Commit: "abc"},
			Files:  []ports.FileChange{{Path: "a.go", State: ports.Untracked}},
		},
		WorkingFiles: map[string]string{"a.go": "package a\n"},
	})
	view, err := ProposePlan(context.Background(), Workspace{Git: git, Runner: fakeCommitlint(conventionalRules)}, PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := utf8.DecodeRuneInString(view.Title); r >= utf8.RuneSelf {
		t.Fatalf("commitlint refuses a leading gitmoji: %q", view.Title)
	}
	var keys []string
	for _, r := range view.Reasons {
		keys = append(keys, r.Key)
	}
	if !slices.Contains(keys, "plan.reason.commitlint.gitmoji") || !slices.Contains(keys, "plan.reason.commitlint") {
		t.Fatalf("reasons = %q", keys)
	}
}
