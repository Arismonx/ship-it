// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// The review screen of `ship stack` (decision of 2026-11-05), drawn like the
// one of `ship`: 80 columns, on stderr, every state written. It lists the
// layers bottom first; the human reorders them, moves files from one to
// another and edits titles and branches, checked as they are typed.

// stackRow is what a line of the screen holds; the cursor is on one.
type stackRow int

const (
	rowLayer stackRow = iota // the layer's heading: reordering
	rowTitle
	rowBranch
	rowFile // one of its files: moving it
)

type stackCursor struct {
	layer int
	row   stackRow
	file  int
}

// stackLayerEdit is one layer as the human edits it.
type stackLayerEdit struct {
	title, branch lineInput
	files         []string
	lines         int
	// planTitle, planBranch and autoBranch play the part they play on the
	// review screen of ship: what the plan proposed follows the next plan
	// until the human edits it.
	planTitle, planBranch, autoBranch string
}

// stackReviewConfig is what the stack review starts from.
type stackReviewConfig struct {
	lang        i18n.Language
	plan        app.StackPlanView
	conventions app.Conventions
	checkBranch func(name string) app.BranchCheck
	branchFor   func(title string) (branch string, ok bool)
	// replan proposes the titles and branches of layers holding these files,
	// bottom first.
	replan func(layers [][]string) (app.StackPlanView, error)
}

// stackReviewOutcome is what the human decided.
type stackReviewOutcome struct {
	action reviewAction
	layers []app.StackLayerSpec
}

type stackReviewModel struct {
	cfg    stackReviewConfig
	layers []stackLayerEdit
	left   []string
	cur    stackCursor
	help   bool
	notice string
	width  int

	delay    time.Duration
	seq      int
	planning bool
	planErr  error

	done    bool
	outcome stackReviewOutcome
}

type stackReplanTick struct{ seq int }

type stackReplanDone struct {
	seq  int
	plan app.StackPlanView
	err  error
}

func newStackReviewModel(cfg stackReviewConfig) stackReviewModel {
	m := stackReviewModel{cfg: cfg, left: cfg.plan.Left, width: reviewWidth, delay: replanDelay}
	for _, l := range cfg.plan.Layers {
		e := stackLayerEdit{files: slices.Clone(l.Files), lines: l.Lines, planTitle: l.Title, planBranch: l.Branch, autoBranch: l.Branch}
		e.title.SetValue(l.Title)
		e.branch.SetValue(l.Branch)
		m.layers = append(m.layers, e)
	}
	m.resizeInputs()
	return m
}

func (m stackReviewModel) Init() tea.Cmd { return nil }

func (m stackReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = min(max(msg.Width, 20), reviewWidth)
		m.resizeInputs()
		return m, nil
	case stackReplanTick:
		if msg.seq != m.seq || m.cfg.replan == nil {
			return m, nil
		}
		replan, layers, seq := m.cfg.replan, m.selection(), msg.seq
		return m, func() tea.Msg {
			p, err := replan(layers)
			return stackReplanDone{seq: seq, plan: p, err: err}
		}
	case stackReplanDone:
		if msg.seq == m.seq {
			m.applyPlan(msg.plan, msg.err)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	if m.onText() {
		m.edit(msg)
	}
	return m, nil
}

func (m stackReviewModel) onText() bool { return m.cur.row == rowTitle || m.cur.row == rowBranch }

// input is the text field under the cursor, nil on another line.
func (m *stackReviewModel) input() *lineInput {
	switch m.cur.row {
	case rowTitle:
		return &m.layers[m.cur.layer].title
	case rowBranch:
		return &m.layers[m.cur.layer].branch
	}
	return nil
}

func (m stackReviewModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.help {
		if k == "ctrl+c" {
			return m.quit(stackReviewOutcome{action: reviewCancel})
		}
		m.help = false
		return m, nil
	}
	m.notice = ""
	switch k {
	case "ctrl+c", "esc":
		return m.quit(stackReviewOutcome{action: reviewCancel})
	case "ctrl+enter", "ctrl+j", "f2":
		return m.finish(reviewShip)
	case "ctrl+l":
		return m.finish(reviewLocal)
	case "up":
		return m.moveCursor(-1), nil
	case "down":
		return m.moveCursor(1), nil
	case "tab":
		return m.nextLayer(1), nil
	case "shift+tab":
		return m.nextLayer(-1), nil
	case "enter":
		if m.onText() {
			return m.moveCursor(1), nil
		}
		return m.finish(reviewShip)
	}
	if m.onText() {
		m.edit(msg)
		return m, nil
	}
	switch k {
	case "k":
		return m.moveCursor(-1), nil
	case "j":
		return m.moveCursor(1), nil
	case "shift+up", "K":
		return m.shift(-1)
	case "shift+down", "J":
		return m.shift(1)
	case "?":
		m.help = true
		return m, nil
	}
	if n, err := strconv.Atoi(k); err == nil && m.cur.row == rowFile && n >= 1 && n <= len(m.layers) {
		return m.moveFile(n - 1)
	}
	return m, nil
}

// shift moves the layer under the cursor one place in the list, or its file
// to the layer shown above or below.
func (m stackReviewModel) shift(delta int) (tea.Model, tea.Cmd) {
	if m.cur.row == rowFile {
		return m.moveFile(m.cur.layer + delta)
	}
	j := m.cur.layer + delta
	if j < 0 || j >= len(m.layers) {
		return m, nil
	}
	m.layers[m.cur.layer], m.layers[j] = m.layers[j], m.layers[m.cur.layer]
	m.cur.layer = j
	m.focusInputs()
	return m, nil
}

// moveFile moves the file under the cursor to layer to; the cursor follows
// it, and the titles of the layers follow the plan of their new files.
func (m stackReviewModel) moveFile(to int) (tea.Model, tea.Cmd) {
	from := m.cur.layer
	if to < 0 || to >= len(m.layers) || to == from {
		return m, nil
	}
	l := &m.layers[from]
	f := l.files[m.cur.file]
	l.files = slices.Delete(slices.Clone(l.files), m.cur.file, m.cur.file+1)
	m.layers[to].files = append(slices.Clone(m.layers[to].files), f)
	m.cur = stackCursor{layer: to, row: rowFile, file: len(m.layers[to].files) - 1}
	m.focusInputs()
	return m, m.filesChanged()
}

// filesChanged schedules a new plan once the moves settle; none while a
// layer is empty, which cannot ship.
func (m *stackReviewModel) filesChanged() tea.Cmd {
	m.seq++
	if m.emptyLayer() >= 0 || m.cfg.replan == nil {
		m.planning = false
		return nil
	}
	m.planning = true
	seq := m.seq
	return tea.Tick(m.delay, func(time.Time) tea.Msg { return stackReplanTick{seq: seq} })
}

func (m stackReviewModel) emptyLayer() int {
	return slices.IndexFunc(m.layers, func(l stackLayerEdit) bool { return len(l.files) == 0 })
}

func (m stackReviewModel) selection() [][]string {
	layers := make([][]string, len(m.layers))
	for i, l := range m.layers {
		layers[i] = slices.Clone(l.files)
	}
	return layers
}

func (m *stackReviewModel) applyPlan(p app.StackPlanView, err error) {
	m.planning, m.planErr = false, err
	if err != nil || len(p.Layers) != len(m.layers) {
		return
	}
	for i := range m.layers {
		l, pl := &m.layers[i], p.Layers[i]
		untouched := l.branch.Value() == l.autoBranch
		if l.title.Value() == l.planTitle {
			l.title.SetValue(pl.Title)
		}
		l.planTitle, l.planBranch, l.lines = pl.Title, pl.Branch, pl.Lines
		if untouched {
			m.proposeBranch(i, true)
		}
	}
	m.left = p.Left
}

// edit sends msg to the text field under the cursor; an edited title moves
// an untouched branch along.
func (m *stackReviewModel) edit(msg tea.Msg) {
	in := m.input()
	before := in.Value()
	in.update(msg)
	if m.cur.row == rowTitle && in.Value() != before {
		m.proposeBranch(m.cur.layer, false)
	}
}

// proposeBranch is reviewModel.proposeBranch for layer i.
func (m *stackReviewModel) proposeBranch(i int, untouched bool) {
	l := &m.layers[i]
	if !untouched && l.branch.Value() != l.autoBranch {
		return
	}
	auto := l.planBranch
	if title := strings.TrimSpace(l.title.Value()); title != l.planTitle && m.cfg.branchFor != nil {
		auto = l.autoBranch
		if b, ok := m.cfg.branchFor(title); ok {
			auto = b
		}
	}
	l.autoBranch = auto
	l.branch.SetValue(auto)
}

// rows lists every line the cursor stops on, in screen order. The files of
// every layer are in it, though only the layer under the cursor shows them:
// moving up from a layer lands on the last file of the one above.
func (m stackReviewModel) rows() []stackCursor {
	var rows []stackCursor
	for i, l := range m.layers {
		rows = append(rows, stackCursor{layer: i, row: rowLayer}, stackCursor{layer: i, row: rowTitle}, stackCursor{layer: i, row: rowBranch})
		for j := range l.files {
			rows = append(rows, stackCursor{layer: i, row: rowFile, file: j})
		}
	}
	return rows
}

func (m stackReviewModel) moveCursor(delta int) stackReviewModel {
	rows := m.rows()
	i := max(slices.Index(rows, m.cur), 0)
	m.cur = rows[min(max(i+delta, 0), len(rows)-1)]
	m.focusInputs()
	return m
}

func (m stackReviewModel) nextLayer(delta int) stackReviewModel {
	m.cur = stackCursor{layer: (m.cur.layer + delta + len(m.layers)) % len(m.layers), row: rowLayer}
	m.focusInputs()
	return m
}

// focus puts the cursor on c.
func (m stackReviewModel) focus(c stackCursor) stackReviewModel {
	m.cur = c
	m.focusInputs()
	return m
}

// focusInputs gives the text field under the cursor, if any, the focus.
func (m *stackReviewModel) focusInputs() {
	for i := range m.layers {
		m.layers[i].title.focused, m.layers[i].branch.focused = false, false
	}
	if in := m.input(); in != nil {
		in.focused = true
		in.pos = len(in.value)
	}
}

// branchFault says what is wrong with the branch of layer i, nil when
// nothing: the conventions first, then a name another layer has.
func (m stackReviewModel) branchFault(i int) *app.Fault {
	name := strings.TrimSpace(m.layers[i].branch.Value())
	if m.cfg.checkBranch != nil {
		if check := m.cfg.checkBranch(name); !check.Valid() {
			return check.Fault
		}
	}
	for j, other := range m.layers {
		if j != i && strings.TrimSpace(other.branch.Value()) == name {
			return app.StackBranchTaken(name)
		}
	}
	return nil
}

// finish ships, or says what stands in the way and goes there.
func (m stackReviewModel) finish(action reviewAction) (tea.Model, tea.Cmd) {
	t := func(key string, params map[string]string) string { return i18n.Render(m.cfg.lang, key, params) }
	if i := m.emptyLayer(); i >= 0 {
		m.notice = t("cli.stack_review.blocked.empty", map[string]string{"position": strconv.Itoa(i + 1)})
		return m.focus(stackCursor{layer: i, row: rowLayer}), nil
	}
	switch {
	case m.planning:
		m.notice = t("cli.review.blocked.planning", nil)
		return m, nil
	case m.planErr != nil:
		m.notice = "✗ " + errorText(m.cfg.lang, m.planErr)
		return m, nil
	}
	for i, l := range m.layers {
		if !m.cfg.conventions.CheckTitle(strings.TrimSpace(l.title.Value())).Valid() {
			m.notice = t("cli.review.blocked.title", nil)
			return m.focus(stackCursor{layer: i, row: rowTitle}), nil
		}
		if m.branchFault(i) != nil {
			m.notice = t("cli.review.blocked.branch", nil)
			return m.focus(stackCursor{layer: i, row: rowBranch}), nil
		}
	}
	outcome := stackReviewOutcome{action: action}
	for _, l := range m.layers {
		outcome.layers = append(outcome.layers, app.StackLayerSpec{
			Title: strings.TrimSpace(l.title.Value()), Branch: strings.TrimSpace(l.branch.Value()), Files: l.files,
		})
	}
	return m.quit(outcome)
}

func (m stackReviewModel) quit(o stackReviewOutcome) (tea.Model, tea.Cmd) {
	m.done, m.outcome = true, o
	return m, tea.Quit
}

func (m stackReviewModel) labelWidth() int {
	w := 0
	for _, l := range []string{"title", "branch", "files"} {
		w = max(w, lipgloss.Width(i18n.Render(m.cfg.lang, "cli.review.label."+l, nil)))
	}
	return w
}

func (m *stackReviewModel) resizeInputs() {
	for i := range m.layers {
		w := max(m.width-m.labelWidth()-6, 10)
		m.layers[i].title.width, m.layers[i].branch.width = w, w
	}
}

func (m stackReviewModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(m.render())
}

// render lays the screen out as text; View wraps it, tests read it.
func (m stackReviewModel) render() string {
	lang := m.cfg.lang
	t := func(key string, params map[string]string) string { return i18n.Render(lang, key, params) }
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }

	head := t("cli.stack_review.header", map[string]string{"count": strconv.Itoa(len(m.layers)), "base": m.cfg.plan.Base})
	if m.planning {
		head += "  " + styleFaint.Render(t("cli.review.replanning", nil))
	}
	line(head)
	line("")
	if m.help {
		for _, l := range strings.Split(t("cli.stack_review.help", nil), "\n") {
			line("  " + l)
		}
		return b.String()
	}

	lw := m.labelWidth()
	indent := strings.Repeat(" ", lw+6)
	room := m.width - lw - 6
	pointer := func(c stackCursor) string {
		if m.cur == c {
			return "› "
		}
		return "  "
	}
	label := func(c stackCursor, name string) string {
		text := ""
		if name != "" {
			text = t("cli.review.label."+name, nil)
		}
		pad := strings.Repeat(" ", lw-lipgloss.Width(text))
		if c.row == rowFile {
			// The pointer of a file stands next to it, as on the screen of ship.
			return "    " + text + pad + pointer(c)
		}
		if m.cur == c {
			text = styleFocused.Render(text)
		}
		return pointer(c) + "  " + text + pad + "  "
	}
	for i, l := range m.layers {
		heading := stackCursor{layer: i, row: rowLayer}
		text := t("cli.stack_review.layer", map[string]string{
			"position": strconv.Itoa(i + 1), "files": strconv.Itoa(len(l.files)), "lines": strconv.Itoa(l.lines)})
		if m.cur == heading {
			text = styleFocused.Render(text)
		}
		line(pointer(heading) + text)
		if len(l.files) == 0 {
			line(indent + styleBad.Render(fit(t("cli.stack_review.empty", nil), room)))
		}

		titleRow := stackCursor{layer: i, row: rowTitle}
		line(label(titleRow, "title") + l.title.view())
		title := strings.TrimSpace(l.title.Value())
		if check := m.cfg.conventions.CheckTitle(title); check.Valid() {
			line(indent + styleOK.Render(t("cli.review.title_valid", map[string]string{
				"length": strconv.Itoa(utf8.RuneCountInString(title)), "max": strconv.Itoa(m.cfg.conventions.MaxTitleLength()),
			})))
		} else {
			f := check.Faults[0]
			line(indent + styleBad.Render(fit("✗ "+t(f.Key, f.Params), room)))
		}

		branchRow := stackCursor{layer: i, row: rowBranch}
		line(label(branchRow, "branch") + l.branch.view())
		if f := m.branchFault(i); f != nil {
			line(indent + styleBad.Render(fit("✗ "+t(f.Key, f.Params), room)))
		} else {
			line(indent + styleOK.Render(t("cli.review.branch_valid", nil)))
		}

		if len(l.files) > 0 && m.cur.layer != i {
			line(label(stackCursor{layer: -1}, "files") + styleFaint.Render(fit(strings.Join(l.files, ", "), room)))
		} else if len(l.files) > 0 {
			m.renderFiles(line, i, label)
		}
		line("")
	}
	if len(m.left) > 0 {
		line(fit(t("cli.stack.left", map[string]string{"files": strings.Join(m.left, ", ")}), m.width))
		line("")
	}
	if m.planErr != nil {
		line(styleBad.Render(fit("✗ "+errorText(lang, m.planErr), m.width)))
		line("")
	}
	if m.notice != "" {
		line(m.notice)
	}
	keys := "cli.stack_review.keys"
	switch m.cur.row {
	case rowTitle, rowBranch:
		keys = "cli.stack_review.keys_text"
	case rowFile:
		keys = "cli.stack_review.keys_file"
	}
	line(styleFaint.Render(t(keys, map[string]string{"count": strconv.Itoa(len(m.layers))})))
	return b.String()
}

// renderFiles lists the files of the layer under the cursor: a window of
// maxVisibleFiles lines that follows it.
func (m stackReviewModel) renderFiles(line func(string), i int, label func(stackCursor, string) string) {
	files := m.layers[i].files
	room := m.width - m.labelWidth() - 6
	first := 0
	if m.cur.row == rowFile && m.cur.file >= maxVisibleFiles {
		first = m.cur.file - maxVisibleFiles + 1
	}
	last := min(first+maxVisibleFiles, len(files))
	for j := first; j < last; j++ {
		name := ""
		if j == first {
			name = "files"
		}
		line(label(stackCursor{layer: i, row: rowFile, file: j}, name) + fitLeft(files[j], room))
	}
	if hidden := len(files) - last + first; hidden > 0 {
		line(strings.Repeat(" ", m.labelWidth()+6) + styleFaint.Render(i18n.Render(m.cfg.lang, "cli.review.files_hidden",
			map[string]string{"count": strconv.Itoa(hidden)})))
	}
}

// errorText renders an error of the plan: a fault in lang, else as is.
func errorText(lang i18n.Language, err error) string {
	if f, ok := err.(*app.Fault); ok {
		return string(f.Code) + "  " + i18n.Render(lang, f.Key, f.Params)
	}
	return err.Error()
}
