// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/testkit"
)

var (
	keyUp        = tea.KeyPressMsg{Code: tea.KeyUp}
	keyShiftUp   = tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}
	keyShiftDown = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
)

// cartStack is a stack of three layers, as ship stack proposes it.
func cartStack() app.StackPlanView {
	return app.StackPlanView{Base: "main", MaxLayers: 5, Left: []string{"README.md"}, Layers: []app.StackLayerPlan{
		{Title: "✨ feat(api): add the cart", Branch: "feat/add-the-cart", Area: "api/", Lines: 310,
			Files: []string{"api/cart.go", "api/cart_test.go"}},
		{Title: "✨ feat(web): add the cart page", Branch: "feat/add-the-cart-page", Area: "web/", Lines: 120,
			Files: []string{"web/cart.ts"}},
		{Title: "📝 docs: describe the cart", Branch: "chore/describe-the-cart", Area: "docs/", Lines: 40,
			Files: []string{"docs/cart.md"}},
	}}
}

// stackReplanner records the layers it was asked to plan and titles each
// after its first file.
type stackReplanner struct{ calls [][][]string }

func (s *stackReplanner) replan(layers [][]string) (app.StackPlanView, error) {
	s.calls = append(s.calls, layers)
	view := cartStack()
	for i, files := range layers {
		view.Layers[i].Files, view.Layers[i].Lines = files, 10*len(files)
	}
	view.Layers[1].Title, view.Layers[1].Branch = "✨ feat(web): add the cart and its api", "feat/add-the-cart-and-its-api"
	return view, nil
}

func newTestStackReview(lang i18n.Language, plan app.StackPlanView, rp *stackReplanner) stackReviewModel {
	m := newStackReviewModel(stackReviewConfig{
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

// sendStack is send for the stack review.
func sendStack(t *testing.T, m stackReviewModel, msgs ...tea.Msg) stackReviewModel {
	t.Helper()
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next.(stackReviewModel)
		for cmd != nil {
			result := cmd()
			switch result.(type) {
			case stackReplanTick, stackReplanDone:
				next, cmd = m.Update(result)
				m = next.(stackReviewModel)
			default:
				cmd = nil // tea.Quit
			}
		}
	}
	return m
}

// stackScreen is the rendered screen without styles or trailing blanks.
func stackScreen(m stackReviewModel) string {
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

func TestStackReviewGolden(t *testing.T) {
	bad := cartStack()
	bad.Layers[0].Title = "Feat: Add the cart."
	bad.Layers[2].Branch = "feat/add-the-cart"
	down := func(n int) []tea.Msg {
		var msgs []tea.Msg
		for range n {
			msgs = append(msgs, keyDown)
		}
		return msgs
	}
	cases := []struct {
		name string
		lang i18n.Language
		plan app.StackPlanView
		keys []tea.Msg
	}{
		{"stack", i18n.English, cartStack(), nil},
		{"stack.fr", i18n.French, cartStack(), nil},
		{"stack-help", i18n.English, cartStack(), []tea.Msg{keyHelp}},
		{"stack-help.fr", i18n.French, cartStack(), []tea.Msg{keyHelp}},
		{"stack-title-focused", i18n.English, cartStack(), down(1)},
		{"stack-file-focused", i18n.English, cartStack(), down(4)},
		{"stack-file-focused.fr", i18n.French, cartStack(), down(4)},
		// The second file of the bottom layer, moved to layer 2.
		{"stack-file-moved", i18n.English, cartStack(), append(down(4), typed("2")...)},
		{"stack-invalid", i18n.English, bad, nil},
		{"stack-reordered", i18n.English, cartStack(), []tea.Msg{keyShiftDown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := sendStack(t, newTestStackReview(tc.lang, tc.plan, &stackReplanner{}), tc.keys...)
			got := stackScreen(m)
			assertFits(t, got)
			golden(t, tc.name, got)
		})
	}
}

func TestStackReviewShipsTheProposedLayers(t *testing.T) {
	m := sendStack(t, newTestStackReview(i18n.English, cartStack(), &stackReplanner{}), keyEnter)
	if !m.done || m.outcome.action != reviewShip || len(m.outcome.layers) != 3 {
		t.Fatalf("outcome = %+v", m.outcome)
	}
	want := app.StackLayerSpec{Title: "✨ feat(api): add the cart", Branch: "feat/add-the-cart", Files: []string{"api/cart.go", "api/cart_test.go"}}
	if got := m.outcome.layers[0]; got.Title != want.Title || got.Branch != want.Branch || !slices.Equal(got.Files, want.Files) {
		t.Errorf("bottom layer = %+v", got)
	}
	if m.View().Content != "" {
		t.Error("the screen must clear itself once decided")
	}
	for name, tc := range map[string]struct {
		keys []tea.Msg
		want reviewAction
	}{
		"esc cancels":         {[]tea.Msg{keyEsc}, reviewCancel},
		"ctrl+c cancels":      {[]tea.Msg{keyDown, keyCtrlC}, reviewCancel},
		"ctrl+l commits only": {[]tea.Msg{keyCtrlL}, reviewLocal},
		"f2 from a field":     {[]tea.Msg{keyDown, keyF2}, reviewShip},
	} {
		t.Run(name, func(t *testing.T) {
			m := sendStack(t, newTestStackReview(i18n.English, cartStack(), &stackReplanner{}), tc.keys...)
			if !m.done || m.outcome.action != tc.want {
				t.Fatalf("outcome = %+v (done %v)", m.outcome, m.done)
			}
		})
	}
}

func TestStackReviewReordersTheLayers(t *testing.T) {
	m := newTestStackReview(i18n.English, cartStack(), &stackReplanner{})
	m = sendStack(t, m, keyShiftDown, keyShiftDown) // the bottom layer goes to the top
	if m.cur.layer != 2 {
		t.Fatalf("the cursor follows the layer: %+v", m.cur)
	}
	m = sendStack(t, m, tea.KeyPressMsg{Code: 'k', Text: "K", Mod: tea.ModShift}) // K: one place back
	m = sendStack(t, m, keyEnter)
	var branches []string
	for _, l := range m.outcome.layers {
		branches = append(branches, l.Branch)
	}
	if !slices.Equal(branches, []string{"feat/add-the-cart-page", "feat/add-the-cart", "chore/describe-the-cart"}) {
		t.Fatalf("order = %q", branches)
	}
}

func TestStackReviewMovesAFile(t *testing.T) {
	rp := &stackReplanner{}
	m := newTestStackReview(i18n.English, cartStack(), rp)
	m = sendStack(t, m, keyDown, keyDown, keyDown, keyDown) // api/cart_test.go
	if m.cur.row != rowFile || m.layers[0].files[m.cur.file] != "api/cart_test.go" {
		t.Fatalf("cursor = %+v", m.cur)
	}
	m = sendStack(t, m, keyShiftDown) // to the layer below on the screen: layer 2
	if !slices.Equal(m.layers[1].files, []string{"web/cart.ts", "api/cart_test.go"}) || m.cur.layer != 1 {
		t.Fatalf("layers = %+v, cursor %+v", m.layers, m.cur)
	}
	// The plan follows: the untouched title and branch of layer 2 are the new plan's.
	if len(rp.calls) != 1 || m.layers[1].title.Value() != "✨ feat(web): add the cart and its api" ||
		m.layers[1].branch.Value() != "feat/add-the-cart-and-its-api" {
		t.Fatalf("calls %q, layer 2 = %q / %q", rp.calls, m.layers[1].title.Value(), m.layers[1].branch.Value())
	}
	// Shift+↑ and a number move it again; the cursor follows the file.
	m = sendStack(t, m, keyShiftUp)
	m = sendStack(t, m, typed("3")...)
	if !slices.Equal(m.layers[2].files, []string{"docs/cart.md", "api/cart_test.go"}) || m.cur.layer != 2 {
		t.Fatalf("layers = %+v", m.layers)
	}
}

func TestStackReviewKeepsEditsThroughANewPlan(t *testing.T) {
	m := newTestStackReview(i18n.English, cartStack(), &stackReplanner{})
	// Layer 2's title, edited: its untouched branch follows the title.
	m = sendStack(t, m, tea.KeyPressMsg{Code: tea.KeyTab}, keyDown, keyCtrlU)
	m = sendStack(t, m, typed("✨ feat(web): show the cart")...)
	if got := m.layers[1].branch.Value(); got != "feat/show-cart" {
		t.Fatalf("branch = %q", got)
	}
	// A move replans: the edited title and the branch that follows it stay.
	m = sendStack(t, m, keyUp, keyUp) // api/cart_test.go, the last file of layer 1
	m = sendStack(t, m, typed("2")...)
	if got := m.layers[1].title.Value(); got != "✨ feat(web): show the cart" {
		t.Errorf("edited title = %q", got)
	}
	if got := m.layers[1].branch.Value(); got != "feat/show-cart" {
		t.Errorf("branch = %q", got)
	}
}

func TestStackReviewRefusesWhatCannotShip(t *testing.T) {
	bad := cartStack()
	bad.Layers[1].Title = "Feat: Add."
	m := sendStack(t, newTestStackReview(i18n.English, bad, &stackReplanner{}), keyEnter)
	if m.done || m.cur != (stackCursor{layer: 1, row: rowTitle}) || !strings.Contains(stackScreen(m), "✗ Fix the title first.") {
		t.Fatalf("an invalid title ships: done %v, cursor %+v\n%s", m.done, m.cur, stackScreen(m))
	}
	dup := cartStack()
	dup.Layers[2].Branch = "feat/add-the-cart"
	m = sendStack(t, newTestStackReview(i18n.English, dup, &stackReplanner{}), keyEnter)
	if m.done || m.cur != (stackCursor{layer: 0, row: rowBranch}) || !strings.Contains(stackScreen(m), "Another layer already has the branch") {
		t.Fatalf("a duplicate branch ships: done %v, cursor %+v\n%s", m.done, m.cur, stackScreen(m))
	}
	// Layer 2's only file moved away: the layer is empty, nothing replans.
	rp := &stackReplanner{}
	m = sendStack(t, newTestStackReview(i18n.English, cartStack(), rp), tea.KeyPressMsg{Code: tea.KeyTab})
	m = sendStack(t, m, keyDown, keyDown, keyDown)
	m = sendStack(t, m, typed("1")...)
	m = sendStack(t, m, keyEnter)
	if m.done || len(rp.calls) != 0 || !strings.Contains(stackScreen(m), "✗ Layer 2 holds no file") {
		t.Fatalf("an empty layer ships: done %v, calls %d\n%s", m.done, len(rp.calls), stackScreen(m))
	}
}

func TestPlainStackReviewAsksForEachLayer(t *testing.T) {
	var errOut bytes.Buffer
	// Layer 1 keeps its title and gets a new branch; layer 2's title is
	// edited and its branch follows; layer 3 is first given layer 1's branch.
	stdin := "\nfeat/cart-api\n✨ feat(web): show the cart\n\n\nfeat/cart-api\n\n\n"
	r := runner{ctx: context.Background(), lang: i18n.English, streams: Streams{In: strings.NewReader(stdin), Err: &errOut}}
	cfg := stackReviewConfig{lang: i18n.English, plan: cartStack(),
		checkBranch: func(name string) app.BranchCheck { return app.CheckBranch(name) },
		branchFor: func(title string) (string, bool) {
			b, err := app.BranchForTitle(title, 0)
			return b, err == nil
		}}
	outcome := r.plainStackReview(cfg)
	if outcome.action != reviewShip || len(outcome.layers) != 3 {
		t.Fatalf("outcome = %+v\n%s", outcome, errOut.String())
	}
	if outcome.layers[0].Branch != "feat/cart-api" || outcome.layers[1].Branch != "feat/show-cart" {
		t.Errorf("layers = %+v", outcome.layers)
	}
	for _, want := range []string{"Layer 1 title [✨ feat(api): add the cart]: ", "Layer 1 branch [feat/add-the-cart]: ",
		"Layer 2 branch [feat/show-cart]: ", "SHIP-E0110  Another layer already has the branch feat/cart-api",
		"Layer 3 branch [chore/describe-the-cart]: ", "Ship these 3 layers? [Y/n]"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("prompts miss %q:\n%s", want, errOut.String())
		}
	}
	if outcome.layers[2].Branch != "chore/describe-the-cart" {
		t.Errorf("layer 3 = %+v", outcome.layers[2])
	}
	r.streams.In = strings.NewReader("")
	if outcome := r.plainStackReview(cfg); outcome.action != reviewCancel {
		t.Fatalf("end of input: %+v", outcome)
	}
}

// stackRepository is new work in three areas, on main.
func stackRepository(calls *[]string) testkit.FakeGit {
	git := testkit.HealthyRepository("main", "api/cart.go", "web/cart.ts", "docs/cart.md")
	git.DiffText = "diff --git a/api/cart.go b/api/cart.go\n--- a/api/cart.go\n+++ b/api/cart.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/web/cart.ts b/web/cart.ts\n--- a/web/cart.ts\n+++ b/web/cart.ts\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/docs/cart.md b/docs/cart.md\n--- a/docs/cart.md\n+++ b/docs/cart.md\n@@ -1 +1 @@\n-a\n+b\n"
	git.Calls = calls
	return git
}

func runStack(t *testing.T, stdin string, terminalUI bool, calls *[]string, args ...string) (code int, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	streams := Streams{In: strings.NewReader(stdin), Out: &out, Err: &errOut, Interactive: true, TerminalUI: terminalUI}
	deps := Deps{Git: stackRepository(calls), Checkpoints: &testkit.FakeCheckpointStore{}, Operations: &testkit.FakeOperationLog{},
		Forge: &testkit.FakeForge{}, OperatingSystem: "linux"}
	code = Run(context.Background(), append([]string{"stack", "--local"}, args...), streams, i18n.English, testBuild, deps)
	return code, errOut.String()
}

func TestStackCommandReviewModes(t *testing.T) {
	// --plain: one line per prompt, the edited branch is committed.
	var calls []string
	code, stderr := runStack(t, "\nfeat/cart-api\n\n\n\n\n\n", true, &calls, "--plain")
	if code != ExitOK || !slices.Contains(calls, "switch feat/cart-api") {
		t.Fatalf("--plain: exit %d, calls %q\n%s", code, calls, stderr)
	}
	// Declined: nothing changes.
	calls = nil
	if code, _ := runStack(t, "\n\n\n\n\n\nn\n", true, &calls, "--plain"); code != ExitAborted || len(calls) != 0 {
		t.Fatalf("declined: exit %d, calls %q", code, calls)
	}
	// stderr redirected: one question, as before.
	calls = nil
	if code, stderr := runStack(t, "\n", false, &calls); code != ExitOK || !strings.Contains(stderr, "Ship these 3 layers? [Y/n]") {
		t.Fatalf("confirm: exit %d\n%s", code, stderr)
	}
	// --yes asks nothing.
	calls = nil
	if code, stderr := runStack(t, "", false, &calls, "--yes"); code != ExitOK || strings.Contains(stderr, "[Y/n]") {
		t.Fatalf("--yes: exit %d\n%s", code, stderr)
	}
}
