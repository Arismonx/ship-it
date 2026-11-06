// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fix

import (
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// MinimumGitVersion is the oldest supported git: the one shipped by Ubuntu
// 24.04 LTS (decision of 2026-10-22).
var MinimumGitVersion = GitVersion{Major: 2, Minor: 43}

// GitVersion is the numeric part of `git --version`.
type GitVersion struct{ Major, Minor, Patch int }

// ParseGitVersion reads "2.52.0.windows.1" or "2.43.0" and ignores suffixes.
func ParseGitVersion(s string) (GitVersion, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ".", 4)
	if len(parts) < 2 {
		return GitVersion{}, false
	}
	var numbers [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			if i < 2 {
				return GitVersion{}, false
			}
			break
		}
		numbers[i] = n
	}
	return GitVersion{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}, true
}

// AtLeast reports whether v is the same as or newer than other.
func (v GitVersion) AtLeast(other GitVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch >= other.Patch
}

func (v GitVersion) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

// Severity grades a doctor finding.
type Severity int

// Severities in increasing order.
const (
	Healthy Severity = iota
	Warning
	Blocking
)

// Finding is one result of the doctor. Remedy is the i18n key of the exact
// command or action that resolves it, empty when healthy.
type Finding struct {
	Code     fault.Code
	Severity Severity
	Key      string
	Remedy   string
	Params   map[string]string
}

// Facts is what the doctor needs to know; app gathers them through ports.
type Facts struct {
	OperatingSystem     string // runtime.GOOS of the machine
	GitVersion          string // empty when git is missing
	InsideRepository    bool
	DetachedHead        bool
	OperationInProgress string // "merge", "rebase", "cherry-pick", "revert" or empty
	UserName            string
	UserEmail           string
	// HasRemote: the remote ship publishes to (Remote, ship.git.remote)
	// exists.
	HasRemote     bool
	Remote        string
	MirrorRemotes []string // remotes with remote.<name>.mirror=true
	// RemoteNotGitHub: the remote's URL names no GitHub repository, so ship
	// can commit but not publish.
	RemoteNotGitHub  bool
	LongPathsEnabled bool // core.longpaths, relevant on Windows only
	// GitHub is set when origin is a GitHub repository and the network check
	// was requested; nil otherwise.
	GitHub *GitHubFacts
	// Configuration is set when the configuration files were read.
	Configuration *ConfigurationFacts
}

// ConfigurationFacts describe the configuration files.
type ConfigurationFacts struct {
	// Problem is the fault that makes them unusable, if any.
	Problem *fault.Fault
	// Organization imposes Locked settings through its policy ("" if none).
	Organization string
	Contact      string
	Locked       int
}

// GitHubAccess is what one API call to the repository revealed.
type GitHubAccess int

// Outcomes of the GitHub check.
const (
	GitHubAccessGranted GitHubAccess = iota
	GitHubNoToken
	GitHubTokenRejected
	GitHubRepositoryHidden
	GitHubUnreachable
)

// GitHubFacts describe the link between the repository and GitHub.
type GitHubFacts struct {
	Host       string
	Repository string // owner/name
	// TokenSource names where the token came from ("GH_TOKEN", "gh"…).
	TokenSource string
	Access      GitHubAccess
}

// Diagnose evaluates the facts. Findings come in a stable order; blocking
// findings stop the remaining checks that depend on them.
func Diagnose(f Facts) []Finding {
	var findings []Finding
	add := func(code fault.Code, severity Severity, key, remedy string, kv ...string) {
		finding := Finding{Code: code, Severity: severity, Key: key, Remedy: remedy}
		if len(kv) > 0 {
			finding.Params = map[string]string{}
			for i := 0; i+1 < len(kv); i += 2 {
				finding.Params[kv[i]] = kv[i+1]
			}
		}
		findings = append(findings, finding)
	}

	version, ok := ParseGitVersion(f.GitVersion)
	switch {
	case f.GitVersion == "":
		add(fault.GitMissing, Blocking, "doctor.git.missing", "doctor.remedy.install_git."+f.OperatingSystem)
		return findings
	case !ok || !version.AtLeast(MinimumGitVersion):
		add(fault.GitTooOld, Blocking, "doctor.git.too_old", "doctor.remedy.upgrade_git."+f.OperatingSystem,
			"version", f.GitVersion, "minimum", MinimumGitVersion.String())
		return findings
	default:
		add(fault.Healthy, Healthy, "doctor.git.ok", "", "version", version.String())
	}

	if f.UserName == "" || f.UserEmail == "" {
		add(fault.GitIdentityMissing, Blocking, "doctor.identity.missing", "doctor.remedy.set_identity")
	} else {
		add(fault.Healthy, Healthy, "doctor.identity.ok", "", "name", f.UserName, "email", f.UserEmail)
	}

	if c := f.Configuration; c != nil {
		switch {
		case c.Problem != nil:
			// The fault itself says which file, key and line: shown as is.
			findings = append(findings, Finding{Code: c.Problem.Code, Severity: Blocking, Key: c.Problem.Key,
				Remedy: "doctor.remedy.fix_config", Params: c.Problem.Params})
		case c.Organization != "":
			add(fault.Healthy, Healthy, "doctor.policy.active", "",
				"organization", c.Organization, "count", strconv.Itoa(c.Locked), "contact", c.Contact)
		}
	}

	if f.OperatingSystem == "windows" && !f.LongPathsEnabled {
		add(fault.LongPathsDisabled, Warning, "doctor.longpaths.disabled", "doctor.remedy.enable_longpaths")
	}

	if !f.InsideRepository {
		add(fault.NotARepository, Warning, "doctor.repository.missing", "doctor.remedy.init_repository")
		return findings
	}
	add(fault.Healthy, Healthy, "doctor.repository.ok", "")

	if f.OperationInProgress != "" {
		add(fault.OperationInProgress, Blocking, "doctor.operation.in_progress",
			"doctor.remedy.finish_"+f.OperationInProgress, "operation", f.OperationInProgress)
	}
	if f.DetachedHead {
		add(fault.DetachedHead, Warning, "doctor.head.detached", "doctor.remedy.create_branch")
	}
	if !f.HasRemote {
		remote := f.Remote
		if remote == "" {
			remote = "origin"
		}
		add(fault.NoOriginRemote, Warning, "doctor.remote.missing", "doctor.remedy.add_remote", "remote", remote)
	}
	if f.HasRemote && f.RemoteNotGitHub {
		remote := f.Remote
		if remote == "" {
			remote = "origin"
		}
		add(fault.RemoteNotGitHub, Warning, "doctor.remote.not_github", "doctor.remedy.local_only", "remote", remote)
	}
	for _, remote := range f.MirrorRemotes {
		// `git push --mirror` would also push the local checkpoints (V13).
		add(fault.MirrorRemote, Warning, "doctor.remote.mirror", "doctor.remedy.disable_mirror", "remote", remote)
	}
	// GitHub problems are warnings: local work (commit, undo) still works.
	if gh := f.GitHub; gh != nil {
		params := []string{"host", gh.Host, "repository", gh.Repository, "source", gh.TokenSource}
		switch gh.Access {
		case GitHubAccessGranted:
			add(fault.Healthy, Healthy, "doctor.github.ok", "", params...)
		case GitHubNoToken:
			add(fault.GitHubNoToken, Warning, "doctor.github.no_token", "doctor.remedy.github_login", params...)
		case GitHubTokenRejected:
			add(fault.GitHubTokenRejected, Warning, "doctor.github.token_rejected", "doctor.remedy.github_renew_token", params...)
		case GitHubRepositoryHidden:
			add(fault.GitHubRepositoryHidden, Warning, "doctor.github.repository_hidden", "doctor.remedy.github_check_access", params...)
		case GitHubUnreachable:
			add(fault.GitHubUnreachable, Warning, "doctor.github.unreachable", "doctor.remedy.github_status", params...)
		}
	}
	return findings
}

// Worst returns the highest severity among findings.
func Worst(findings []Finding) Severity {
	worst := Healthy
	for _, f := range findings {
		if f.Severity > worst {
			worst = f.Severity
		}
	}
	return worst
}
