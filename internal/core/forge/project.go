// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package forge

import (
	"regexp"
	"strconv"
	"strings"
)

// Project identifies a GitHub Project (v2), owned by an organisation or a
// user.
type Project struct {
	// Host is the GitHub host of a project given by URL, "" for the short
	// form (the repository's host).
	Host  string `json:"host,omitempty"`
	Owner string `json:"owner"`
	// Kind is ProjectOfOrganization or ProjectOfUser; "" when the short form
	// "owner/7" leaves it open (the API is then asked for both, in order).
	Kind   string `json:"kind,omitempty"`
	Number int    `json:"number"`
}

// Kinds of project owners, as their URLs and API routes name them.
const (
	ProjectOfOrganization = "orgs"
	ProjectOfUser         = "users"
)

// ProjectPattern matches what ParseProject accepts, and the empty string
// (no project). The settings validate github.project.url with it.
const ProjectPattern = `^(https://[A-Za-z0-9.-]+/(orgs|users)/[A-Za-z0-9-]+/projects/[1-9][0-9]*(/views/[0-9]+)?/?|[A-Za-z0-9-]+/[1-9][0-9]*)?$`

var projectURL = regexp.MustCompile(ProjectPattern)

// ParseProject reads a project as a user copies it: its URL
// ("https://github.com/orgs/acme/projects/7", a view of it included) or the
// short form "acme/7". "" and anything else are not projects.
func ParseProject(s string) (Project, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !projectURL.MatchString(s) {
		return Project{}, false
	}
	rest, isURL := strings.CutPrefix(s, "https://")
	parts := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	if !isURL {
		n, _ := strconv.Atoi(parts[1])
		return Project{Owner: parts[0], Number: n}, true
	}
	// host / kind / owner / "projects" / number [/ "views" / n]
	n, _ := strconv.Atoi(parts[4])
	return Project{Host: strings.ToLower(parts[0]), Kind: parts[1], Owner: parts[2], Number: n}, true
}

// String names the project in messages: "acme/7".
func (p Project) String() string { return p.Owner + "/" + strconv.Itoa(p.Number) }
