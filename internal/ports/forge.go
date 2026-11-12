// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/core/settings"
)

// RepositoryInfo is what ship-it needs to know about a GitHub repository.
type RepositoryInfo struct {
	DefaultBranch string
	HTMLURL       string
	// Parent is the repository a fork was made from; nil when the
	// repository is no fork.
	Parent *ParentRepository
	// Private: only the people given access see the repository.
	Private bool
	// TokenScopes are the scopes of the classic token that read it
	// (X-OAuth-Scopes); nil when GitHub does not say, as for fine-grained
	// tokens and GitHub Apps.
	TokenScopes []string
}

// ParentRepository is the repository a fork was made from: pull requests
// of the fork open there.
type ParentRepository struct {
	Repository    forge.Repository
	DefaultBranch string
	// CloneURL (HTTPS) and SSHURL fetch it without a remote.
	CloneURL, SSHURL string
}

// PullRequest is an open pull request.
type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Draft  bool   `json:"draft"`
	// Labels are the names of its labels, when the API returned them.
	Labels []string `json:"labels,omitempty"`
	// State is "open" or "closed"; a merged pull request is closed and Merged.
	State      string `json:"state,omitempty"`
	Merged     bool   `json:"merged,omitempty"`
	HeadBranch string `json:"head_branch,omitempty"`
	// HeadSHA is the last commit of the pull request: what was merged.
	HeadSHA string `json:"head_sha,omitempty"`
	// Author is the login of who opened it, when the API returned it.
	Author string `json:"author,omitempty"`
}

// User is a GitHub account.
type User struct {
	Login string
	ID    int64
	// Name is the display name, empty when not set.
	Name string
}

// PullRequestSpec is a pull request to open.
type PullRequestSpec struct {
	Title string
	Body  string
	// Head and Base are branch names of the same repository; from a fork,
	// Head is "<owner of the fork>:<branch>".
	Head  string
	Base  string
	Draft bool
}

// Forge is the GitHub API (ADR-0006 for stacks, later).
type Forge interface {
	Repository(ctx context.Context, repo forge.Repository) (RepositoryInfo, error)
	// FindOpenPullRequest returns the open pull request whose head is branch:
	// a branch of repo, or "<owner>:<branch>" for a branch of a fork.
	FindOpenPullRequest(ctx context.Context, repo forge.Repository, branch string) (PullRequest, bool, error)
	CreatePullRequest(ctx context.Context, repo forge.Repository, spec PullRequestSpec) (PullRequest, error)
	// LatestPullRequest returns the most recent pull request of branch,
	// whatever its state.
	LatestPullRequest(ctx context.Context, repo forge.Repository, branch string) (PullRequest, bool, error)
	// MergedPullRequests returns up to limit recently merged pull requests.
	MergedPullRequests(ctx context.Context, repo forge.Repository, limit int) ([]PullRequest, error)
	// Labels returns every label of the repository.
	Labels(ctx context.Context, repo forge.Repository) ([]settings.Label, error)
	// AddLabels adds existing labels to a pull request, keeping its others.
	AddLabels(ctx context.Context, repo forge.Repository, number int, labels []string) error
	// RequestReviewers asks users (logins) and teams (slugs of the
	// repository's organisation) to review a pull request.
	RequestReviewers(ctx context.Context, repo forge.Repository, number int, users, teams []string) error
	// CurrentUser returns the account the token belongs to, on repo's host.
	CurrentUser(ctx context.Context, repo forge.Repository) (User, error)
	// CommitAuthorLogin returns the GitHub account of a commit's author, ""
	// when its email belongs to no account.
	CommitAuthorLogin(ctx context.Context, repo forge.Repository, sha string) (string, error)
	// ClosePullRequest leaves comment on the pull request, then closes it.
	ClosePullRequest(ctx context.Context, repo forge.Repository, number int, comment string) error
	// RemoveLabel takes a label off a pull request; already absent is fine.
	RemoveLabel(ctx context.Context, repo forge.Repository, number int, label string) error
	// CreateLabel creates a label.
	CreateLabel(ctx context.Context, repo forge.Repository, label settings.Label) error
	// UpdateLabel renames the label currently called name and sets its color
	// and description.
	UpdateLabel(ctx context.Context, repo forge.Repository, name string, label settings.Label) error
	// IssueTitle returns the title of an issue (or pull request) number.
	IssueTitle(ctx context.Context, repo forge.Repository, number int) (string, error)
	// FileContent returns a file of a repository at a commit.
	FileContent(ctx context.Context, repo forge.Repository, path, commit string) ([]byte, error)
	// BranchRules returns the active ruleset rules that apply to a branch of
	// that name, whether it exists or not.
	BranchRules(ctx context.Context, repo forge.Repository, branch string) ([]rules.Rule, error)
	// PullRequestContent returns a pull request with the messages of its
	// commits (the API lists up to 250) and its changed files (up to 3,000).
	PullRequestContent(ctx context.Context, repo forge.Repository, number int) (PullRequestContent, error)
	// OpenPullRequestPatches returns the open pull requests into base, up to
	// limit, newest first, with the patch of each file they change.
	OpenPullRequestPatches(ctx context.Context, repo forge.Repository, base string, limit int) ([]PullRequestPatches, error)
	// CheckRuns returns the latest check run of each check of a commit.
	CheckRuns(ctx context.Context, repo forge.Repository, commit string) ([]review.CheckRun, error)
	// CommitStatuses returns the latest status of each context of a commit.
	CommitStatuses(ctx context.Context, repo forge.Repository, commit string) ([]review.CommitStatus, error)
	// Reviews returns the submitted reviews of a pull request, oldest first.
	Reviews(ctx context.Context, repo forge.Repository, number int) ([]review.Review, error)
}

// PullRequestPatches is an open pull request and the patch of each file it
// changes, to find the lines it shares with another change.
type PullRequestPatches struct {
	PullRequest PullRequest
	// Head is the branch the pull request comes from.
	Head  string
	Title string
	Files []FilePatch
}

// FilePatch is one file of a pull request as GitHub lists it: Status is
// "added", "removed", "modified", "renamed"…; Patch is empty when GitHub
// leaves it out (a binary or a very large file).
type FilePatch struct {
	Path, PreviousPath, Status, Patch string
}

// PullRequestContent is what a pull request brings, as GitHub sees it.
type PullRequestContent struct {
	PullRequest    PullRequest
	CommitMessages []string
	Files          []FileStat
}

// Errors a Forge or Credentials implementation returns, possibly wrapped.
var (
	// ErrNotAuthenticated: no token was found for the host.
	ErrNotAuthenticated = errors.New("ports: no GitHub token found")
	// ErrTokenRejected: GitHub refused the token (expired, revoked, SSO).
	ErrTokenRejected = errors.New("ports: GitHub rejected the token")
	// ErrNotFound: the repository does not exist or the token cannot see it.
	ErrNotFound = errors.New("ports: not found on GitHub")
	// ErrRateLimited: the API rate limit is exhausted.
	ErrRateLimited = errors.New("ports: GitHub API rate limit exceeded")
	// ErrForbidden: the token may not read or do this (403, rate limit aside).
	ErrForbidden = errors.New("ports: GitHub refused access")
)

// UnreachableError: a request to GitHub got no answer at all (name
// resolution, proxy, refused connection, TLS, timeout): nothing reached the
// API, so no HTTP status says more.
type UnreachableError struct {
	Host string
	Err  error
}

func (e *UnreachableError) Error() string {
	return "ports: " + e.Host + " does not answer: " + e.Err.Error()
}

func (e *UnreachableError) Unwrap() error { return e.Err }

// Token is a GitHub token. It never prints its value: fmt, %v, %#v and slog
// all show [REDACTED]; Reveal is the only way to read it.
type Token struct {
	value string
	// Source names where the token came from, such as "GH_TOKEN" or "gh".
	Source string
}

// NewToken wraps a token value.
func NewToken(value, source string) Token { return Token{value: value, Source: source} }

// Reveal returns the token value, for the Authorization header only.
func (t Token) Reveal() string { return t.value }

// String implements fmt.Stringer.
func (Token) String() string { return "[REDACTED]" }

// GoString implements fmt.GoStringer.
func (Token) GoString() string { return "[REDACTED]" }

// LogValue implements slog.LogValuer.
func (t Token) LogValue() slog.Value {
	return slog.GroupValue(slog.String("value", "[REDACTED]"), slog.String("source", t.Source))
}

// Credentials finds a token for a GitHub host (ADR-0003).
type Credentials interface {
	// Token returns ErrNotAuthenticated when no source has a token.
	Token(ctx context.Context, host string) (Token, error)
}

// TokenForgetter is implemented by Credentials that cache tokens: Forget
// drops the token of host after GitHub rejected it, so that the next Token
// looks again (a refreshed gh token, a new SHIP_TOKEN).
type TokenForgetter interface {
	Forget(host string)
}
