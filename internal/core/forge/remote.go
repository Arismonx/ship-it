// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package forge models the GitHub side of a repository without talking to it:
// which repository a remote URL designates, and where its API lives.
//
// Allowed imports: standard library without I/O, other core packages.
package forge

import (
	"net/url"
	"regexp"
	"strings"
)

// DotCom is the host of github.com.
const DotCom = "github.com"

// Repository identifies a repository on a GitHub host.
type Repository struct {
	Host  string `json:"host"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// Slug returns "owner/name".
func (r Repository) Slug() string { return r.Owner + "/" + r.Name }

// IsEnterpriseServer reports whether the host is a GitHub Enterprise Server
// instance, as opposed to github.com or GHE.com (data residency, *.ghe.com).
func (r Repository) IsEnterpriseServer() bool {
	return r.Host != DotCom && !strings.HasSuffix(r.Host, ".ghe.com")
}

// APIBaseURL returns the REST API root of the host, without trailing slash:
// api.github.com, api.<subdomain>.ghe.com, or <host>/api/v3 on Enterprise Server.
func (r Repository) APIBaseURL() string {
	switch {
	case r.Host == DotCom:
		return "https://api.github.com"
	case !r.IsEnterpriseServer():
		return "https://api." + r.Host
	default:
		return "https://" + r.Host + "/api/v3"
	}
}

// scpLike matches "git@github.com:owner/name.git", the scp-like syntax of
// git-clone(1).
var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):([^/].*)$`)

// ParseRemoteURL returns the repository a git remote URL points to. It
// accepts https://, ssh:// and scp-like URLs, with or without ".git".
func ParseRemoteURL(raw string) (Repository, bool) {
	raw = strings.TrimSpace(raw)
	var host, path string
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		switch u.Scheme {
		case "https", "http", "ssh", "git":
		default:
			return Repository{}, false
		}
		host, path = u.Hostname(), u.Path
	} else if m := scpLike.FindStringSubmatch(raw); m != nil && !strings.Contains(m[1], `\`) {
		host, path = m[1], m[2]
	} else {
		return Repository{}, false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if host == "" || len(parts) != 2 {
		return Repository{}, false
	}
	owner, name := parts[0], strings.TrimSuffix(parts[1], ".git")
	if owner == "" || name == "" {
		return Repository{}, false
	}
	return Repository{Host: strings.ToLower(host), Owner: owner, Name: name}, true
}
