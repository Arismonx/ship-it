// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

func header(t *testing.T, s string) convention.Header {
	t.Helper()
	h, err := convention.ParseHeader(s)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func commit(t *testing.T, message string) Commit {
	t.Helper()
	subject, body := ParseMessage(message)
	return Commit{Header: header(t, subject), Body: body}
}

func TestBodyOfOneCommit(t *testing.T) {
	got := Body(BodyInput{
		Issue: 42, IssueTitle: "Cart crashes on an empty basket",
		Commits: []Commit{commit(t, "fix(cart): handle an empty basket\n\nThe total divided by the item count.\n\nCloses #42\nSigned-off-by: Ada <ada@example.invalid>")},
		Checks:  []string{"npm run lint", "npm test"},
	})
	want := "Closes #42 (Cart crashes on an empty basket)\n\n" +
		"The total divided by the item count.\n\n" +
		"### Testing\n\n`npm run lint` and `npm test` pass locally.\n\n" +
		Marker + "\n"
	if got != want {
		t.Fatalf("Body =\n%s\nwant\n%s", got, want)
	}
}

func TestBodyOfSeveralCommits(t *testing.T) {
	got := Body(BodyInput{Commits: []Commit{
		commit(t, "feat(api)!: drop v1 routes"),
		commit(t, "docs: explain the v2 migration"),
	}, AIAssistant: "host agent"})
	want := "- Drop v1 routes\n- Explain the v2 migration\n\n" +
		"### Notes\n\n**Breaking change:** Drop v1 routes.\n\n" +
		"The commit message was drafted with AI assistance (host agent).\n\n" + Marker + "\n"
	if got != want {
		t.Fatalf("Body =\n%s\nwant\n%s", got, want)
	}
}

func TestBodyHasNoEmptySection(t *testing.T) {
	got := Body(BodyInput{Commits: []Commit{commit(t, "fix: handle nil")}})
	if got != Marker+"\n" {
		t.Fatalf("a one-line commit needs no body beyond its title:\n%q", got)
	}
}

func TestBodySaysWhenChecksWereSkipped(t *testing.T) {
	got := Body(BodyInput{Commits: []Commit{commit(t, "fix: nil")}, ChecksSkipped: true, Checks: []string{"npm test"}})
	if !strings.Contains(got, "### Testing\n\nLocal checks were skipped (`--skip-gates`)") || strings.Contains(got, "pass locally") {
		t.Fatalf("body:\n%s", got)
	}
}

func TestParseMessage(t *testing.T) {
	for _, tt := range []struct{ message, subject, body string }{
		{"feat: x", "feat: x", ""},
		{"feat: x\r\n\r\nWhy it matters.\r\n", "feat: x", "Why it matters."},
		{"fix: y\n\nFirst.\n\nSecond.\n\nRefs: #3\nCo-authored-by: Bob <b@example.invalid>", "fix: y", "First.\n\nSecond."},
		{"fix: z\n\nFixes #4, acme/shop#5\nKeep this line.", "fix: z", "Keep this line."},
		// A paragraph that only looks like prose with a colon is not a trailer block.
		{"docs: w\n\nNote: the API changed and callers must update.\nSee below.", "docs: w", "Note: the API changed and callers must update.\nSee below."},
	} {
		subject, body := ParseMessage(tt.message)
		if subject != tt.subject || body != tt.body {
			t.Errorf("ParseMessage(%q) = %q, %q; want %q, %q", tt.message, subject, body, tt.subject, tt.body)
		}
	}
}

func TestSentence(t *testing.T) {
	for in, want := range map[string]string{
		"handle an empty cart": "Handle an empty cart",
		"iOS build fails":      "iOS build fails",
		"npm scripts":          "npm scripts",
		"parseHeader accepts":  "parseHeader accepts",
		"go.mod tidy":          "go.mod tidy",
		"Already a sentence":   "Already a sentence",
		"":                     "",
	} {
		if got := Sentence(in); got != want {
			t.Errorf("Sentence(%q) = %q, want %q", in, got, want)
		}
	}
}

// reactTemplate is facebook/react's template, shortened (MIT).
const reactTemplate = `<!--
  Thanks for submitting a pull request!
-->

## Summary

<!--
 Explain the **motivation** for making this change.
-->

## How did you test this change?

<!--
  Demonstrate the code is solid.
-->
`

func TestFillTemplate(t *testing.T) {
	s := Sections{Issue: "Closes #7", Summary: "The cart crashed.", Testing: "`npm test` passes locally."}
	got := FillTemplate(reactTemplate, s)
	want := "Closes #7\n\n<!--\n  Thanks for submitting a pull request!\n-->\n\n" +
		"## Summary\n\n<!--\n Explain the **motivation** for making this change.\n-->\n\nThe cart crashed.\n\n" +
		"## How did you test this change?\n\n<!--\n  Demonstrate the code is solid.\n-->\n\n`npm test` passes locally.\n\n" + Marker + "\n"
	if got != want {
		t.Fatalf("FillTemplate =\n%s\nwant\n%s", got, want)
	}
}

// kubernetesTemplate follows kubernetes/kubernetes' section headings (Apache-2.0).
const kubernetesTemplate = `#### What type of PR is this?

<!-- /kind bug -->

#### What this PR does / why we need it:

#### Which issue(s) this PR is related to:

<!-- Fixes #<issue number> -->

#### Special notes for your reviewer:

#### Does this PR introduce a user-facing change?

` + "```release-note\n\n```\n"

func TestFillTemplateLeavesTheAuthorsSections(t *testing.T) {
	got := FillTemplate(kubernetesTemplate, Sections{Issue: "Fixes #9", Summary: "Why.", Notes: "**Breaking change:** x."})
	for _, want := range []string{
		"#### What type of PR is this?\n\n<!-- /kind bug -->\n\n#### What",
		"#### What this PR does / why we need it:\n\nWhy.\n\n#### Which",
		"#### Which issue(s) this PR is related to:\n\n<!-- Fixes #<issue number> -->\n\nFixes #9\n\n#### Special",
		"#### Does this PR introduce a user-facing change?\n\n```release-note\n\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "Breaking change") {
		t.Error("a template without a place for notes keeps its own shape")
	}
}

func TestFillTemplateRespectsWrittenContentAndComments(t *testing.T) {
	tpl := "<!--\n## Summary\n-->\n## Description\n\nN/A\n\n## Testing\n"
	got := FillTemplate(tpl, Sections{Summary: "Why.", Testing: "Ran it."})
	if !strings.HasPrefix(got, "Why.\n\n<!--\n## Summary\n-->\n## Description\n\nN/A") {
		t.Errorf("summary placed first when its section is taken:\n%s", got)
	}
	if !strings.Contains(got, "## Testing\n\nRan it.") {
		t.Errorf("testing:\n%s", got)
	}
}
func TestDraft(t *testing.T) {
	feat, wip := header(t, "feat: x"), header(t, "wip: y")
	if Draft(false, []convention.Header{feat}) {
		t.Error("a reviewed feature must not be a draft")
	}
	if !Draft(true, []convention.Header{feat}) {
		t.Error("an unattended delivery must be a draft (ADR-0010)")
	}
	if !Draft(false, []convention.Header{feat, wip}) {
		t.Error("a wip commit forces a draft (docs/reference/conventions.md)")
	}
}

func TestTitle(t *testing.T) {
	fix, feat, breaking := header(t, "fix: a"), header(t, "feat: b"), header(t, "refactor!: c")
	for _, tt := range []struct {
		commits []convention.Header
		want    string
	}{
		{nil, ""},
		{[]convention.Header{fix}, "🐛 fix: a"},
		{[]convention.Header{fix, feat}, "✨ feat: b"},
		{[]convention.Header{fix, feat, breaking}, "💥 refactor!: c"},
	} {
		if got := Title(tt.commits); got != tt.want {
			t.Errorf("Title(%v) = %q, want %q", tt.commits, got, tt.want)
		}
	}
}

func TestAILabel(t *testing.T) {
	if got := AILabel([]string{"bug", "LLM-generated", "ai-assisted"}); got != "ai-assisted" {
		t.Errorf("AILabel = %q, want the preferred name", got)
	}
	if got := AILabel([]string{"bug"}); got != "" {
		t.Errorf("AILabel = %q, want none", got)
	}
}
