// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"path"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
)

// templateReadLimit bounds a pull request template; real ones are a few KB.
const templateReadLimit = 64 << 10

// templateName is GitHub's file name for a pull request template.
const templateName = "pull_request_template.md"

// templateDirectories are where GitHub looks for a pull request template,
// in its order of precedence: .github, the root, docs (GitHub Docs, "Creating
// a default community health file", verified 2026-10-23).
var templateDirectories = []string{".github", "", "docs"}

// pullRequestTemplate returns the template the pull request body follows:
// none for "ship", the named file for a path, and for "auto" the one GitHub
// itself would offer, read from rev (the default branch: templates apply
// once merged there). A directory of several templates
// (PULL_REQUEST_TEMPLATE/) offers none by default on GitHub either.
func pullRequestTemplate(ctx context.Context, ws Workspace, rev, setting string) string {
	switch setting {
	case config.TemplateShip:
		return ""
	case config.TemplateAuto, "":
	default:
		content, err := ws.Git.ReadFileAt(ctx, rev, setting, templateReadLimit)
		if err != nil {
			return ""
		}
		return string(content)
	}
	for _, dir := range templateDirectories {
		entries, err := ws.Git.FilesAt(ctx, rev, dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			// GitHub documents both spellings; the match ignores case.
			if strings.EqualFold(path.Base(entry), templateName) {
				if content, err := ws.Git.ReadFileAt(ctx, rev, entry, templateReadLimit); err == nil {
					return string(content)
				}
			}
		}
	}
	return ""
}

// branchCommits parses the messages of the branch with the repository's
// commit types: the pull request is written from their headers and from the
// prose of their bodies.
func branchCommits(rules convention.Rules, messages []string) (commits []pullrequest.Commit, headers []convention.Header) {
	for _, m := range messages {
		subject, body := pullrequest.ParseMessage(m)
		h, err := rules.ParseHeader(subject)
		if err != nil {
			continue
		}
		commits = append(commits, pullrequest.Commit{Header: h, Body: body, Tested: pullrequest.TrailerValues(m, convention.TestedTrailer)})
		headers = append(headers, h)
	}
	return commits, headers
}

// passedChecks are the commands of the gates that ran and passed.
func passedChecks(results []GateResult) []string {
	var commands []string
	for _, r := range results {
		if r.Status == GatePassed && r.Command != "" {
			commands = append(commands, r.Command)
		}
	}
	return commands
}

// pullRequestBody writes the body in the repository's template, else in
// ship-it's format.
func pullRequestBody(in pullrequest.BodyInput, template string) string {
	if strings.TrimSpace(template) == "" {
		return pullrequest.Body(in)
	}
	return pullrequest.FillTemplate(template, pullrequest.Compose(in))
}
