// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// trailerCases are messages and the trailers git finds in them (the gitexec
// tests compare Trailers with `git interpret-trailers` itself).
var trailerCases = []struct {
	name     string
	message  string
	trailers []Trailer
}{
	{"none", "✨ feat: add a\n\nBecause.", nil},
	{"title only", "Signed-off-by: A <a@x.org>", nil},
	{"git commit -s", "✨ feat: add a\n\nBody.\n\nSigned-off-by: A <a@x.org>\n",
		[]Trailer{{"Signed-off-by", "A <a@x.org>"}}},
	{"several", "fix: b\n\nCo-Authored-By: C <c@x.org>\nSigned-off-by: A <a@x.org>",
		[]Trailer{{"Co-Authored-By", "C <c@x.org>"}, {"Signed-off-by", "A <a@x.org>"}}},
	{"not the last paragraph", "fix: b\n\nSigned-off-by: A <a@x.org>\n\nMore text.", nil},
	{"25% with a git trailer", "fix: b\n\nnot a trailer line\nSigned-off-by: A <a@x.org>",
		[]Trailer{{"Signed-off-by", "A <a@x.org>"}}},
	{"under 25%", "fix: b\n\nline one\nline two\nline three\nline four\nSigned-off-by: A <a@x.org>", nil},
	{"no git trailer, not all trailers", "fix: b\n\nReviewed-by: R <r@x.org>\nsome prose", nil},
	{"folded value", "fix: b\n\nNote: a long\n  value\nSigned-off-by: A <a@x.org>",
		[]Trailer{{"Note", "a long value"}, {"Signed-off-by", "A <a@x.org>"}}},
	{"url is not a trailer", "fix: b\n\nhttps://example.org/x\nSigned-off-by: A <a@x.org>",
		[]Trailer{{"Signed-off-by", "A <a@x.org>"}}},
	{"space before colon", "fix: b\n\nSigned-off-by : A <a@x.org>",
		[]Trailer{{"Signed-off-by", "A <a@x.org>"}}},
	{"trailing blank lines", "fix: b\n\nSigned-off-by: A <a@x.org>\n\n\n",
		[]Trailer{{"Signed-off-by", "A <a@x.org>"}}},
}

func TestTrailers(t *testing.T) {
	for _, tc := range trailerCases {
		if got := Trailers(tc.message); !reflect.DeepEqual(got, tc.trailers) {
			t.Errorf("%s: Trailers = %q, want %q", tc.name, got, tc.trailers)
		}
	}
}

func TestCheckSignoff(t *testing.T) {
	author := SignedCommit{AuthorName: "Ada Lovelace", AuthorEmail: "ada@example.org",
		CommitterName: "Ada Lovelace", CommitterEmail: "ada@example.org"}
	with := func(c SignedCommit, message string) SignedCommit { c.Message = message; return c }
	for _, tc := range []struct {
		name   string
		commit SignedCommit
		want   fault.Code
	}{
		{"signed", with(author, "fix: a\n\nSigned-off-by: Ada Lovelace <ada@example.org>"), ""},
		{"case does not matter", with(author, "fix: a\n\nsigned-off-by: ADA LOVELACE <Ada@Example.org>"), ""},
		{"missing", with(author, "fix: a"), fault.SignoffMissing},
		{"in the body only", with(author, "fix: a\n\nSigned-off-by: Ada Lovelace <ada@example.org>\n\nMore."), fault.SignoffMissing},
		{"someone else", with(author, "fix: a\n\nSigned-off-by: Bob <bob@example.org>"), fault.SignoffMismatch},
		{"right name, wrong email", with(author, "fix: a\n\nSigned-off-by: Ada Lovelace <other@example.org>"), fault.SignoffMismatch},
		{"one of several", with(author, "fix: a\n\nSigned-off-by: Bob <bob@example.org>\nSigned-off-by: Ada Lovelace <ada@example.org>"), ""},
		{"no email", with(author, "fix: a\n\nSigned-off-by: Ada Lovelace"), fault.SignoffMismatch},
		{"committer signs", with(SignedCommit{AuthorName: "Bob", AuthorEmail: "bob@example.org",
			CommitterName: "Ada Lovelace", CommitterEmail: "ada@example.org"}, "fix: a\n\nSigned-off-by: Ada Lovelace <ada@example.org>"), ""},
		{"merge", SignedCommit{Merge: true, Message: "Merge branch 'x'"}, ""},
		{"bot", SignedCommit{AuthorName: "dependabot[bot]", AuthorEmail: "49699333+dependabot[bot]@users.noreply.github.com", Message: "bump"}, ""},
	} {
		got := CheckSignoff(tc.commit)
		switch {
		case tc.want == "" && got != nil:
			t.Errorf("%s: unexpected %v", tc.name, got)
		case tc.want != "" && (got == nil || got.Code != tc.want):
			t.Errorf("%s: got %v, want %s", tc.name, got, tc.want)
		}
	}
}

func FuzzTrailers(f *testing.F) {
	for _, tc := range trailerCases {
		f.Add(tc.message)
	}
	f.Fuzz(func(t *testing.T, message string) {
		for _, tr := range Trailers(message) {
			if tr.Key == "" {
				t.Fatalf("empty key in %q", message)
			}
		}
		_ = CheckSignoff(SignedCommit{Message: message})
	})
}
