// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package codeowners

import (
	"strings"
	"testing"
)

// githubExample is the example file of GitHub's documentation, as published
// on 2026-11-12 (see the package comment), without its last block: it
// repeats /apps/ with an owner for /apps/github, tested apart.
const githubExample = `# This is a comment.
# Each line is a file pattern followed by one or more owners.

# These owners will be the default owners for everything in
# the repo. Unless a later match takes precedence,
# @global-owner1 and @global-owner2 will be requested for
# review when someone opens a pull request.
*       @global-owner1 @global-owner2

# Order is important; the last matching pattern takes the most
# precedence. When someone opens a pull request that only
# modifies JS files, only @js-owner and not the global
# owner(s) will be requested for a review.
*.js    @js-owner #This is an inline comment.

# You can also use email addresses if you prefer. They'll be
# used to look up users just like we do for commit author
# emails.
*.go docs@example.com

# Teams can be specified as code owners as well. Teams should
# be identified in the format @org/team-name. Teams must have
# explicit write access to the repository. In this example,
# the octocats team in the octo-org organization owns all .txt files.
*.txt @octo-org/octocats

# In this example, @doctocat owns any files in the build/logs
# directory at the root of the repository and any of its
# subdirectories.
/build/logs/ @doctocat

# The ` + "`docs/*`" + ` pattern will match files like
# ` + "`docs/getting-started.md`" + ` but not further nested files like
# ` + "`docs/build-app/troubleshooting.md`" + `.
docs/* docs@example.com

# In this example, @octocat owns any file in an apps directory
# anywhere in your repository.
apps/ @octocat

# In this example, @doctocat owns any file in the ` + "`/docs`" + `
# directory in the root of your repository and any of its
# subdirectories.
/docs/ @doctocat

# In this example, any change inside the ` + "`/scripts`" + ` directory
# will require approval from @doctocat or @octocat.
/scripts/ @doctocat @octocat

# In this example, @octocat owns any file in a ` + "`/logs`" + ` directory such as
# ` + "`/build/logs`" + `, ` + "`/scripts/logs`" + `, and ` + "`/deeply/nested/logs`" + `. Any changes
# in a ` + "`/logs`" + ` directory will require approval from @octocat.
**/logs @octocat

# In this example, @octocat owns any file in the ` + "`/apps`" + `
# directory in the root of your repository except for the ` + "`/apps/github`" + `
# subdirectory, as its owners are left empty. Without an owner, changes
# to ` + "`apps/github`" + ` can be made with the approval of any user who has
# write access to the repository.
/apps/ @octocat
/apps/github
`

// TestGitHubsExample: each path's owners as the comments of GitHub's own
// example describe them.
func TestGitHubsExample(t *testing.T) {
	f := Parse([]byte(githubExample))
	for path, want := range map[string]string{
		"README.md":                         "@global-owner1 @global-owner2",
		"src/app.js":                        "@js-owner",
		"cmd/main.go":                       "docs@example.com",
		"notes.txt":                         "@octo-org/octocats",
		"build/logs/today.txt":              "@octocat", // **/logs comes later than /build/logs/
		"build/logs/deep/today.log":         "@octocat",
		"docs/getting-started.md":           "@doctocat", // /docs/ comes later than docs/*
		"lib/apps/main.py":                  "@octocat",
		"scripts/deploy.sh":                 "@doctocat @octocat",
		"deeply/nested/logs/a.log":          "@octocat",
		"apps/web/index.html":               "@octocat",
		"apps/github/workflow.yml":          "",
		"apps/github/deep/workflow.yml":     "",
		"docs/build-app/troubleshooting.md": "@doctocat",
	} {
		if got := strings.Join(f.Owners(path), " "); got != want {
			t.Errorf("Owners(%s) = %q, want %q", path, got, want)
		}
	}
}

// TestDirectFilesOnly: "docs/*" without the later /docs/ rule.
func TestDirectFilesOnly(t *testing.T) {
	f := Parse([]byte("* @all\ndocs/* @docs\n"))
	if got := f.Owners("docs/getting-started.md"); len(got) != 1 || got[0] != "@docs" {
		t.Errorf("docs/getting-started.md: %v", got)
	}
	if got := f.Owners("docs/build-app/troubleshooting.md"); len(got) != 1 || got[0] != "@all" {
		t.Errorf("docs/build-app/troubleshooting.md: %v, want the default owner", got)
	}
}

func TestAnOwnerForTheSubdirectory(t *testing.T) {
	f := Parse([]byte("/apps/ @octocat\n/apps/github @doctocat\n"))
	if got := f.Owners("apps/github/a.yml"); len(got) != 1 || got[0] != "@doctocat" {
		t.Errorf("apps/github/a.yml: %v", got)
	}
	if got := f.Owners("apps/other/a.yml"); len(got) != 1 || got[0] != "@octocat" {
		t.Errorf("apps/other/a.yml: %v", got)
	}
}

// TestWhatGitHubDoesNotSupport is skipped: escaping, negation, ranges.
func TestWhatGitHubDoesNotSupport(t *testing.T) {
	f := Parse([]byte("* @all\n\\#notes @hash\n!vendor/ @not\n*.[ch] @c\n"))
	if len(f.Rules) != 1 {
		t.Fatalf("rules = %+v, want only the first", f.Rules)
	}
	if f.Owners("") != nil || Parse(nil).Owners("a.go") != nil {
		t.Error("no rule must give no owners")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(githubExample), "apps/github/a.yml")
	f.Fuzz(func(t *testing.T, content []byte, path string) {
		Parse(content).Owners(path)
	})
}
