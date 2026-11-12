// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package reviewers suggests who could review a change (#46): the
// CODEOWNERS owners of its files, and the people who changed those files
// most lately. The author is never suggested, nor are bots.
//
// Allowed imports: standard library without I/O, other core packages.
package reviewers

import (
	"regexp"
	"sort"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/codeowners"
)

// Commit is the author of one commit of the history.
type Commit struct {
	SHA, Name, Email string
}

// Person is someone who changed the files lately.
type Person struct {
	Login   string `json:"login,omitempty"`
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Commits int    `json:"commits"`
	// SHA is their latest commit, to ask GitHub for their account.
	SHA string `json:"-"`
}

// Owners returns the owners of paths in the order they first appear,
// without the author's account (login without "@").
func Owners(f codeowners.File, paths []string, author string) []string {
	seen := map[string]bool{}
	var owners []string
	for _, p := range paths {
		for _, o := range f.Owners(p) {
			key := strings.ToLower(o)
			if seen[key] || (author != "" && strings.EqualFold(o, "@"+author)) {
				continue
			}
			seen[key] = true
			owners = append(owners, o)
		}
	}
	return owners
}

// noreply is GitHub's private commit email: "123+login@…" or "login@…".
var noreply = regexp.MustCompile(`(?i)^(?:\d+\+)?([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)@users\.noreply\.github\.com$`)

// LoginFromEmail returns the account a GitHub noreply email names.
func LoginFromEmail(email string) (string, bool) {
	m := noreply.FindStringSubmatch(email)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// IsBot reports a bot's commit: "dependabot[bot]", "github-actions[bot]"…
func IsBot(name, email string) bool {
	return strings.Contains(name, "[bot]") || strings.Contains(email, "[bot]")
}

// Rank counts the commits of each person, newest first in commits, most
// commits first, the author (by email or login) and bots left out.
func Rank(commits []Commit, authorEmail, authorLogin string) []Person {
	var people []Person
	index := map[string]int{}
	for _, c := range commits {
		if IsBot(c.Name, c.Email) || (authorEmail != "" && strings.EqualFold(c.Email, authorEmail)) {
			continue
		}
		login, _ := LoginFromEmail(c.Email)
		if authorLogin != "" && strings.EqualFold(login, authorLogin) {
			continue
		}
		key := strings.ToLower(c.Email)
		if i, ok := index[key]; ok {
			people[i].Commits++
			continue
		}
		index[key] = len(people)
		people = append(people, Person{Login: login, Name: c.Name, Email: c.Email, Commits: 1, SHA: c.SHA})
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i].Commits > people[j].Commits })
	return people
}
