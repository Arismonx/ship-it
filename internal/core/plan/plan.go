// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package plan

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// Reason explains one decision of the planner; Key is an i18n key.
type Reason struct {
	Key    string
	Params map[string]string
}

// Plan is what ship-it proposes before acting.
type Plan struct {
	Header  convention.Header
	Branch  string
	Files   []string
	Reasons []Reason
}

// Options carries the context the snapshot does not contain.
type Options struct {
	Issue int
	Agent convention.AgentPrefix
	// Rules are the repository's conventions: types, scopes, limits.
	Rules convention.Rules
	// NoScope leaves the scope to the human (ship.commits.scopeFrom "none")
	// instead of taking it from the changed folders.
	NoScope bool
}

// Propose builds the deterministic plan for a snapshot (ADR-0004: the plan
// never depends on AI; providers may only refine it).
func Propose(s snapshot.Snapshot, opts Options) (Plan, error) {
	if len(s.Files) == 0 {
		return Plan{}, fault.New(fault.NothingToShip, "plan.nothing_to_ship")
	}
	dominant, main := dominantCategory(s.Files)
	reasons := []Reason{{Key: "plan.reason.dominant_category", Params: map[string]string{
		"category": categoryToken(dominant), "count": strconv.Itoa(len(main)), "total": strconv.Itoa(len(s.Files)),
	}}}

	commitType := typeForCategory[dominant]
	if dominant == snapshot.SourceCode {
		var codeReasons []Reason
		commitType, codeReasons = sourceCodeType(main, s.Files)
		reasons = append(reasons, codeReasons...)
	}
	rules := opts.Rules
	if closest, replaced := rules.Closest(commitType); replaced {
		// The repository removed the type (ship.commits.types): the closest
		// one it keeps.
		reasons = append(reasons, Reason{Key: "plan.reason.type_removed", Params: map[string]string{
			"from": string(commitType), "to": string(closest),
		}})
		commitType = closest
	}
	scope := ""
	if !opts.NoScope {
		scope = commonScope(main)
	}
	if scope != "" && !rules.ScopeAllowed(scope) {
		reasons = append(reasons, Reason{Key: "plan.reason.scope_not_allowed", Params: map[string]string{"scope": scope}})
		scope = ""
	}

	// The subject only names the files of the dominant status, so that
	// "add" never describes a modified file (rule from the nuxt-core draft).
	status := dominantStatus(main)
	verb := verbs[status]
	subject := verb + " " + describe(withStatus(main, status), rules.SubjectMax()-len(verb)-1)
	branchSubject := "" // the subject for the branch name, when it differs
	// One new declaration in existing code is what the change is about:
	// "add Discount" says more than "update cart".
	if declared := newDeclarations(main); commitType == convention.Feature && len(declared) == 1 && declared[0] != "" &&
		len(withStatus(main, snapshot.Added)) == 0 && len("add "+declared[0]) <= rules.SubjectMax() {
		subject = "add " + declared[0]
		branchSubject = "add " + kebab(declared[0])
	}
	header := rules.FitToMaxLength(convention.Header{
		Type:    commitType,
		Scope:   scope,
		Subject: subject,
	})

	if branchSubject == "" || header.Subject != subject {
		branchSubject = header.Subject
	}
	files := make([]string, 0, len(s.Files))
	for _, f := range s.Files {
		files = append(files, f.Path)
	}
	return Plan{
		Header:  header,
		Branch:  rules.BranchName(header.Type, branchSubject, convention.BranchOptions{Issue: opts.Issue, Agent: opts.Agent}),
		Files:   files,
		Reasons: reasons,
	}, nil
}

var typeForCategory = map[snapshot.Category]convention.CommitType{
	snapshot.Tests:                 convention.Testing,
	snapshot.Documentation:         convention.Documentation,
	snapshot.Stylesheets:           convention.Styling,
	snapshot.ContinuousIntegration: convention.ContinuousIntegration,
	snapshot.Dependencies:          convention.BuildSystem,
	snapshot.Configuration:         convention.Maintenance,
	snapshot.Assets:                convention.Maintenance,
}

// weight measures a file's importance for the title. Generated files weigh
// nothing; prose counts half, since documentation accumulates lines quickly
// (both rules come from the nuxt-core draft).
func weight(f snapshot.File) int {
	if f.Generated {
		return 0
	}
	w := 2 * (f.AddedCount() + f.RemovedCount() + 1)
	if f.Category == snapshot.Documentation {
		w /= 2
	}
	return w
}

// dominantCategory picks source code whenever a non-generated source file
// changed, otherwise the heaviest category; ties keep the category order.
func dominantCategory(files []snapshot.File) (snapshot.Category, []snapshot.File) {
	byCategory := map[snapshot.Category][]snapshot.File{}
	weights := map[snapshot.Category]int{}
	for _, f := range files {
		byCategory[f.Category] = append(byCategory[f.Category], f)
		weights[f.Category] += weight(f)
	}
	for _, f := range files {
		if f.Category == snapshot.SourceCode && !f.Generated {
			return snapshot.SourceCode, byCategory[snapshot.SourceCode]
		}
	}
	best, bestWeight := snapshot.Category(-1), -1
	for c := snapshot.SourceCode; c <= snapshot.Assets; c++ {
		if _, present := byCategory[c]; present && weights[c] > bestWeight {
			best, bestWeight = c, weights[c]
		}
	}
	return best, byCategory[best]
}

// newDefinition matches a line that declares something new: a function, a
// method, a type or an exported value, in the common languages. Adding one is
// the usual shape of a feature, not of a fix.
var newDefinition = regexp.MustCompile(`^\s*(func\s|def\s|class\s|fn\s|pub\s+(fn|struct|enum|trait)\s|type\s+\w+\s+(struct|interface)|export\s+(default\s+)?(async\s+)?(function|class|const|interface|type)\s|(public|protected)\s+[\w<>\[\], ]+\s+\w+\s*\()`)

// declarationName captures the name a declaration line declares, so that a
// line rewritten in place (its signature changed) is not taken for a new one.
var declarationName = regexp.MustCompile(`^\s*(?:(?:public|protected)\s+[\w<>\[\], ]+\s+(\w+)\s*\(|(?:func\s+(?:\([^)]*\)\s*)?|def\s+|class\s+|fn\s+|pub\s+(?:fn|struct|enum|trait)\s+|type\s+|export\s+(?:default\s+)?(?:async\s+)?(?:function|class|const|interface|type)\s+)(\w+))`)

// DeclaredName returns the name a line of code declares (a function, a type,
// a class…), or "" when it declares nothing that can be named.
func DeclaredName(line string) string { return nameOf(line) }

func nameOf(line string) string {
	if !newDefinition.MatchString(line) {
		return ""
	}
	m := declarationName.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[1] + m[2]
}

// newDeclarations returns the names declared by added lines of existing
// files that no removed line of the same file declared: a function whose
// signature changed is modified, not new. Names come once, in order.
func newDeclarations(code []snapshot.File) []string {
	var names []string
	seen := map[string]bool{}
	for _, f := range code {
		if f.Status == snapshot.Added {
			continue
		}
		before := map[string]bool{}
		for _, l := range f.Removed {
			if name := nameOf(l); name != "" {
				before[name] = true
			}
		}
		for _, l := range f.Added {
			name := nameOf(l.Text)
			if !newDefinition.MatchString(l.Text) || before[name] {
				continue
			}
			key := f.Path + "\x00" + name
			if name == "" {
				key = f.Path + "\x00" + strconv.Itoa(l.Number) // a declaration without a name we read
			}
			if !seen[key] {
				seen[key] = true
				names = append(names, name)
			}
		}
	}
	return names
}

var fixVocabulary = regexp.MustCompile(`(?i)\b(fix(es|ed)?|bug|regression|workaround|hotfix|prevent(s|ed)?|avoid(s|ed)?|crash(es|ed)?)\b`)

// sourceCodeType scores feat, fix and refactor from signals in the diff.
// Ties favour feat, then fix; no signal at all means fix (an edit to
// existing code), as in the nuxt-core draft.
func sourceCodeType(code, all []snapshot.File) (convention.CommitType, []Reason) {
	scores := map[convention.CommitType]int{}
	var reasons []Reason
	signal := func(t convention.CommitType, points int, key string, kv ...string) {
		if points <= 0 {
			return
		}
		scores[t] += points
		r := Reason{Key: key, Params: map[string]string{}}
		for i := 0; i+1 < len(kv); i += 2 {
			r.Params[kv[i]] = kv[i+1]
		}
		reasons = append(reasons, r)
	}

	added, removed, newFiles, renames, fixLines := 0, 0, 0, 0, 0
	for _, f := range code {
		added += f.AddedCount()
		removed += f.RemovedCount()
		switch f.Status {
		case snapshot.Added:
			newFiles++
		case snapshot.Renamed:
			renames++
		}
		for _, l := range f.Added {
			if fixVocabulary.MatchString(l.Text) {
				fixLines++
			}
		}
	}
	definitions := len(newDeclarations(code))
	newTests := false
	for _, f := range all {
		if f.Category == snapshot.Tests && f.Status == snapshot.Added {
			newTests = true
		}
	}

	signal(convention.Feature, 3*newFiles, "plan.reason.new_files", "count", strconv.Itoa(newFiles))
	if added > 20 && added > 2*removed {
		signal(convention.Feature, 1, "plan.reason.mostly_additions")
	}
	signal(convention.Feature, 2*min(definitions, 2), "plan.reason.new_definitions", "count", strconv.Itoa(definitions))
	signal(convention.BugFix, 2*min(fixLines, 3), "plan.reason.fix_vocabulary", "count", strconv.Itoa(fixLines))
	if newFiles == 0 && added+removed <= 20 {
		signal(convention.BugFix, 1, "plan.reason.small_change")
	}
	if newFiles == 0 && newTests {
		signal(convention.BugFix, 1, "plan.reason.test_without_code")
	}
	if removed > added {
		signal(convention.Refactoring, 2, "plan.reason.more_deletions")
	}
	signal(convention.Refactoring, 3*min(renames, 1), "plan.reason.files_moved", "count", strconv.Itoa(renames))
	if total := added + removed; newFiles == 0 && total > 20 && abs(added-removed)*4 < total {
		signal(convention.Refactoring, 2, "plan.reason.balanced_restructure")
	}

	best := convention.BugFix
	bestScore := 0
	for _, t := range []convention.CommitType{convention.Feature, convention.BugFix, convention.Refactoring} {
		if scores[t] > bestScore {
			best, bestScore = t, scores[t]
		}
	}
	if bestScore == 0 {
		reasons = append(reasons, Reason{Key: "plan.reason.edit_existing_code"})
	}
	return best, reasons
}

// dominantStatus picks the change status that weighs the most.
func dominantStatus(files []snapshot.File) snapshot.ChangeStatus {
	weights := map[snapshot.ChangeStatus]int{}
	for _, f := range files {
		weights[f.Status] += weight(f) + 1
	}
	best, bestWeight := snapshot.Modified, -1
	for _, s := range []snapshot.ChangeStatus{snapshot.Added, snapshot.Modified, snapshot.Deleted, snapshot.Renamed} {
		if weights[s] > bestWeight {
			best, bestWeight = s, weights[s]
		}
	}
	return best
}

// withStatus keeps the files that have the given status.
func withStatus(files []snapshot.File, status snapshot.ChangeStatus) []snapshot.File {
	var kept []snapshot.File
	for _, f := range files {
		if f.Status == status {
			kept = append(kept, f)
		}
	}
	return kept
}

var verbs = map[snapshot.ChangeStatus]string{
	snapshot.Added: "add", snapshot.Modified: "update", snapshot.Deleted: "remove", snapshot.Renamed: "move",
}

// nouns name groups of files in subjects; an empty noun means the label
// stands alone (source files are named after themselves).
var nouns = map[snapshot.Category]string{
	snapshot.Tests: "tests", snapshot.Documentation: "docs", snapshot.Stylesheets: "styles",
	snapshot.ContinuousIntegration: "workflow", snapshot.Assets: "assets",
}

// describe names the files of the dominant category within max characters:
// "parser", "parser and lexer", "3 files", "ci workflow", "README".
func describe(files []snapshot.File, max int) string {
	sorted := append([]snapshot.File(nil), files...)
	sort.SliceStable(sorted, func(i, j int) bool { return weight(sorted[i]) > weight(sorted[j]) })
	labels := make([]string, 0, len(sorted))
	seen := map[string]bool{}
	for _, f := range sorted {
		label := fileLabel(f)
		if !seen[label] {
			seen[label] = true
			labels = append(labels, label)
		}
	}
	category := sorted[0].Category
	if category == snapshot.Dependencies {
		return "dependencies"
	}
	noun := nouns[category]
	count := strconv.Itoa(len(labels)) + " " + orDefault(noun, "files")
	if len(labels) >= 3 {
		return count
	}
	phrase := strings.Join(labels, " and ")
	if noun != "" && !(len(labels) == 1 && labels[0] == readmeLabel) {
		phrase += " " + noun
	}
	if len(phrase) > max {
		return count
	}
	return phrase
}

// readmeLabel stands alone in subjects: "update README", not "update README docs".
const readmeLabel = "README"

// fileLabel is the readable name of a file: "README", "ci", "parse-diff".
func fileLabel(f snapshot.File) string {
	base := path.Base(f.Path)
	if strings.EqualFold(base, "README.md") {
		return readmeLabel
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	stem = strings.TrimSuffix(strings.TrimSuffix(stem, "_test"), ".test")
	stem = strings.TrimSuffix(stem, ".spec")
	return kebab(stem)
}

// genericDirectories do not make useful scopes.
var genericDirectories = map[string]bool{
	"": true, ".": true, "internal": true, "src": true, "lib": true, "pkg": true, "cmd": true,
	"app": true, "source": true, "docs": true, ".github": true, "workflows": true, "test": true,
	"tests": true, "testdata": true, "assets": true, "public": true,
}

// commonScope returns the last segment of the deepest directory shared by the
// files, when it is specific enough and a valid scope.
func commonScope(files []snapshot.File) string {
	dir := path.Dir(files[0].Path)
	for _, f := range files[1:] {
		for dir != "." && !strings.HasPrefix(path.Dir(f.Path)+"/", dir+"/") {
			dir = path.Dir(dir)
		}
	}
	scope := kebab(path.Base(dir))
	if genericDirectories[scope] {
		return ""
	}
	return scope
}

// kebab converts "parseDiff" or "parse_diff" to "parse-diff".
func kebab(s string) string {
	var b strings.Builder
	prevLower := false
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			if prevLower {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
			prevLower = false
		case r == '_' || r == ' ' || r == '.':
			b.WriteByte('-')
			prevLower = false
		default:
			b.WriteRune(r)
			prevLower = unicode.IsLower(r) || unicode.IsDigit(r)
		}
	}
	return strings.Trim(b.String(), "-")
}

func categoryToken(c snapshot.Category) string {
	return [...]string{"source_code", "tests", "documentation", "stylesheets", "continuous_integration",
		"dependencies", "configuration", "assets"}[c]
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
