// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/core/split"
	"github.com/GregoireF/ship-it/internal/i18n"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// Keys as Bubble Tea v2 decodes them.
var (
	keyEnter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyCtrlEnter = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	keyTab       = tea.KeyPressMsg{Code: tea.KeyTab}
	keyShiftTab  = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	keySpace     = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyEsc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyCtrlC     = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	keyCtrlL     = tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	keyCtrlU     = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	keyF2        = tea.KeyPressMsg{Code: tea.KeyF2}
	keyDown      = tea.KeyPressMsg{Code: tea.KeyDown}
	keyBackspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
	keyHelp      = tea.KeyPressMsg{Code: '?', Text: "?"}
)

func typed(s string) []tea.Msg {
	var msgs []tea.Msg
	for _, r := range s {
		msgs = append(msgs, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return msgs
}

var keyRight = tea.KeyPressMsg{Code: tea.KeyRight}

// perfPlan is a plan whose type the project's commitlint refuses, with two
// allowed types to pick from.
func perfPlan() app.PlanView {
	return app.PlanView{
		Title: "⚡️ perf(cart): cache vat rates", Branch: "chore/cache-vat-rates",
		CurrentBranch: "main", CreatesBranch: true,
		Files:       []string{"internal/cart/vat.go"},
		TypeChoices: []string{"refactor", "fix"},
	}
}

func TestReviewAsksForTheType(t *testing.T) {
	m := newTestReview(i18n.English, perfPlan(), &replanner{})
	if m.focus != fieldType {
		t.Fatalf("the type comes first: focus %v", m.focus)
	}
	// Shipping the refused type is blocked, and says why.
	m = send(t, m, keyEnter)
	if m.done || !strings.Contains(screen(m), "Pick a commit type") || m.focus != fieldType {
		t.Fatalf("refused type shipped:\n%s", screen(m))
	}
	m = send(t, m, keyRight, keySpace, keyEnter)
	if !m.done || m.outcome.action != reviewShip || m.outcome.title != "⚡️ fix(cart): cache vat rates" {
		t.Fatalf("outcome = %+v", m.outcome)
	}
	// The branch follows the picked type while untouched.
	if m.outcome.branch != "fix/cache-vat-rates" {
		t.Fatalf("branch = %q", m.outcome.branch)
	}
}

func vatPlan() app.PlanView {
	return app.PlanView{
		Title: "✨ feat(cart): add vat rate", Branch: "feat/add-vat-rate",
		CurrentBranch: "main", CreatesBranch: true,
		Files: []string{"internal/cart/vat.go", "internal/cart/vat_test.go", "docs/vat.md"},
	}
}

// replanner records the selections it was asked to plan.
type replanner struct {
	calls [][]string
	plan  func(only []string) app.PlanView
	err   error
}

func (r *replanner) replan(only []string) (app.PlanView, error) {
	r.calls = append(r.calls, only)
	if r.err != nil {
		return app.PlanView{}, r.err
	}
	return r.plan(only), nil
}

func newTestReview(lang i18n.Language, plan app.PlanView, rp *replanner) reviewModel {
	m := newReviewModel(reviewConfig{
		lang: lang, plan: plan,
		checkBranch: func(name string) app.BranchCheck { return app.CheckBranch(name) },
		branchFor: func(title string) (string, bool) {
			b, err := app.BranchForTitle(title, 0)
			return b, err == nil
		},
		replan: rp.replan,
	})
	m.delay = 0
	return m
}

// send runs msgs through Update, then the plan updates they start, as the
// program would (the tests set no debounce delay).
func send(t *testing.T, m reviewModel, msgs ...tea.Msg) reviewModel {
	t.Helper()
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(reviewModel)
		for cmd != nil {
			result := cmd()
			switch result.(type) {
			case replanTick, replanDone:
				next, cmd = m.Update(result)
				m = next.(reviewModel)
			default:
				cmd = nil // tea.Quit
			}
		}
	}
	return m
}

// screen is the rendered screen without styles or trailing blanks.
func screen(m reviewModel) string {
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "golden", "review", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (go test ./internal/surface/cli -run %s -update)", err, t.Name())
	}
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); w != got {
		t.Errorf("screen differs from %s:\n--- got\n%s\n--- want\n%s", path, got, w)
	}
}

func assertFits(t *testing.T, s string) {
	t.Helper()
	for _, l := range strings.Split(s, "\n") {
		if w := lipgloss.Width(l); w > reviewWidth {
			t.Errorf("line of %d columns (more than %d): %q", w, reviewWidth, l)
		}
	}
}

func TestReviewScreenGolden(t *testing.T) {
	big := vatPlan()
	big.Files = []string{"internal/cart/vat.go", "internal/cart/vat_test.go", "web/src/components/checkout/VatSummary.vue", ".env"}
	big.Secrets = []app.SecretView{{Path: ".env", Line: 3, Rule: "aws_key", Preview: "AKIA…MPLE", Length: 20}}
	big.Size = &app.SizeView{Lines: 912, Max: 400, Groups: []split.Group{
		{Path: "web/src/components/checkout/", Lines: 610, Files: []string{"web/src/components/checkout/VatSummary.vue"}},
		{Path: "internal/cart/", Lines: 302, Files: []string{"internal/cart/vat.go", "internal/cart/vat_test.go"}},
	}}
	onBranch := vatPlan()
	onBranch.CreatesBranch, onBranch.CurrentBranch, onBranch.Branch = false, "feat/add-vat-rate", "feat/add-vat-rate"
	bad := vatPlan()
	bad.Title = "Feat: Add VAT."
	choice := perfPlan()
	cases := []struct {
		name string
		lang i18n.Language
		plan app.PlanView
		keys []tea.Msg
	}{
		{"new-branch", i18n.English, vatPlan(), nil},
		{"new-branch.fr", i18n.French, vatPlan(), nil},
		{"on-branch", i18n.English, onBranch, nil},
		{"title-focused", i18n.English, vatPlan(), []tea.Msg{keyTab}},
		{"invalid-title", i18n.English, bad, nil},
		{"secrets-and-size", i18n.English, big, nil},
		{"secrets-and-size.fr", i18n.French, big, nil},
		{"help", i18n.English, vatPlan(), []tea.Msg{keyHelp}},
		{"help.fr", i18n.French, vatPlan(), []tea.Msg{keyHelp}},
		{"type-choice", i18n.English, choice, nil},
		{"type-choice.fr", i18n.French, choice, nil},
		{"type-picked", i18n.English, choice, []tea.Msg{keyRight, keySpace}},
		{"type-choice", i18n.English, choice, nil},
		{"type-choice.fr", i18n.French, choice, nil},
		{"type-picked", i18n.English, choice, []tea.Msg{keyRight, keySpace}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := send(t, newTestReview(tc.lang, tc.plan, &replanner{}), tc.keys...)
			got := screen(m)
			assertFits(t, got)
			golden(t, tc.name, got)
		})
	}
}

func TestReviewEnterShipsTheProposedPlan(t *testing.T) {
	m := send(t, newTestReview(i18n.English, vatPlan(), &replanner{}), keyEnter)
	if !m.done || m.outcome.action != reviewShip || m.outcome.title != "✨ feat(cart): add vat rate" ||
		m.outcome.branch != "feat/add-vat-rate" || m.outcome.only != nil {
		t.Fatalf("outcome = %+v (done %v)", m.outcome, m.done)
	}
	if m.View().Content != "" {
		t.Error("the screen must clear itself once decided")
	}
}

func TestReviewEditsEveryField(t *testing.T) {
	m := newTestReview(i18n.English, vatPlan(), &replanner{})
	msgs := []tea.Msg{keyTab, keyCtrlU}                     // Title, emptied
	msgs = append(msgs, typed("🐛 fix(cart): round vat")...) // a new title
	msgs = append(msgs, keyEnter)                           // Enter in a text field: next field
	if m = send(t, m, msgs...); m.focus != fieldBranch {
		t.Fatalf("focus = %d, want the branch", m.focus)
	}
	msgs = []tea.Msg{keyCtrlU}
	msgs = append(msgs, typed("fix/round-vat")...)
	msgs = append(msgs, keyTab)
	msgs = append(msgs, typed("Totals were off by a cent.")...)
	msgs = append(msgs, keyTab)
	msgs = append(msgs, typed("go test ./...")...)
	m = send(t, m, msgs...)
	if s := screen(m); !strings.Contains(s, "⛵ ship · main → fix/round-vat") || !strings.Contains(s, "✓ valid · 22/72") {
		t.Errorf("the header and the title check follow the edits:\n%s", s)
	}
	m = send(t, m, keyCtrlEnter) // ships from a text field
	want := reviewOutcome{action: reviewShip, title: "🐛 fix(cart): round vat", branch: "fix/round-vat",
		why: "Totals were off by a cent.", tested: "go test ./..."}
	if !m.done || !equalOutcome(m.outcome, want) {
		t.Fatalf("outcome = %+v", m.outcome)
	}
}

func equalOutcome(a, b reviewOutcome) bool {
	return a.action == b.action && a.title == b.title && a.branch == b.branch && a.why == b.why &&
		a.tested == b.tested && slices.Equal(a.only, b.only)
}

func TestReviewKeys(t *testing.T) {
	for name, tc := range map[string]struct {
		keys []tea.Msg
		want reviewAction
	}{
		"esc cancels":           {[]tea.Msg{keyEsc}, reviewCancel},
		"ctrl+c cancels":        {[]tea.Msg{keyTab, keyCtrlC}, reviewCancel},
		"ctrl+l commits only":   {[]tea.Msg{keyCtrlL}, reviewLocal},
		"f2 ships from a field": {[]tea.Msg{keyTab, keyF2}, reviewShip},
		"legacy ctrl+enter":     {[]tea.Msg{keyTab, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}}, reviewShip},
	} {
		t.Run(name, func(t *testing.T) {
			m := send(t, newTestReview(i18n.English, vatPlan(), &replanner{}), tc.keys...)
			if !m.done || m.outcome.action != tc.want {
				t.Fatalf("outcome = %+v (done %v)", m.outcome, m.done)
			}
		})
	}
	// Tab and Shift+Tab cycle through the fields, the branch included.
	m := newTestReview(i18n.English, vatPlan(), &replanner{})
	var order []reviewField
	for range 5 {
		m = send(t, m, keyTab)
		order = append(order, m.focus)
	}
	if !slices.Equal(order, []reviewField{fieldTitle, fieldBranch, fieldWhy, fieldTested, fieldFiles}) {
		t.Errorf("Tab order = %v", order)
	}
	if m = send(t, m, keyShiftTab); m.focus != fieldTested {
		t.Errorf("Shift+Tab focus = %d", m.focus)
	}
	// "?" and Space are text in a text field.
	m = send(t, m, keySpace, keyHelp)
	if m.help || m.inputs[fieldTested].Value() != " ?" {
		t.Errorf("help %v, tested %q", m.help, m.inputs[fieldTested].Value())
	}
	// Outside one, "?" opens the help and any key closes it without acting.
	m = send(t, m, keyTab, keyHelp)
	if !m.help {
		t.Fatal("? must open the help")
	}
	if m = send(t, m, keyEnter); m.help || m.done {
		t.Fatalf("a key closes the help only: help %v, done %v", m.help, m.done)
	}
}

func TestReviewRefusesWhatCannotShip(t *testing.T) {
	bad := vatPlan()
	bad.Title = "Feat: Add VAT."
	m := send(t, newTestReview(i18n.English, bad, &replanner{}), keyEnter)
	if m.done || m.focus != fieldTitle || !strings.Contains(screen(m), "✗ Fix the title first.") {
		t.Fatalf("an invalid title ships: done %v, focus %d\n%s", m.done, m.focus, screen(m))
	}
	m = send(t, newTestReview(i18n.English, vatPlan(), &replanner{}), keyTab, keyTab, keyCtrlU, keyBackspace)
	m = send(t, m, typed("Feature/VAT")...)
	if m = send(t, m, keyF2); m.done || m.focus != fieldBranch || !strings.Contains(screen(m), "✗ Fix the branch first.") {
		t.Fatalf("an invalid branch ships: done %v\n%s", m.done, screen(m))
	}
	secret := vatPlan()
	secret.Secrets = []app.SecretView{{Path: "docs/vat.md", Line: 7, Rule: "aws_key", Preview: "AKIA…", Length: 20}}
	m = send(t, newTestReview(i18n.English, secret, &replanner{}), keyEnter)
	if m.done || !strings.Contains(screen(m), "docs/vat.md:7") || !strings.Contains(screen(m), "✗ Secrets block shipping") {
		t.Fatalf("secrets ship: done %v\n%s", m.done, screen(m))
	}
}

func TestReviewSelectionReplansAndKeepsEdits(t *testing.T) {
	rp := &replanner{plan: func(only []string) app.PlanView {
		p := vatPlan()
		p.Title, p.Branch, p.Files = "✨ feat(cart): add vat", "feat/add-vat", only
		return p
	}}
	m := newTestReview(i18n.English, vatPlan(), rp)
	// Unselect docs/vat.md: down to the third file, Space.
	m = send(t, m, keyDown, keyDown, keyDown, keySpace)
	if len(rp.calls) != 1 || !slices.Equal(rp.calls[0], []string{"internal/cart/vat.go", "internal/cart/vat_test.go"}) {
		t.Fatalf("replan calls = %q", rp.calls)
	}
	if s := screen(m); !strings.Contains(s, "▾ 2/3 selected") || !strings.Contains(s, "[ ] docs/vat.md") ||
		!strings.Contains(s, "✨ feat(cart): add vat\n") {
		t.Fatalf("the screen follows the new plan:\n%s", s)
	}
	// An edited title is kept through the next plan; the untouched branch follows the title.
	m = send(t, m, keyTab) // wraps to the title
	m = send(t, m, typed(" now")...)
	m = send(t, m, keyShiftTab, keySpace) // back on docs/vat.md, selected again
	if got := m.inputs[fieldTitle].Value(); got != "✨ feat(cart): add vat now" {
		t.Errorf("edited title = %q", got)
	}
	if got := m.inputs[fieldBranch].Value(); got != "feat/add-vat-now" {
		t.Errorf("untouched branch = %q, want the edited title's", got)
	}
	m = send(t, m, keyEnter)
	if m.outcome.only != nil {
		t.Errorf("everything selected ships the command line's selection, got %q", m.outcome.only)
	}

	// Nothing selected: no plan, and nothing ships.
	m = send(t, newTestReview(i18n.English, vatPlan(), rp), keyDown, keySpace, keyDown, keySpace, keyDown, keySpace, keyEnter)
	if m.done || !strings.Contains(screen(m), "✗ Select at least one file.") {
		t.Fatalf("an empty selection ships:\n%s", screen(m))
	}
	// A plan that fails says why and blocks shipping.
	failing := &replanner{err: &app.Fault{Code: "SHIP-E0201", Key: "plan.nothing_to_ship"}}
	m = send(t, newTestReview(i18n.English, vatPlan(), failing), keyDown, keySpace, keyEnter)
	if m.done || !strings.Contains(screen(m), "SHIP-E0201") {
		t.Fatalf("a failed plan ships:\n%s", screen(m))
	}
}

func TestReviewShipsOnePart(t *testing.T) {
	plan := vatPlan()
	plan.Size = &app.SizeView{Lines: 900, Max: 400, Groups: []split.Group{
		{Path: "internal/cart/", Lines: 800, Files: []string{"internal/cart/vat.go", "internal/cart/vat_test.go"}},
		{Path: "docs/vat.md", Lines: 100, Files: []string{"docs/vat.md"}},
	}}
	rp := &replanner{plan: func(only []string) app.PlanView {
		p := vatPlan()
		p.Files = only
		return p // small enough now: no size warning
	}}
	m := send(t, newTestReview(i18n.English, plan, rp), keyTab) // the parts, after the files
	if m.focus != fieldParts {
		t.Fatalf("focus = %d, want the parts", m.focus)
	}
	m = send(t, m, keyDown, keySpace)
	if s := screen(m); !strings.Contains(s, "› (•) docs/vat.md") || !strings.Contains(s, "Parts that ship on their own") {
		t.Fatalf("the part is selected:\n%s", s)
	}
	if m = send(t, m, keyEnter); !slices.Equal(m.outcome.only, []string{"docs/vat.md"}) {
		t.Fatalf("only = %q", m.outcome.only)
	}
}

func TestLineInputEditing(t *testing.T) {
	in := lineInput{focused: true, width: 8}
	in.SetValue("add vat")
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyHome}, {Code: tea.KeyRight}, {Code: tea.KeyDelete}} {
		in.update(k)
	}
	in.update(tea.PasteMsg{Content: "x\ny"})
	if in.Value() != "ax yd vat" {
		t.Fatalf("value = %q", in.Value())
	}
	in.update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if in.Value() != "ax d vat" {
		t.Fatalf("after ctrl+w = %q", in.Value())
	}
	in.SetValue("a long title that scrolls")
	if v := ansi.Strip(in.view()); lipgloss.Width(v) > 8 || !strings.HasSuffix(v, "lls ") {
		t.Fatalf("view = %q: the end and the cursor must show", v)
	}
	if in.update(tea.KeyPressMsg{Code: tea.KeyF5}) {
		t.Error("a key without text is not an edit")
	}
}

func TestReviewModeFollowsTheTerminal(t *testing.T) {
	r := runner{streams: Streams{Interactive: true, TerminalUI: true}}
	if got := r.reviewModeFor(false); got != reviewScreen {
		t.Errorf("terminal: mode %d", got)
	}
	for name, env := range map[string][]string{"SHIP_ACCESSIBLE": {"SHIP_ACCESSIBLE=1"}, "TERM=dumb": {"TERM=dumb"}} {
		r.deps.Environ = env
		if got := r.reviewModeFor(false); got != reviewPlain {
			t.Errorf("%s: mode %d", name, got)
		}
	}
	r.deps.Environ = []string{"NO_COLOR=1"} // colours go, the screen stays
	if got := r.reviewModeFor(false); got != reviewScreen {
		t.Errorf("NO_COLOR: mode %d", got)
	}
	r.global.plain = true
	if got := r.reviewModeFor(false); got != reviewPlain {
		t.Errorf("--plain: mode %d", got)
	}
	r.global.plain = false
	r.ui.accessible = true
	if got := r.reviewModeFor(false); got != reviewPlain {
		t.Errorf("ui.accessible: mode %d", got)
	}
	r.ui = uiPrefs{noReviewScreen: true}
	if got := r.reviewModeFor(false); got != reviewConfirm {
		t.Errorf("ui.reviewScreen false: mode %d", got)
	}
	r.ui = uiPrefs{}
	if got := r.reviewModeFor(true); got != reviewNone {
		t.Errorf("--yes: mode %d", got)
	}
	r.streams.TerminalUI = false
	if got := r.reviewModeFor(false); got != reviewConfirm {
		t.Errorf("stderr redirected: mode %d", got)
	}
	r.streams.Interactive = false
	if got := r.reviewModeFor(false); got != reviewNobody {
		t.Errorf("no terminal: mode %d", got)
	}
}

func TestReviewUntouchedBranchFollowsTheTitle(t *testing.T) {
	rp := &replanner{plan: func(only []string) app.PlanView {
		p := vatPlan()
		p.Title, p.Branch, p.Files = "✨ feat(cart): add vat", "feat/add-vat", only
		return p
	}}
	m := send(t, newTestReview(i18n.English, vatPlan(), rp), keyTab, keyCtrlU)
	m = send(t, m, typed("🐛 fix(cart): round vat")...)
	if got := m.inputs[fieldBranch].Value(); got != "fix/round-vat" {
		t.Fatalf("untouched branch = %q, want the edited title's", got)
	}
	// A new plan keeps the edited title, so the branch stays the title's.
	m = send(t, m, keyShiftTab, keyDown, keyDown, keyDown, keySpace)
	if got := m.inputs[fieldBranch].Value(); got != "fix/round-vat" || len(rp.calls) != 1 {
		t.Fatalf("after a new plan: branch %q, calls %q", got, rp.calls)
	}
	// Once the human edits the branch, the title no longer moves it.
	m = send(t, m, keyTab, keyTab) // files → title → branch
	if m.focus != fieldBranch {
		t.Fatalf("focus = %d", m.focus)
	}
	m = send(t, m, typed("-2")...)
	m = send(t, m, keyShiftTab)
	m = send(t, m, typed(" again")...)
	if got := m.inputs[fieldBranch].Value(); got != "fix/round-vat-2" {
		t.Fatalf("edited branch = %q, must stay", got)
	}
}

func TestPlainReviewProposesTheBranchOfTheEditedTitle(t *testing.T) {
	var calls []string
	code, _, stderr := runReview(t, "✨ feat: add retries\n\n\n\nn\n", true, nil, &calls, "--plain")
	if code != ExitAborted || !strings.Contains(stderr, "Branch [feat/add-retries]: ") {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
}

func TestReviewBlocksAnyTypeTheProjectRefuses(t *testing.T) {
	m := newTestReview(i18n.English, perfPlan(), &replanner{})
	// The human types a type that is neither the refused one nor a choice.
	m.inputs[fieldTitle].SetValue("✨ feat(cart): cache vat rates")
	m = send(t, m, keyEnter)
	if m.done || !strings.Contains(screen(m), "Pick a commit type") {
		t.Fatalf("a type outside the choices shipped:\n%s", screen(m))
	}
	if !strings.Contains(screen(m), "✗") {
		t.Fatalf("the screen must say the type is refused:\n%s", screen(m))
	}
}

func TestScreenEnvironmentColours(t *testing.T) {
	tests := []struct {
		env   []string
		color string
		want  []string
	}{
		{[]string{"TERM=xterm"}, "", []string{"TERM=xterm"}},
		{[]string{"TERM=xterm", "CLICOLOR_FORCE=1"}, app.ColorAuto, []string{"TERM=xterm", "CLICOLOR_FORCE=1"}},
		{[]string{"TERM=xterm"}, app.ColorNever, []string{"TERM=xterm", "NO_COLOR=1"}},
		{[]string{"TERM=xterm", "CLICOLOR_FORCE=1"}, app.ColorNever, []string{"TERM=xterm", "NO_COLOR=1"}},
		{[]string{"TERM=xterm"}, app.ColorAlways, []string{"TERM=xterm", "CLICOLOR_FORCE=1"}},
		// NO_COLOR, whatever its value, wins over ui.color.
		{[]string{"NO_COLOR=yes please", "TERM=xterm"}, app.ColorAlways, []string{"TERM=xterm", "NO_COLOR=1"}},
		{[]string{"NO_COLOR=x"}, "", []string{"NO_COLOR=1"}},
	}
	for _, tt := range tests {
		r := runner{deps: Deps{Environ: tt.env}, ui: uiPrefs{color: tt.color}}
		if got := r.screenEnvironment(); !slices.Equal(got, tt.want) {
			t.Errorf("%v, ui.color %q: %v, want %v", tt.env, tt.color, got, tt.want)
		}
	}
}

func TestLanguagePrecedence(t *testing.T) {
	r := runner{lang: i18n.French} // from the locale
	if got := r.language(""); got != i18n.French {
		t.Errorf("auto: %s", got)
	}
	if got := r.language("en"); got != i18n.English {
		t.Errorf("ui.language en: %s", got)
	}
	r.deps.Environ = []string{"SHIP_LANG=fr_FR.UTF-8"}
	if got := r.language("en"); got != i18n.French {
		t.Errorf("SHIP_LANG beats ui.language: %s", got)
	}
}
