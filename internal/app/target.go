// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
)

// upstreamRemote is the name git users give the parent of a fork (GitHub
// Docs, "Configuring a remote repository for a fork").
const upstreamRemote = "upstream"

// parentRefPrefix holds the base branches of a fork's parent fetched without
// a remote: ship's own refs, outside refs/remotes so that `git branch -r`
// does not list them.
const parentRefPrefix = "refs/ship/parent/"

// publication is where a branch goes: ship pushes it to its remote
// (ship.git.remote) and opens its pull request on prRepo, the remote's
// repository or, for a fork, the parent.
type publication struct {
	remote string
	repo   forge.Repository // the remote's repository
	// prRepo is where pull requests open; prDefault is its default branch.
	prRepo    forge.Repository
	prDefault string
	// base is the branch pull requests target (ship.pullRequest.base, else
	// prDefault).
	base string
	// baseRemote is the remote whose branches are prRepo's: the remote
	// itself, the "upstream" remote of a fork, or "" when the parent is
	// fetched by URL (parentURL).
	baseRemote string
	parentURL  string
}

// fork reports whether pull requests open on another repository.
func (p publication) fork() bool { return !sameRepository(p.prRepo, p.repo) }

// head is the head of the pull request of branch: "<owner>:<branch>" from a
// fork.
func (p publication) head(branch string) string {
	if p.fork() {
		return p.repo.Owner + ":" + branch
	}
	return branch
}

// closesIssues reports whether closing keywords work: GitHub only reads them
// in pull requests to the default branch.
func (p publication) closesIssues() bool { return p.base == p.prDefault }

// pullRequestSlug is what the operation log records of prRepo: "" when it
// is the remote's own.
func (p publication) pullRequestSlug() string {
	if p.fork() {
		return p.prRepo.Slug()
	}
	return ""
}

func sameRepository(a, b forge.Repository) bool {
	return strings.EqualFold(a.Host, b.Host) && strings.EqualFold(a.Slug(), b.Slug())
}

// resolvePublication finds where pull requests open. A fork is told by
// GitHub (the repository has a parent) or, failing that, by an "upstream"
// remote naming another repository of the same server; its base comes from
// the parent, through that remote when it names the parent, else fetched by
// URL.
func resolvePublication(ctx context.Context, ws Workspace, eff config.Effective, repo forge.Repository, info ports.RepositoryInfo) (publication, error) {
	remote := remoteOf(eff)
	p := publication{remote: remote, repo: repo, prRepo: repo, prDefault: info.DefaultBranch, baseRemote: remote}
	var upstream *forge.Repository
	if remote != upstreamRemote {
		if r, err := repositoryOf(ctx, ws, upstreamRemote); err == nil && strings.EqualFold(r.Host, repo.Host) && !sameRepository(r, repo) {
			upstream = &r
		}
	}
	switch {
	case info.Parent != nil:
		parent := info.Parent
		p.prRepo, p.prDefault = parent.Repository, parent.DefaultBranch
		if p.prRepo.Host == "" {
			p.prRepo.Host = repo.Host
		}
		if upstream != nil && sameRepository(*upstream, p.prRepo) {
			p.baseRemote = upstreamRemote
		} else {
			p.baseRemote, p.parentURL = "", parentURL(ctx, ws, remote, parent)
		}
	case upstream != nil:
		upInfo, err := ws.Forge.Repository(ctx, *upstream)
		if err != nil {
			return p, forgeFault(err, *upstream)
		}
		p.prRepo, p.prDefault, p.baseRemote = *upstream, upInfo.DefaultBranch, upstreamRemote
	}
	p.base = p.prDefault
	if b := eff.Settings.Ship.PullRequest.Base; b != "" {
		p.base = b
	}
	return p, nil
}

// parentURL is the URL that fetches the parent with the transport of the
// remote: SSH for an SSH remote (its key works there too), else HTTPS.
func parentURL(ctx context.Context, ws Workspace, remote string, parent *ports.ParentRepository) string {
	url, _ := ws.Git.RemoteURL(ctx, remote)
	if parent.SSHURL != "" && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return parent.SSHURL
	}
	if parent.CloneURL != "" {
		return parent.CloneURL
	}
	return parent.SSHURL
}

// fetchBase fetches the remote (the lease and the remote branch of the next
// push depend on it) and the base, and returns the ref of the base to bring
// the branch up to date with: "origin/main", "upstream/main" or, for a fork
// without an upstream remote, a ref under parentRefPrefix. It returns "" when
// the base does not exist yet (a first publication).
func fetchBase(ctx context.Context, ws Workspace, p publication) (string, error) {
	fetched := ws.begin(ctx, "publish/fetch")
	err := ws.Git.Fetch(ctx, p.remote)
	fetched(err)
	if err != nil {
		return "", fault.New(fault.FetchFailed, "tidy.fetch_failed", "remote", p.remote, "detail", lastLine(err.Error()))
	}
	switch {
	case p.baseRemote == "":
		ref := parentRefPrefix + p.base
		fetched := ws.begin(ctx, "publish/fetch_parent")
		err := ws.Git.FetchBranch(ctx, p.parentURL, p.base, ref)
		fetched(err)
		if err != nil {
			return "", fault.New(fault.FetchFailed, "tidy.fetch_failed", "remote", p.parentURL, "detail", lastLine(err.Error()))
		}
		if commit, err := ws.Git.CommitOf(ctx, ref); err == nil && commit != "" {
			return ref, nil
		}
		return "", nil
	case p.baseRemote != p.remote:
		fetched := ws.begin(ctx, "publish/fetch_parent")
		err := ws.Git.Fetch(ctx, p.baseRemote)
		fetched(err)
		if err != nil {
			return "", fault.New(fault.FetchFailed, "tidy.fetch_failed", "remote", p.baseRemote, "detail", lastLine(err.Error()))
		}
	}
	if exists, err := ws.Git.RemoteBranchExists(ctx, p.baseRemote, p.base); err == nil && exists {
		return p.baseRemote + "/" + p.base, nil
	}
	return "", nil
}
