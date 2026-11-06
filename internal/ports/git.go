// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"errors"
)

// ChangeKind is one side (index or worktree) of a status entry, using git's
// short-format letters: 'M' modified, 'T' type changed, 'A' added,
// 'D' deleted, 'R' renamed, 'C' copied, 'U' updated but unmerged, '.' unchanged.
type ChangeKind byte

// Change kinds reported by git status.
const (
	Unchanged   ChangeKind = '.'
	Modified    ChangeKind = 'M'
	TypeChanged ChangeKind = 'T'
	Added       ChangeKind = 'A'
	Deleted     ChangeKind = 'D'
	Renamed     ChangeKind = 'R'
	Copied      ChangeKind = 'C'
	Unmerged    ChangeKind = 'U'
)

// FileState says how a path differs from HEAD.
type FileState int

// File states derived from a status entry.
const (
	Tracked FileState = iota
	Untracked
	Ignored
	Conflicted
)

// FileChange is one entry of the working tree status.
type FileChange struct {
	Path string
	// OriginalPath is set for renames and copies.
	OriginalPath string
	Staged       ChangeKind
	Unstaged     ChangeKind
	State        FileState
	Submodule    bool
}

// BranchState describes HEAD and its upstream.
type BranchState struct {
	// Name is empty when HEAD is detached.
	Name     string
	Detached bool
	// Commit is empty before the first commit.
	Commit   string
	Upstream string
	Ahead    int
	Behind   int
}

// WorkingTreeStatus is a snapshot of `git status`.
type WorkingTreeStatus struct {
	Branch BranchState
	Files  []FileChange
}

// Clean reports whether nothing differs from HEAD, ignored files aside.
func (s WorkingTreeStatus) Clean() bool {
	for _, f := range s.Files {
		if f.State != Ignored {
			return false
		}
	}
	return true
}

// Git is everything ship-it needs from the git binary (ADR-0002).
type Git interface {
	// Version returns the git version, such as "2.52.0".
	Version(ctx context.Context) (string, error)
	// TopLevel returns the absolute path of the working tree root.
	TopLevel(ctx context.Context) (string, error)
	// GitDir returns the absolute path of the git directory.
	GitDir(ctx context.Context) (string, error)
	// Status returns the working tree status, untracked files included.
	Status(ctx context.Context) (WorkingTreeStatus, error)
	// DiffFromHead returns the unified diff of the working tree and the index
	// against HEAD, renames detected, paths unquoted.
	DiffFromHead(ctx context.Context) (string, error)
	// ReadWorkingFile returns the content of a file of the working tree,
	// relative to TopLevel. Files larger than limit bytes return ErrFileTooLarge.
	ReadWorkingFile(ctx context.Context, path string, limit int64) ([]byte, error)
	// ConfigValue returns the effective value of a git configuration key.
	ConfigValue(ctx context.Context, key string) (value string, found bool, err error)
	// ConfigMatching returns every key/value whose key matches a regular
	// expression, as `git config --get-regexp` does.
	ConfigMatching(ctx context.Context, keyPattern string) (map[string]string, error)
	// Remotes returns the names of the configured remotes.
	Remotes(ctx context.Context) ([]string, error)
	// OperationInProgress returns "merge", "rebase", "cherry-pick" or
	// "revert" while one is unfinished, or an empty string.
	OperationInProgress(ctx context.Context) (string, error)
	// LocalBranchExists reports whether refs/heads/<name> exists.
	LocalBranchExists(ctx context.Context, name string) (bool, error)
	// SwitchToNewBranch creates a branch at HEAD and switches to it, keeping
	// the working tree and the index as they are.
	SwitchToNewBranch(ctx context.Context, name string) error
	// StageAll stages every change of the working tree, ignored files excluded.
	StageAll(ctx context.Context) error
	// Commit records the index with message and returns the new commit id.
	// Hooks follow the user's git configuration, signing too unless opts
	// say otherwise.
	Commit(ctx context.Context, message string, opts CommitOptions) (string, error)
	// CommitPaths stages and commits only paths (files or directories,
	// relative to TopLevel); every other change stays as it is.
	CommitPaths(ctx context.Context, message string, paths []string, opts CommitOptions) (string, error)
	// RecentMessages returns the full message of the last limit commits of
	// HEAD, newest first; none before the first commit.
	RecentMessages(ctx context.Context, limit int) ([]string, error)

	// RemoteURL returns the URL of a remote as configured (remote.<name>.url),
	// before any insteadOf rewrite.
	RemoteURL(ctx context.Context, remote string) (string, error)
	// RemoteBranchExists reports whether refs/remotes/<remote>/<branch> exists.
	RemoteBranchExists(ctx context.Context, remote, branch string) (bool, error)
	// RecentSubjects returns the first line of the last limit commits of HEAD,
	// newest first; none before the first commit.
	RecentSubjects(ctx context.Context, limit int) ([]string, error)
	// CommitMessagesSince returns the full message of every commit in
	// base..HEAD, oldest first; with base "" only HEAD's.
	CommitMessagesSince(ctx context.Context, base string) ([]string, error)
	// CommitsBetween returns the commits of from..to (reachable from to, not
	// from from), oldest first, with their raw identities and messages.
	CommitsBetween(ctx context.Context, from, to string) ([]CommitRecord, error)
	// SignedCommits reports which of the commits ids carry a signature,
	// valid or not (verifying it is GitHub's job).
	SignedCommits(ctx context.Context, ids []string) (map[string]bool, error)
	// FilesAt lists the entries of directory dir ("" for the root) in the
	// tree of rev, as paths relative to the repository root.
	FilesAt(ctx context.Context, rev, dir string) ([]string, error)
	// ReadFileAt returns the content of path in the tree of rev; larger than
	// limit bytes returns ErrFileTooLarge.
	ReadFileAt(ctx context.Context, rev, path string, limit int64) ([]byte, error)
	// CommitPatches returns the patch of every commit (a merge: its conflict
	// resolution) reachable
	// from HEAD and from none of exclude (all of HEAD's history when exclude
	// is empty), oldest first: exactly what a push would send, a line added
	// then removed included, for the secret scan.
	CommitPatches(ctx context.Context, exclude []string) (string, error)
	// DiffStatSince returns the files changed in base...HEAD with their line
	// counts; with base "" those of HEAD alone. Renames count as delete + add.
	DiffStatSince(ctx context.Context, base string) ([]FileStat, error)
	// DiffStatBetween returns the files changed in base...head: the work of
	// one layer of a stack, whatever is checked out.
	DiffStatBetween(ctx context.Context, base, head string) ([]FileStat, error)
	// GeneratedPaths returns which of paths .gitattributes marks
	// linguist-generated (GitHub's convention for generated files).
	GeneratedPaths(ctx context.Context, paths []string) (map[string]bool, error)
	// CompareWith counts the commits of HEAD missing from ref (ahead) and of
	// ref missing from HEAD (behind).
	CompareWith(ctx context.Context, ref string) (ahead, behind int, err error)
	// RemoteDefaultBranch returns the branch refs/remotes/<remote>/HEAD points
	// to, as recorded by clone or `git remote set-head`, or "" when unknown.
	RemoteDefaultBranch(ctx context.Context, remote string) (string, error)
	// DeleteRemoteBranch deletes branch on remote only if it still points at
	// expected (--force-with-lease); someone else's push makes it fail.
	DeleteRemoteBranch(ctx context.Context, remote, branch, expected string) error
	// Push sends branch to remote under the same name and sets it as upstream.
	// Authentication is the user's git setup (SSH key, credential helper).
	// A non-empty lease forces the push, only if the remote branch still
	// points at lease (--force-with-lease): for a branch just rebased.
	Push(ctx context.Context, remote, branch, lease string) error
	// PushCommit sets branch on remote to commit, only if it still points at
	// expected, or does not exist when expected is "" (--force-with-lease):
	// an undo puts back what ship-it pushed, never someone else's push.
	PushCommit(ctx context.Context, remote, branch, commit, expected string) error
	// Rebase replays the current branch on onto, putting local changes aside
	// and back (--autostash). On conflict it aborts, leaving everything as it
	// was, and reports conflicted.
	Rebase(ctx context.Context, onto string) (conflicted bool, err error)
	// RebaseOnto replays the commits of branch that upstream lacks on onto
	// (git rebase --onto onto upstream branch), leaving HEAD on branch; the
	// working tree must be clean. On conflict it aborts and reports it.
	// The replayed commits are signed as opts say (ship.commits.sign).
	RebaseOnto(ctx context.Context, branch, onto, upstream string, opts CommitOptions) (conflicted bool, err error)
	// MergeBase returns the best common ancestor of two commits, "" if none.
	MergeBase(ctx context.Context, a, b string) (string, error)
	// RebaseWith is Rebase signing the replayed commits as opts say (its
	// Signoff is ignored: the commits keep their trailers).
	RebaseWith(ctx context.Context, onto string, opts CommitOptions) (conflicted bool, err error)
	// Merge merges ref into the current branch (a merge commit, or a
	// fast-forward), putting local changes aside and back (--autostash). On
	// conflict it aborts, leaving everything as it was, and reports
	// conflicted.
	Merge(ctx context.Context, ref string, opts CommitOptions) (conflicted bool, err error)

	// Fetch updates the remote-tracking branches of remote, pruning the gone.
	Fetch(ctx context.Context, remote string) error
	// FetchBranch fetches branch of the repository at url into ref, without
	// a remote (the parent of a fork that has no "upstream" remote).
	FetchBranch(ctx context.Context, url, branch, ref string) error
	// CommitOf returns the commit ref points to, or "" when it does not exist.
	CommitOf(ctx context.Context, ref string) (string, error)
	// IsAncestor reports whether ancestor is reachable from descendant.
	IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
	// LocalBranches maps every local branch to its commit.
	LocalBranches(ctx context.Context) (map[string]string, error)
	// SwitchBranch moves to an existing branch, or creates it from
	// <remote>/<branch> (git's guess). Local changes must be put aside first.
	SwitchBranch(ctx context.Context, branch string) error
	// FastForward moves the current branch to ref when no commit is lost.
	FastForward(ctx context.Context, ref string) error
	// Stash puts every local change aside, untracked files included.
	Stash(ctx context.Context, message string) error
	// StashAppliesCleanlyOn reports whether the latest stash would apply on
	// ref without conflict: a three-way merge computed without touching the
	// working tree, plus no untracked file colliding with a file of ref.
	StashAppliesCleanlyOn(ctx context.Context, ref string) (bool, error)
	// StashPop applies the latest stash and drops it.
	StashPop(ctx context.Context) error
	// RecreateBranch creates refs/heads/<branch> at commit; it fails when the
	// branch exists.
	RecreateBranch(ctx context.Context, branch, commit string) error

	// ScopedConfigValue reads a value from the repository or the global
	// configuration only, to restore exactly that scope later.
	ScopedConfigValue(ctx context.Context, key string, global bool) (value string, found bool, err error)
	// SetConfig writes a configuration value in the repository, or in the
	// user's global configuration.
	SetConfig(ctx context.Context, key, value string, global bool) error
	// UnsetConfig removes a configuration value; already absent is fine.
	UnsetConfig(ctx context.Context, key string, global bool) error
	// TrackedFiles lists the files of the index, relative to TopLevel.
	TrackedFiles(ctx context.Context) ([]string, error)
	// Untrack removes paths from the index, keeping the files on disk.
	Untrack(ctx context.Context, paths []string) error
	// Track adds paths to the index, even when ignored (to undo Untrack).
	Track(ctx context.Context, paths []string) error
	// WriteWorkingFile writes a file of the working tree, relative to TopLevel,
	// creating its missing parent directories.
	WriteWorkingFile(ctx context.Context, path string, content []byte) error
	// RemoveWorkingFile deletes a file of the working tree, and the parent
	// directories it leaves empty; already gone is fine.
	RemoveWorkingFile(ctx context.Context, path string) error

	// The methods below move refs and the index only; the files of the working
	// tree are never touched. Ref changes are compare-and-swap: they fail when
	// the ref no longer holds expected.

	// AttachHead points HEAD at refs/heads/<branch>.
	AttachHead(ctx context.Context, branch string) error
	// DetachHead points HEAD directly at commit.
	DetachHead(ctx context.Context, commit string) error
	// MoveBranch moves refs/heads/<branch> from expected to commit.
	MoveBranch(ctx context.Context, branch, commit, expected string) error
	// DeleteBranch removes refs/heads/<branch> if it still points at expected.
	DeleteBranch(ctx context.Context, branch, expected string) error
	// RenameBranch renames refs/heads/<from> to <to>, with its reflog and
	// settings, if from still points at expected; it fails when <to> exists.
	// HEAD follows when it was on from.
	RenameBranch(ctx context.Context, from, to, expected string) error
	// ResetIndex makes the index match commit, or empties it when commit is "".
	ResetIndex(ctx context.Context, commit string) error
}

// CommitOptions say how a commit is made beyond the user's git
// configuration (ship.commits.signoff, ship.commits.sign).
type CommitOptions struct {
	// Signoff adds a Signed-off-by trailer with the committer's identity
	// (git commit --signoff).
	Signoff bool
	Sign    SignMode
}

// SignMode says whether a commit is signed.
type SignMode string

// Sign modes.
const (
	// SignDefault follows commit.gpgSign.
	SignDefault SignMode = ""
	// SignAlways signs (-S), SignNever does not (--no-gpg-sign).
	SignAlways SignMode = "always"
	SignNever  SignMode = "never"
)

// CommitRecord is one commit as git records it: identities are the raw
// author and committer (no .mailmap), Message the raw message.
type CommitRecord struct {
	ID             string
	Parents        []string
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	Message        string
}

// FileStat is one line of `git diff --numstat`.
type FileStat struct {
	Path    string
	Added   int
	Deleted int
	// Binary files have no line counts.
	Binary bool
}

// ErrFileTooLarge is returned by ReadWorkingFile above its size limit.
var ErrFileTooLarge = errors.New("ports: file larger than the read limit")

// ErrNotARepository: git ran outside a repository. A Git implementation
// returns an error that matches it with errors.Is.
var ErrNotARepository = errors.New("ports: not inside a git repository")
