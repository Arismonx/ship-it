// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fix

import (
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// RemedyKind is an action `ship fix` can apply (docs/reference/error-playbook.md, playbook).
type RemedyKind string

// Remedies. Each one is applied after a checkpoint and logged, so that
// `ship undo` reverts it.
const (
	// SetIdentity writes user.name and user.email globally: an identity is
	// personal, not per repository (decision of 2026-10-23).
	SetIdentity RemedyKind = "set_identity"
	// EnableLongPaths sets core.longpaths in the repository (Windows).
	EnableLongPaths RemedyKind = "enable_long_paths"
	// DisableMirror unsets remote.<name>.mirror in the repository.
	DisableMirror RemedyKind = "disable_mirror"
	// UntrackFiles adds patterns to .gitignore and removes the files from the
	// index, keeping them on disk.
	UntrackFiles RemedyKind = "untrack_files"
	// TidyBranches leaves merged branches and deletes them (ship tidy).
	TidyBranches RemedyKind = "tidy_branches"
)

// Remedy is one proposed fix, described by an i18n key and parameters.
type Remedy struct {
	Kind   RemedyKind        `json:"kind"`
	Code   fault.Code        `json:"code"`
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
	// ConfigKey and ConfigValue describe a configuration change.
	ConfigKey   string `json:"config_key,omitempty"`
	ConfigValue string `json:"config_value,omitempty"`
	// Paths are the files to untrack; Patterns the .gitignore lines to add.
	Paths    []string `json:"paths,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
}

// GitHubUser is the account the GitHub token belongs to.
type GitHubUser struct {
	Login string
	ID    int64
	Name  string
}

// NoReplyEmail is the private commit address GitHub gives every account
// created since 2017-07-18: ID+LOGIN@users.noreply.github.com (GitHub Docs,
// "Email addresses reference").
func (u GitHubUser) NoReplyEmail() string {
	return strconv.FormatInt(u.ID, 10) + "+" + u.Login + "@users.noreply.github.com"
}

// FixFacts are the facts of the doctor plus what only ship fix looks at.
type FixFacts struct {
	Facts
	// GitHubUser is set when a token is available; it feeds the identity.
	GitHubUser *GitHubUser
	// Tracked lists the files in the index.
	Tracked []string
	// Gitignore is the content of the root .gitignore ("" when absent).
	Gitignore string
	// MergedBranches counts local branches whose pull request is merged.
	MergedBranches int
}

// junk are files that should never be tracked: the pattern to ignore and
// whether a path falls under it.
var junk = []struct {
	pattern string
	matches func(p string) bool
}{
	{"node_modules/", func(p string) bool {
		return p == "node_modules" || strings.HasPrefix(p, "node_modules/") || strings.Contains(p, "/node_modules/")
	}},
	// ".env.*" and "!.env.example" are added together with ".env".
	{".env", func(p string) bool { return isEnvFile(path.Base(p)) }},
}

// isEnvFile matches .env and .env.local, not the shared templates.
func isEnvFile(name string) bool {
	switch name {
	case ".env.example", ".env.sample", ".env.template", ".env.dist":
		return false
	}
	return name == ".env" || strings.HasPrefix(name, ".env.")
}

// Remedies lists what ship fix proposes, in a stable order. Problems that
// need a human decision (an unfinished merge…) are left to the doctor.
func Remedies(f FixFacts) []Remedy {
	var remedies []Remedy
	if f.UserName == "" || f.UserEmail == "" {
		if u := f.GitHubUser; u != nil {
			name := f.UserName
			if name == "" {
				name = u.Name
			}
			if name == "" {
				name = u.Login
			}
			email := f.UserEmail
			if email == "" {
				email = u.NoReplyEmail()
			}
			remedies = append(remedies, Remedy{Kind: SetIdentity, Code: fault.GitIdentityMissing, Key: "fix.set_identity",
				Params: map[string]string{"name": name, "email": email}})
		}
	}
	if f.OperatingSystem == "windows" && !f.LongPathsEnabled {
		remedies = append(remedies, Remedy{Kind: EnableLongPaths, Code: fault.LongPathsDisabled, Key: "fix.enable_long_paths",
			ConfigKey: "core.longpaths", ConfigValue: "true"})
	}
	for _, remote := range f.MirrorRemotes {
		remedies = append(remedies, Remedy{Kind: DisableMirror, Code: fault.MirrorRemote, Key: "fix.disable_mirror",
			Params: map[string]string{"remote": remote}, ConfigKey: "remote." + remote + ".mirror"})
	}
	if r, ok := untrackRemedy(f.Tracked, f.Gitignore); ok {
		remedies = append(remedies, r)
	}
	if f.MergedBranches > 0 {
		remedies = append(remedies, Remedy{Kind: TidyBranches, Code: fault.Healthy, Key: "fix.tidy_branches",
			Params: map[string]string{"count": strconv.Itoa(f.MergedBranches)}})
	}
	return remedies
}

func untrackRemedy(tracked []string, gitignore string) (Remedy, bool) {
	var paths, patterns []string
	ignored := strings.Split(strings.ReplaceAll(gitignore, "\r\n", "\n"), "\n")
	for _, j := range junk {
		var hits []string
		for _, p := range tracked {
			if j.matches(p) {
				hits = append(hits, p)
			}
		}
		if len(hits) == 0 {
			continue
		}
		paths = append(paths, hits...)
		wanted := []string{j.pattern}
		if j.pattern == ".env" {
			wanted = append(wanted, ".env.*", "!.env.example")
		}
		for _, w := range wanted {
			if !slices.Contains(ignored, w) {
				patterns = append(patterns, w)
			}
		}
	}
	if len(paths) == 0 {
		return Remedy{}, false
	}
	return Remedy{Kind: UntrackFiles, Code: fault.TrackedJunk, Key: "fix.untrack_files",
		Params: map[string]string{"count": strconv.Itoa(len(paths)), "patterns": strings.Join(patterns, " ")},
		Paths:  paths, Patterns: patterns}, true
}
