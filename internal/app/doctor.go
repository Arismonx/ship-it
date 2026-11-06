// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/fix"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/ports"
)

// FindingView is one doctor result as surfaces render it.
type FindingView struct {
	Code     string            `json:"code"`
	Severity string            `json:"severity"` // "ok", "warning", "blocking"
	Key      string            `json:"key"`
	Remedy   string            `json:"remedy,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
}

// DoctorReport is the outcome of `ship doctor`.
type DoctorReport struct {
	Findings []FindingView `json:"findings"`
	Worst    string        `json:"worst"`
}

// Blocking reports whether ship-it cannot work until something is fixed.
func (r DoctorReport) Blocking() bool { return r.Worst == "blocking" }

// OutsideRepository reports whether the doctor ran outside a git repository,
// where ship has nothing to ship even if git itself is fine.
func (r DoctorReport) OutsideRepository() bool {
	for _, f := range r.Findings {
		if f.Code == string(fault.NotARepository) {
			return true
		}
	}
	return false
}

var severityNames = map[fix.Severity]string{fix.Healthy: "ok", fix.Warning: "warning", fix.Blocking: "blocking"}

// Doctor gathers facts about git and the repository and diagnoses them.
// It never changes anything.
func Doctor(ctx context.Context, git ports.Git, operatingSystem string) DoctorReport {
	return doctorWith(ctx, git, operatingSystem, config.DefaultRemote, nil)
}

// doctorWith is Doctor with the caller's read of `git status`, nil to read it.
func doctorWith(ctx context.Context, git ports.Git, operatingSystem, remote string, status *ports.WorkingTreeStatus) DoctorReport {
	return reportOf(fix.Diagnose(gatherFacts(ctx, git, operatingSystem, remote, status)))
}

// CheckUp is the full `ship doctor`: Doctor, plus the configuration files
// (validity, organisation policy) and one read-only API call proving that a
// token exists and sees the repository of origin. Deliveries use Doctor,
// which needs no network.
func CheckUp(ctx context.Context, ws Workspace) DoctorReport {
	remote := gitRemote(ctx, ws)
	facts := gatherFacts(ctx, ws.Git, ws.OperatingSystem, remote, nil)
	if ws.Config != nil {
		facts.Configuration = &fix.ConfigurationFacts{}
		eff, err := LoadSettings(ctx, ws)
		var f *Fault
		switch {
		case errors.As(err, &f):
			facts.Configuration.Problem = f
		case err != nil:
			facts.Configuration.Problem = fault.New(fault.Unexpected, "app.unexpected_error", "error", err.Error())
		case eff.Policy != nil:
			facts.Configuration.Organization = eff.Policy.Organization
			facts.Configuration.Contact = eff.Policy.Contact
			facts.Configuration.Locked = len(eff.Policy.Locked)
		}
	}
	if facts.InsideRepository && facts.HasRemote {
		if url, err := ws.Git.RemoteURL(ctx, remote); err == nil {
			repo, ok := forge.ParseRemoteURL(url)
			switch {
			case !ok:
				facts.RemoteNotGitHub = true
			case ws.Forge != nil && ws.Credentials != nil:
				facts.GitHub = gitHubFacts(ctx, ws, repo)
			}
		}
	}
	return reportOf(fix.Diagnose(facts))
}

func gitHubFacts(ctx context.Context, ws Workspace, repo forge.Repository) *fix.GitHubFacts {
	gh := &fix.GitHubFacts{Host: repo.Host, Repository: repo.Slug()}
	token, err := ws.Credentials.Token(ctx, repo.Host)
	if err != nil {
		gh.Access = fix.GitHubNoToken
		return gh
	}
	gh.TokenSource = token.Source
	_, err = ws.Forge.Repository(ctx, repo)
	switch {
	case err == nil:
		gh.Access = fix.GitHubAccessGranted
	case errors.Is(err, ports.ErrNotAuthenticated):
		gh.Access = fix.GitHubNoToken
	case errors.Is(err, ports.ErrTokenRejected):
		gh.Access = fix.GitHubTokenRejected
	case errors.Is(err, ports.ErrNotFound):
		gh.Access = fix.GitHubRepositoryHidden
	default:
		gh.Access = fix.GitHubUnreachable
	}
	return gh
}

func reportOf(findings []fix.Finding) DoctorReport {
	report := DoctorReport{Worst: severityNames[fix.Worst(findings)]}
	for _, f := range findings {
		report.Findings = append(report.Findings, FindingView{
			Code: string(f.Code), Severity: severityNames[f.Severity], Key: f.Key, Remedy: f.Remedy, Params: f.Params,
		})
	}
	return report
}

// gatherFacts collects what fix.Diagnose needs; each failed probe simply
// leaves its fact at the zero value, which the diagnosis reports. status is
// the caller's read of `git status`, nil to read it.
func gatherFacts(ctx context.Context, git ports.Git, operatingSystem, remote string, status *ports.WorkingTreeStatus) fix.Facts {
	facts := fix.Facts{OperatingSystem: operatingSystem, Remote: remote}
	version, err := git.Version(ctx)
	if err != nil {
		return facts
	}
	facts.GitVersion = version
	facts.UserName, _, _ = git.ConfigValue(ctx, "user.name")
	facts.UserEmail, _, _ = git.ConfigValue(ctx, "user.email")
	if longPaths, _, _ := git.ConfigValue(ctx, "core.longpaths"); longPaths == "true" {
		facts.LongPathsEnabled = true
	}
	if _, err := git.TopLevel(ctx); err != nil {
		return facts
	}
	facts.InsideRepository = true
	if status != nil {
		facts.DetachedHead = status.Branch.Detached
	} else if status, err := git.Status(ctx); err == nil {
		facts.DetachedHead = status.Branch.Detached
	}
	facts.OperationInProgress, _ = git.OperationInProgress(ctx)
	if remotes, err := git.Remotes(ctx); err == nil {
		for _, r := range remotes {
			if r == remote {
				facts.HasRemote = true
			}
		}
	}
	if mirrors, err := git.ConfigMatching(ctx, `^remote\..*\.mirror$`); err == nil {
		for key, value := range mirrors {
			if value == "true" {
				facts.MirrorRemotes = append(facts.MirrorRemotes,
					strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".mirror"))
			}
		}
		sort.Strings(facts.MirrorRemotes)
	}
	return facts
}
