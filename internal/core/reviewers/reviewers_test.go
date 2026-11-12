// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package reviewers

import (
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/codeowners"
)

func TestOwners(t *testing.T) {
	f := codeowners.Parse([]byte("* @ada\n*.go @bob @acme/core\n/docs/ @Ada\n"))
	got := Owners(f, []string{"main.go", "docs/a.md", "x.go", "README.md"}, "")
	if want := []string{"@bob", "@acme/core", "@Ada"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Owners = %v, want %v", got, want)
	}
	if got := Owners(f, []string{"README.md", "main.go"}, "ada"); !reflect.DeepEqual(got, []string{"@bob", "@acme/core"}) {
		t.Errorf("without the author: %v", got)
	}
}

func TestLoginFromEmail(t *testing.T) {
	for email, want := range map[string]string{
		"16638358+GregoireF@users.noreply.github.com":       "GregoireF",
		"octocat@users.noreply.github.com":                  "octocat",
		"ada@example.com":                                   "",
		"49699333+dependabot[bot]@users.noreply.github.com": "",
	} {
		if got, _ := LoginFromEmail(email); got != want {
			t.Errorf("LoginFromEmail(%s) = %q, want %q", email, got, want)
		}
	}
}

func TestRank(t *testing.T) {
	commits := []Commit{
		{"1", "Bob", "bob@example.com"},
		{"2", "Ada", "1+ada@users.noreply.github.com"},
		{"3", "dependabot[bot]", "49699333+dependabot[bot]@users.noreply.github.com"},
		{"4", "Bob", "BOB@example.com"},
		{"5", "Me", "me@example.com"},
		{"6", "Me again", "2+me@users.noreply.github.com"},
		{"7", "Cy", "cy@example.com"},
	}
	got := Rank(commits, "me@example.com", "me")
	want := []Person{
		{Name: "Bob", Email: "bob@example.com", Commits: 2, SHA: "1"},
		{Login: "ada", Name: "Ada", Email: "1+ada@users.noreply.github.com", Commits: 1, SHA: "2"},
		{Name: "Cy", Email: "cy@example.com", Commits: 1, SHA: "7"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Rank = %+v\nwant %+v", got, want)
	}
}
