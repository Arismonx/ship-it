// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package plan

import (
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// lines returns n added lines with the given text.
func lines(n int, text string) []snapshot.Line {
	out := make([]snapshot.Line, n)
	for i := range out {
		out[i] = snapshot.Line{Number: i + 1, Text: text}
	}
	return out
}

func file(p string, status snapshot.ChangeStatus, added []snapshot.Line, removed int) snapshot.FileDiff {
	return snapshot.FileDiff{Path: p, Status: status, Added: added, Removed: make([]string, removed)}
}

func propose(t *testing.T, diffs ...snapshot.FileDiff) Plan {
	t.Helper()
	p, err := Propose(snapshot.New(diffs, snapshot.Options{}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if faults := convention.Validate(p.Header); len(faults) != 0 {
		t.Fatalf("plan header %q is invalid: %v", convention.Format(p.Header, convention.GitmojiFirst), faults)
	}
	if err := convention.ValidateBranch(p.Branch); err != nil {
		t.Fatalf("plan branch %q is invalid: %v", p.Branch, err)
	}
	return p
}

func title(p Plan) string { return convention.Format(p.Header, convention.GitmojiFirst) }

func TestProposeTitles(t *testing.T) {
	tests := []struct {
		name  string
		diffs []snapshot.FileDiff
		want  string
	}{
		{"new source file is a feature",
			[]snapshot.FileDiff{file("internal/core/plan/plan.go", snapshot.Added, lines(40, "x := 1"), 0)},
			"✨ feat(plan): add plan"},
		{"fix vocabulary in a small edit",
			[]snapshot.FileDiff{file("internal/app/run.go", snapshot.Modified, lines(2, "// fix: avoid nil config"), 1)},
			"🐛 fix: update run"},
		{"a new function in an existing file is a feature",
			[]snapshot.FileDiff{file("cart/cart.go", snapshot.Modified, lines(1, "func Sub(a, b int) int {"), 0)},
			"✨ feat(cart): add Sub"},
		{"a changed signature is not a new declaration",
			[]snapshot.FileDiff{{Path: "cart/cart.go", Status: snapshot.Modified,
				Added:   []snapshot.Line{{Number: 3, Text: "func Total(items []Item, discount int) int {"}},
				Removed: []string{"func Total(items []Item) int {"}}},
			"🐛 fix(cart): update cart"},
		{"two new declarations keep the file name",
			[]snapshot.FileDiff{file("cart/cart.go", snapshot.Modified, []snapshot.Line{
				{Number: 1, Text: "func Sub(a, b int) int {"}, {Number: 5, Text: "func Add(a, b int) int {"}}, 0)},
			"✨ feat(cart): update cart"},
		{"deletions dominate: refactor",
			[]snapshot.FileDiff{file("internal/app/run.go", snapshot.Modified, lines(5, "y"), 40)},
			"♻️ refactor: update run"},
		{"docs only",
			[]snapshot.FileDiff{file("docs/guide.md", snapshot.Modified, lines(10, "text"), 2)},
			"📝 docs: update guide docs"},
		{"README stands alone",
			[]snapshot.FileDiff{file("README.md", snapshot.Modified, lines(3, "text"), 1)},
			"📝 docs: update README"},
		{"tests only",
			[]snapshot.FileDiff{file("internal/core/plan/plan_test.go", snapshot.Added, lines(30, "t.Run"), 0)},
			"✅ test(plan): add plan tests"},
		{"lockfile never drives the title",
			[]snapshot.FileDiff{
				file("pnpm-lock.yaml", snapshot.Modified, lines(3000, "x"), 2000),
				file(".github/workflows/ci.yml", snapshot.Modified, lines(4, "run: go test"), 1)},
			"👷 ci: update ci workflow"},
		{"dependencies",
			[]snapshot.FileDiff{file("package.json", snapshot.Modified, lines(1, `"vue": "3.6.0"`), 1)},
			"📦️ build: update dependencies"},
		{"many source files are counted",
			[]snapshot.FileDiff{
				file("internal/core/plan/a.go", snapshot.Modified, lines(30, "a"), 28),
				file("internal/core/plan/b.go", snapshot.Modified, lines(30, "b"), 28),
				file("internal/core/plan/c.go", snapshot.Modified, lines(30, "c"), 28)},
			"♻️ refactor(plan): update 3 files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := title(propose(t, tt.diffs...)); got != tt.want {
				t.Errorf("title = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProposeBranchAndReasons(t *testing.T) {
	p, err := Propose(snapshot.New([]snapshot.FileDiff{
		file("internal/core/plan/plan.go", snapshot.Added, lines(40, "x"), 0),
	}, snapshot.Options{}), Options{Issue: 42})
	if err != nil {
		t.Fatal(err)
	}
	if p.Branch != "feat/42-add-plan" {
		t.Errorf("branch = %q", p.Branch)
	}
	if len(p.Reasons) < 2 || p.Reasons[0].Key != "plan.reason.dominant_category" {
		t.Errorf("reasons = %+v", p.Reasons)
	}
	if len(p.Files) != 1 {
		t.Errorf("files = %v", p.Files)
	}
}

// TestOneNewDeclarationNamesTheBranch: the subject names the declaration,
// the branch keeps the branch rules (kebab case).
func TestOneNewDeclarationNamesTheBranch(t *testing.T) {
	p := propose(t, file("cart/cart.go", snapshot.Modified, lines(1, "func ApplyDiscount(c Cart) Cart {"), 0))
	if p.Header.Subject != "add ApplyDiscount" || p.Branch != "feat/add-apply-discount" {
		t.Errorf("subject %q, branch %q", p.Header.Subject, p.Branch)
	}
}

func TestProposeEmpty(t *testing.T) {
	_, err := Propose(snapshot.Snapshot{}, Options{})
	if !errors.Is(err, &fault.Fault{Code: fault.NothingToShip}) {
		t.Fatalf("error = %v", err)
	}
}

func TestSubjectStaysWithinLimits(t *testing.T) {
	long := strings.Repeat("very-long-module-name-", 5)
	p := propose(t,
		file("src/"+long+"one.go", snapshot.Modified, lines(5, "x"), 5),
		file("src/"+long+"two.go", snapshot.Modified, lines(5, "x"), 5))
	if n := len([]rune(p.Header.Subject)); n > convention.DefaultSubjectMaxLength {
		t.Fatalf("subject %q has %d characters", p.Header.Subject, n)
	}
}

func TestKebab(t *testing.T) {
	cases := map[string]string{"parseDiff": "parse-diff", "parse_diff": "parse-diff", "HTTPServer": "httpserver", "v2": "v2"}
	for in, want := range cases {
		if got := kebab(in); got != want {
			t.Errorf("kebab(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSubjectNamesOnlyDominantStatus(t *testing.T) {
	p := propose(t,
		file("src/auth/token.go", snapshot.Added, lines(30, "x"), 0),
		file("src/auth/session.go", snapshot.Modified, lines(2, "y"), 1))
	if got := title(p); got != "✨ feat(auth): add token" {
		t.Fatalf("title = %q", got)
	}
}

func TestProposeFollowsTheConventions(t *testing.T) {
	str := func(s string) *string { return &s }
	source := file("internal/core/plan/plan.go", snapshot.Added, lines(40, "x := 1"), 0)
	tests := []struct {
		name    string
		opts    Options
		title   string
		branch  string
		reasons []string
	}{
		{"built-in", Options{Issue: 7}, "✨ feat(plan): add plan", "feat/7-add-plan", nil},
		{"no scope from folders", Options{NoScope: true}, "✨ feat: add plan", "feat/add-plan", nil},
		{"scope not listed", Options{Rules: convention.Rules{Scopes: []string{"api"}}}, "✨ feat: add plan", "feat/add-plan",
			[]string{"plan.reason.scope_not_allowed"}},
		{"scope listed", Options{Rules: convention.Rules{Scopes: []string{"plan"}}}, "✨ feat(plan): add plan", "feat/add-plan", nil},
		{"emoji and prefix of the table", Options{Rules: convention.Rules{Types: convention.WithTypes([]convention.TypeChange{
			{Type: "feat", Gitmoji: str("🚀"), BranchPrefix: str("feature")}})}}, "🚀 feat(plan): add plan", "feature/add-plan", nil},
		{"type removed", Options{Rules: convention.Rules{Types: convention.WithTypes([]convention.TypeChange{{Type: "feat", Remove: true}})}},
			"🐛 fix(plan): add plan", "fix/add-plan", []string{"plan.reason.type_removed"}},
		{"issue left out", Options{Issue: 7, Rules: convention.Rules{BranchWithoutIssue: true}}, "✨ feat(plan): add plan", "feat/add-plan", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Propose(snapshot.New([]snapshot.FileDiff{source}, snapshot.Options{}), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := tt.opts.Rules.Format(p.Header, convention.GitmojiFirst); got != tt.title {
				t.Errorf("title = %q, want %q", got, tt.title)
			}
			if p.Branch != tt.branch {
				t.Errorf("branch = %q, want %q", p.Branch, tt.branch)
			}
			for _, key := range tt.reasons {
				found := false
				for _, r := range p.Reasons {
					found = found || r.Key == key
				}
				if !found {
					t.Errorf("no reason %s in %v", key, p.Reasons)
				}
			}
			if faults := tt.opts.Rules.Validate(p.Header); len(faults) != 0 {
				t.Errorf("the plan breaks its own conventions: %v", faults)
			}
		})
	}
}

func TestProposeSubjectFitsAShorterLimit(t *testing.T) {
	rules := convention.Rules{SubjectMaxLength: 20}
	p, err := Propose(snapshot.New([]snapshot.FileDiff{
		file("internal/app/configuration_loader.go", snapshot.Modified, lines(3, "x"), 1),
		file("internal/app/environment_reader.go", snapshot.Modified, lines(3, "x"), 1),
	}, snapshot.Options{}), Options{Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(p.Header.Subject)); n > 20 {
		t.Fatalf("subject %q has %d characters", p.Header.Subject, n)
	}
}
