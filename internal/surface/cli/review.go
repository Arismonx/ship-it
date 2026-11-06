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

// The review screen of `ship` (decisions of 2026-10-25): a vertical form,
// laid out for 80 columns and drawn on stderr, where the human edits the
// plan before it ships. Every state is written, never shown by colour only.

type reviewAction int

const (
	reviewCancel reviewAction = iota // Esc, Ctrl+C: nothing changes
	reviewShip                       // commit, then publish per the settings
	reviewLocal                      // commit locally only
)

// reviewOutcome is what the human decided.
type reviewOutcome struct {
	action                     reviewAction
	title, branch, why, tested string
	only                       []string
}

type reviewField int

// Fields in screen order; the text fields index reviewModel.inputs.
const (
	fieldTitle reviewField = iota
	fieldBranch
	fieldWhy
	fieldTested
	fieldFiles
	fieldParts
	// fieldType picks the commit type among those the project allows, shown
	// only when its commitlint refuses the planned one (PlanView.TypeChoices).
	fieldType
)

const (
	// reviewWidth is the width the screen is laid out for.
	reviewWidth = 80
	// replanDelay debounces the plan update after a selection change.
	replanDelay = 300 * time.Millisecond
	// maxVisibleFiles bounds the unfolded file list; the cursor scrolls it.
	maxVisibleFiles = 10
)

// reviewConfig is what the review starts from.
type reviewConfig struct {
	lang i18n.Language
	plan app.PlanView
	// only is the --only selection of the command line, shipped as is when
	// every file stays selected.
	only        []string
	why, tested string
	// conventions are the repository's (ship.commits.*): the title is
	// checked against them as the human edits it.
	conventions app.Conventions
	checkBranch func(name string) app.BranchCheck
	// branchFor proposes the branch of an edited title (app.BranchForTitleOf);
	// ok is false when the title cannot be parsed.
	branchFor func(title string) (branch string, ok bool)
	// replan proposes the plan of a new selection.
	replan func(only []string) (app.PlanView, error)
}

type reviewModel struct {
	cfg  reviewConfig
	plan app.PlanView
	// files are the files the review started with, selected or not; parts
	// are the split suggestions of that first plan.
	files    []string
	parts    []partView
	selected map[string]bool
	inputs   [4]lineInput
	// planTitle and planBranch are what the latest plan proposed: a title
	// still holding planTitle follows the next plan, an edited one is kept.
	planTitle, planBranch string
	// autoBranch is the branch ship proposes now: planBranch, or the branch
	// of the edited title. A branch input holding it is untouched and
	// follows; one the human edited stays.
	autoBranch string

	focus      reviewField
	cursor     int // 0 is the "Files" line, then one per file
	partCursor int
	typeCursor int
	// refusedType is the planned type the project's commitlint refuses; the
	// human picks another before shipping. Empty when there is nothing to pick.
	refusedType string
	filesOpen   bool
	help        bool
	notice      string
	width       int

	delay    time.Duration // debounce of the plan updates
	seq      int           // generation of the latest selection change
	planning bool
	planErr  error

	done    bool
	outcome reviewOutcome
}

type partView struct {
	path         string
	lines, count int
	files        []string
}

type replanTick struct{ seq int }

type replanDone struct {
	seq  int
	plan app.PlanView
	err  error
}

func newReviewModel(cfg reviewConfig) reviewModel {
	m := reviewModel{
		cfg: cfg, plan: cfg.plan, files: slices.Clone(cfg.plan.Files), selected: map[string]bool{},
		planTitle: cfg.plan.Title, planBranch: cfg.plan.Branch, autoBranch: cfg.plan.Branch,
		focus: fieldFiles, width: reviewWidth, delay: replanDelay,
	}
	for _, f := range m.files {
		m.selected[f] = true
	}
	m.filesOpen = len(m.files) <= maxVisibleFiles
	if len(cfg.plan.TypeChoices) > 0 {
		m.refusedType, m.focus = cfg.conventions.TypeOfTitle(cfg.plan.Title), fieldType
	}
	if cfg.plan.Size != nil {
		for i, g := range cfg.plan.Size.Groups {
			if i == maxSplitGroups {
				break
			}
			m.parts = append(m.parts, partView{path: g.Path, lines: g.Lines, count: len(g.Files), files: g.Files})
		}
	}
	values := [4]string{cfg.plan.Title, cfg.plan.Branch, cfg.why, cfg.tested}
	for i := range m.inputs {
		var in lineInput
		in.SetValue(values[i])
		if i >= int(fieldWhy) {
			in.placeholder = i18n.Render(cfg.lang, "cli.review.optional", nil)
		}
		m.inputs[i] = in
	}
	m.resizeInputs()
	return m
}

func (m reviewModel) Init() tea.Cmd { return nil }

func (m reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = min(max(msg.Width, 20), reviewWidth)
		m.resizeInputs()
		return m, nil
	case replanTick:
		if msg.seq != m.seq || m.cfg.replan == nil {
			return m, nil
		}
		replan, only, seq := m.cfg.replan, m.only(), msg.seq
		return m, func() tea.Msg {
			p, err := replan(only)
			return replanDone{seq: seq, plan: p, err: err}
		}
	case replanDone:
		if msg.seq == m.seq {
			m.applyPlan(msg.plan, msg.err)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	if isTextField(m.focus) {
		m.edit(msg)
	}
	return m, nil
}

// edit sends msg to the focused field; an edited title moves an untouched
// branch along.
func (m *reviewModel) edit(msg tea.Msg) {
	before := m.inputs[m.focus].Value()
	m.inputs[m.focus].update(msg)
	if m.focus == fieldTitle && m.inputs[fieldTitle].Value() != before {
		m.proposeBranch(false)
	}
}

// proposeBranch puts the branch ship proposes into an untouched branch
// input: the plan's for the plan's title, otherwise the edited title's (kept
// as it was while the title does not parse). untouched skips the check,
// for a caller that saw the input hold the previous proposal.
func (m *reviewModel) proposeBranch(untouched bool) {
	if !untouched && m.inputs[fieldBranch].Value() != m.autoBranch {
		return
	}
	auto := m.planBranch
	if title := strings.TrimSpace(m.inputs[fieldTitle].Value()); title != m.planTitle && m.cfg.branchFor != nil {
		auto = m.autoBranch
		if b, ok := m.cfg.branchFor(title); ok {
			auto = b
		}
	}
	m.autoBranch = auto
	m.inputs[fieldBranch].SetValue(auto)
}

func isTextField(f reviewField) bool { return f <= fieldTested }

func (m reviewModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.help {
		if k == "ctrl+c" {
			return m.quit(reviewOutcome{action: reviewCancel})
		}
		m.help = false
		return m, nil
	}
	m.notice = ""
	switch k {
	case "ctrl+c", "esc":
		return m.quit(reviewOutcome{action: reviewCancel})
	case "ctrl+enter", "ctrl+j", "f2":
		// Legacy terminals send Ctrl+Enter as LF, read as ctrl+j.
		return m.finish(reviewShip)
	case "ctrl+l":
		return m.finish(reviewLocal)
	case "tab":
		return m.move(1)
	case "shift+tab":
		return m.move(-1)
	case "enter":
		if isTextField(m.focus) {
			return m.move(1)
		}
		return m.finish(reviewShip)
	}
	switch m.focus {
	case fieldFiles:
		return m.filesKey(k)
	case fieldParts:
		return m.partsKey(k)
	case fieldType:
		return m.typeKey(k)
	}
	m.edit(msg)
	return m, nil
}

func (m reviewModel) filesKey(k string) (tea.Model, tea.Cmd) {
	rows := 0
	if m.filesOpen {
		rows = len(m.files)
	}
	switch k {
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, rows)
	case "right", "l":
		m.filesOpen = true
	case "left", "h":
		m.filesOpen, m.cursor = false, 0
	case "space":
		if m.cursor == 0 {
			m.filesOpen = !m.filesOpen
			return m, nil
		}
		f := m.files[m.cursor-1]
		m.selected[f] = !m.selected[f]
		return m, m.selectionChanged()
	case "?":
		m.help = true
	}
	return m, nil
}

func (m reviewModel) partsKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "up", "k":
		m.partCursor = max(m.partCursor-1, 0)
	case "down", "j":
		m.partCursor = min(m.partCursor+1, len(m.parts)-1)
	case "space":
		part := m.parts[m.partCursor]
		for _, f := range m.files {
			m.selected[f] = slices.Contains(part.files, f)
		}
		return m, m.selectionChanged()
	case "?":
		m.help = true
	}
	return m, nil
}

// typeKey moves among the allowed types; Space puts the one under the
// cursor into the title, the rest of it kept.
func (m reviewModel) typeKey(k string) (tea.Model, tea.Cmd) {
	choices := m.plan.TypeChoices
	switch k {
	case "left", "h", "up", "k":
		m.typeCursor = max(m.typeCursor-1, 0)
	case "right", "l", "down", "j":
		m.typeCursor = min(m.typeCursor+1, len(choices)-1)
	case "space":
		if title, ok := m.cfg.conventions.RetypeTitle(strings.TrimSpace(m.inputs[fieldTitle].Value()), choices[m.typeCursor]); ok {
			m.inputs[fieldTitle].SetValue(title)
			m.proposeBranch(false)
		}
	case "?":
		m.help = true
	}
	return m, nil
}

// selectionChanged schedules a new plan once the selection settles.
func (m *reviewModel) selectionChanged() tea.Cmd {
	m.seq++
	if m.selectedCount() == 0 {
		m.planning = false
		return nil
	}
	m.planning = true
	seq := m.seq
	return tea.Tick(m.delay, func(time.Time) tea.Msg { return replanTick{seq: seq} })
}

func (m *reviewModel) applyPlan(p app.PlanView, err error) {
	m.planning, m.planErr = false, err
	if err != nil {
		return
	}
	if m.inputs[fieldTitle].Value() == m.planTitle {
		m.inputs[fieldTitle].SetValue(p.Title)
	}
	untouched := m.inputs[fieldBranch].Value() == m.autoBranch
	m.planTitle, m.planBranch, m.plan = p.Title, p.Branch, p
	m.refusedType, m.typeCursor = "", 0
	if len(p.TypeChoices) > 0 {
		m.refusedType = m.cfg.conventions.TypeOfTitle(p.Title)
	} else if m.focus == fieldType {
		m.focus = fieldFiles
	}
	if untouched {
		m.proposeBranch(true)
	}
}

func (m reviewModel) selectedCount() int {
	n := 0
	for _, f := range m.files {
		if m.selected[f] {
			n++
		}
	}
	return n
}

// only is the --only selection to ship: the command line's when every file
// is selected, otherwise the selected files.
func (m reviewModel) only() []string {
	if m.selectedCount() == len(m.files) {
		return m.cfg.only
	}
	var only []string
	for _, f := range m.files {
		if m.selected[f] {
			only = append(only, f)
		}
	}
	return only
}

// fields lists the fields the screen shows, in Tab order.
func (m reviewModel) fields() []reviewField {
	var fields []reviewField
	if len(m.plan.TypeChoices) > 0 {
		fields = append(fields, fieldType)
	}
	fields = append(fields, fieldTitle)
	if m.plan.CreatesBranch {
		fields = append(fields, fieldBranch)
	}
	fields = append(fields, fieldWhy, fieldTested, fieldFiles)
	if len(m.parts) > 0 {
		fields = append(fields, fieldParts)
	}
	return fields
}

func (m reviewModel) move(delta int) (tea.Model, tea.Cmd) {
	fields := m.fields()
	i := max(slices.Index(fields, m.focus), 0)
	return m.focusOn(fields[(i+delta+len(fields))%len(fields)])
}

func (m reviewModel) focusOn(f reviewField) (tea.Model, tea.Cmd) {
	if isTextField(m.focus) {
		m.inputs[m.focus].focused = false
	}
	m.focus = f
	if isTextField(f) {
		m.inputs[f].focused = true
		m.inputs[f].pos = len(m.inputs[f].value)
	}
	return m, nil
}

// finish ships, or says what stands in the way and goes there.
func (m reviewModel) finish(action reviewAction) (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.inputs[fieldTitle].Value())
	branch := strings.TrimSpace(m.inputs[fieldBranch].Value())
	t := func(key string) string { return i18n.Render(m.cfg.lang, key, nil) }
	switch {
	case m.selectedCount() == 0:
		m.notice = t("cli.review.blocked.files")
		return m.focusOn(fieldFiles)
	case m.planning:
		m.notice = t("cli.review.blocked.planning")
		return m, nil
	case m.planErr != nil:
		m.notice = "✗ " + m.errorText(m.planErr)
		return m, nil
	case len(m.plan.Secrets) > 0:
		m.notice = t("cli.review.blocked.secrets")
		return m.focusOn(fieldFiles)
	case app.TypeAllowed(title, m.plan.TypeChoices) != nil:
		m.notice = t("cli.review.blocked.type")
		return m.focusOn(fieldType)
	case !m.cfg.conventions.CheckTitle(title).Valid():
		m.notice = t("cli.review.blocked.title")
		return m.focusOn(fieldTitle)
	case m.plan.CreatesBranch && !m.cfg.checkBranch(branch).Valid():
		m.notice = t("cli.review.blocked.branch")
		return m.focusOn(fieldBranch)
	}
	if !m.plan.CreatesBranch {
		branch = m.plan.Branch
	}
	return m.quit(reviewOutcome{
		action: action, title: title, branch: branch, only: m.only(),
		why: strings.TrimSpace(m.inputs[fieldWhy].Value()), tested: strings.TrimSpace(m.inputs[fieldTested].Value()),
	})
}

func (m reviewModel) quit(o reviewOutcome) (tea.Model, tea.Cmd) {
	m.done, m.outcome = true, o
	return m, tea.Quit
}

func (m reviewModel) errorText(err error) string {
	if f, ok := err.(*app.Fault); ok {
		return string(f.Code) + "  " + i18n.Render(m.cfg.lang, f.Key, f.Params)
	}
	return err.Error()
}

func (m *reviewModel) resizeInputs() {
	for i := range m.inputs {
		m.inputs[i].width = max(m.width-m.labelWidth()-4, 10)
	}
}

// Styles: colour only ever doubles a written symbol (✓ ✗ ⚠ ›). Bubble Tea
// downsamples them to the terminal's profile, which honours NO_COLOR.
var (
	styleOK      = lipgloss.NewStyle().Foreground(lipgloss.Green)
	styleBad     = lipgloss.NewStyle().Foreground(lipgloss.Red)
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	styleFaint   = lipgloss.NewStyle().Faint(true)
	styleFocused = lipgloss.NewStyle().Bold(true)
)

func (m reviewModel) labelWidth() int {
	w := 0
	labels := []string{"title", "branch", "why", "tested", "files"}
	if len(m.plan.TypeChoices) > 0 {
		labels = append(labels, "type")
	}
	for _, l := range labels {
		w = max(w, lipgloss.Width(i18n.Render(m.cfg.lang, "cli.review.label."+l, nil)))
	}
	return w
}

func (m reviewModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(m.render())
}

// render lays the screen out as text; View wraps it, tests read it.
func (m reviewModel) render() string {
	lang := m.cfg.lang
	t := func(key string, params map[string]string) string { return i18n.Render(lang, key, params) }
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\n") }

	if m.plan.CreatesBranch {
		current := m.plan.CurrentBranch
		if current == "" {
			current = "HEAD"
		}
		line(t("cli.review.header_new", map[string]string{"current": current, "branch": strings.TrimSpace(m.inputs[fieldBranch].Value())}))
	} else {
		line(t("cli.review.header_on", map[string]string{"branch": m.plan.Branch}))
	}
	line("")
	if m.help {
		for _, l := range strings.Split(t("cli.review.help", nil), "\n") {
			line("  " + l)
		}
		return b.String()
	}

	lw := m.labelWidth()
	indent := strings.Repeat(" ", lw+4)
	label := func(f reviewField, name string) string {
		text := t("cli.review.label."+name, nil)
		pad := strings.Repeat(" ", lw-lipgloss.Width(text))
		if m.focus == f {
			return "› " + styleFocused.Render(text) + pad + "  "
		}
		return "  " + text + pad + "  "
	}
	room := m.width - lw - 4

	title := strings.TrimSpace(m.inputs[fieldTitle].Value())
	if choices := m.plan.TypeChoices; len(choices) > 0 {
		current := m.cfg.conventions.TypeOfTitle(title)
		var options []string
		for i, c := range choices {
			mark := "( )"
			if c == current {
				mark = "(•)"
			}
			if m.focus == fieldType && m.typeCursor == i {
				mark = "›" + mark
			}
			options = append(options, mark+" "+c)
		}
		line(label(fieldType, "type") + fit(strings.Join(options, "  "), room))
		if app.TypeAllowed(title, choices) != nil {
			line(indent + styleBad.Render(fit("✗ "+t("cli.review.type_refused", map[string]string{"type": current}), room)))
		} else {
			line(indent + styleOK.Render(t("cli.review.type_allowed", nil)))
		}
	}
	line(label(fieldTitle, "title") + m.inputs[fieldTitle].view())
	if check := m.cfg.conventions.CheckTitle(title); check.Valid() {
		line(indent + styleOK.Render(t("cli.review.title_valid", map[string]string{
			"length": strconv.Itoa(utf8.RuneCountInString(title)), "max": strconv.Itoa(m.cfg.conventions.MaxTitleLength()),
		})))
	} else {
		f := check.Faults[0]
		line(indent + styleBad.Render(fit("✗ "+t(f.Key, f.Params), room)))
	}

	if m.plan.CreatesBranch {
		line(label(fieldBranch, "branch") + m.inputs[fieldBranch].view())
		if check := m.cfg.checkBranch(strings.TrimSpace(m.inputs[fieldBranch].Value())); check.Valid() {
			line(indent + styleOK.Render(t("cli.review.branch_valid", nil)))
		} else {
			line(indent + styleBad.Render(fit("✗ "+t(check.Fault.Key, check.Fault.Params), room)))
		}
	} else {
		line(label(fieldBranch, "branch") + m.plan.Branch + styleFaint.Render("  ("+t("cli.review.branch_stays", nil)+")"))
	}
	line(label(fieldWhy, "why") + m.inputs[fieldWhy].view())
	line(label(fieldTested, "tested") + m.inputs[fieldTested].view())
	line("")

	m.renderFiles(line, label(fieldFiles, "files"), indent, room)
	m.renderWarnings(line)

	line("")
	if m.notice != "" {
		line(m.notice)
	}
	keys := "cli.review.keys"
	if isTextField(m.focus) {
		keys = "cli.review.keys_text"
	}
	line(styleFaint.Render(t(keys, nil)))
	return b.String()
}

func (m reviewModel) renderFiles(line func(string), label, indent string, room int) {
	t := func(key string, params map[string]string) string { return i18n.Render(m.cfg.lang, key, params) }
	fold := "▸"
	if m.filesOpen {
		fold = "▾"
	}
	head := label + fold + " " + t("cli.review.files_summary", map[string]string{
		"selected": strconv.Itoa(m.selectedCount()), "total": strconv.Itoa(len(m.files)),
	})
	if m.focus == fieldFiles && m.cursor == 0 {
		head = strings.Replace(head, fold, styleFocused.Render(fold), 1)
	}
	if m.planning {
		head += "  " + styleFaint.Render(t("cli.review.replanning", nil))
	}
	line(head)
	if m.selectedCount() == 0 {
		line(indent + styleBad.Render(t("cli.review.blocked.files", nil)))
	}
	if !m.filesOpen {
		return
	}
	// A window of maxVisibleFiles rows that follows the cursor.
	first := 0
	if m.cursor > maxVisibleFiles {
		first = m.cursor - maxVisibleFiles
	}
	last := min(first+maxVisibleFiles, len(m.files))
	for i := first; i < last; i++ {
		box := "[ ]"
		if m.selected[m.files[i]] {
			box = "[x]"
		}
		pointer := "  "
		if m.focus == fieldFiles && m.cursor == i+1 {
			pointer = "› "
		}
		line(indent[:len(indent)-2] + pointer + box + " " + fitLeft(m.files[i], room-4))
	}
	if hidden := len(m.files) - last + first; hidden > 0 {
		line(indent + styleFaint.Render(t("cli.review.files_hidden", map[string]string{"count": strconv.Itoa(hidden)})))
	}
}

func (m reviewModel) renderWarnings(line func(string)) {
	t := func(key string, params map[string]string) string { return i18n.Render(m.cfg.lang, key, params) }
	if m.planErr != nil {
		line("")
		line(styleBad.Render(fit("✗ "+m.errorText(m.planErr), m.width)))
	}
	if len(m.plan.Secrets) > 0 {
		line("")
		line(styleBad.Render(t("cli.review.secrets", nil)))
		for _, s := range m.plan.Secrets {
			line("    " + fit(secretLine(m.cfg.lang, s), m.width-4))
		}
	}
	if len(m.parts) == 0 {
		return
	}
	line("")
	if size := m.plan.Size; size != nil {
		line(styleWarn.Render(fit(t("cli.review.too_large", map[string]string{
			"lines": strconv.Itoa(size.Lines), "max": strconv.Itoa(size.Max),
		}), m.width)))
	} else {
		// The selection is small enough now; the parts stay to change it.
		line(t("cli.review.parts", nil))
	}
	for i, p := range m.parts {
		mark := "( )"
		if m.isPart(p) {
			mark = "(•)"
		}
		pointer := "  "
		if m.focus == fieldParts && m.partCursor == i {
			pointer = "› "
		}
		detail := t("cli.plan.group_lines", map[string]string{"lines": strconv.Itoa(p.lines), "files": strconv.Itoa(p.count)})
		line("  " + pointer + mark + " " + padRight(fitLeft(p.path, 40), 40) + " " + detail)
	}
}

// isPart reports whether the selection is exactly this part.
func (m reviewModel) isPart(p partView) bool {
	for _, f := range m.files {
		if m.selected[f] != slices.Contains(p.files, f) {
			return false
		}
	}
	return true
}

// secretLine renders a finding as the CLI lists it: path:line, rule and a
// masked preview.
func secretLine(lang i18n.Language, s app.SecretView) string {
	params := map[string]string{
		"path": s.Path, "line": strconv.Itoa(s.Line), "preview": s.Preview, "length": strconv.Itoa(s.Length),
		"rule": ruleName(lang, s.Rule),
	}
	key := "cli.secrets.line"
	if s.Line == 0 {
		key = "cli.secrets.file"
	}
	return i18n.Render(lang, key, params)
}

// ruleName names a rule: a built-in one in lang, a custom one
// (ship.secrets.rules) by its id.
func ruleName(lang i18n.Language, id string) string {
	key := "secrets.rule." + id
	if name := i18n.Render(lang, key, nil); name != key {
		return name
	}
	return id
}

// fit cuts s to width cells, ending with "…".
func fit(s string, width int) string {
	if lipgloss.Width(s) <= width || width < 1 {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// fitLeft cuts the start of a path to width cells: the file name stays.
func fitLeft(s string, width int) string {
	if lipgloss.Width(s) <= width || width < 1 {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[1:]
	}
	return "…" + string(r)
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
