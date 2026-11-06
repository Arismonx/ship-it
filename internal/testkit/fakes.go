// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package testkit

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/ports"
)

// FakeGit is an in-memory ports.Git. Zero values describe a missing repository
// configuration; set only what a test needs.
type FakeGit struct {
	WorkingTree      ports.WorkingTreeStatus
	DiffText         string
	WorkingFiles     map[string]string
	ConfigValues     map[string]string
	MatchingValues   map[string]string
	RemoteNames      []string
	InProgress       string
	ExistingBranches map[string]bool
	RemoteURLs       map[string]string
	// RemoteBranches holds "remote/branch" names that exist.
	RemoteBranches map[string]bool
	// CommitMessages are the full messages of the branch's commits.
	CommitMessages []string
	// Range is what CommitsBetween returns; RangeAsked records its last range.
	Range      []ports.CommitRecord
	RangeAsked *string
	// Signed lists the commits SignedCommits reports as signed.
	Signed map[string]bool
	// Files is the tree of the default branch, by path (FilesAt, ReadFileAt).
	Files map[string]string
	// History is what RecentSubjects returns.
	History   []string
	DiffStats []ports.FileStat
	// PushErr fails PushCommit by branch.
	PushErr map[string]error
	// LayerStats answer DiffStatBetween by head ("refs/heads/feat/a").
	LayerStats map[string][]ports.FileStat
	// PushedDiff is the patch of the commits a push would send (CommitPatches).
	PushedDiff string
	// Generated lists paths marked linguist-generated.
	Generated map[string]bool
	// Comparisons maps a ref to {ahead, behind} for CompareWith.
	Comparisons map[string][2]int
	// DefaultBranch is what RemoteDefaultBranch returns.
	DefaultBranch string
	// Refs maps full ref names ("refs/heads/x", "origin/main") to commits.
	Refs map[string]string
	// Ancestors holds the "ancestor..descendant" pairs that IsAncestor accepts.
	Ancestors map[string]bool
	// FetchErr fails Fetch (offline); Fetched are the Refs a fetch updates.
	FetchErr error
	Fetched  map[string]string
	// Diverged makes FastForward fail.
	Diverged bool
	// StashConflicts makes StashAppliesCleanlyOn answer false.
	StashConflicts bool
	// ScopedConfig holds "local:key" and "global:key" values.
	ScopedConfig map[string]string
	// Tracked is what TrackedFiles returns.
	Tracked []string
	// RebaseConflicts makes Rebase conflict; Rebased is HEAD after a rebase.
	RebaseConflicts bool
	Rebased         string
	// MergeBases answers MergeBase by "a..b"; absent, the merge base is b.
	MergeBases map[string]string
	// MergeConflicts makes Merge conflict.
	MergeConflicts bool
	// Messages is what RecentMessages returns, newest first.
	Messages []string
	// Moved, when non-nil, records SwitchToNewBranch and Commit, and Status
	// reports them, as a real repository would to a flow that reads it again.
	Moved *ports.BranchState
	// Calls records mutating calls, in order, when non-nil.
	Calls *[]string
}

var _ ports.Git = FakeGit{}

func (f FakeGit) record(call string) {
	if f.Calls != nil {
		*f.Calls = append(*f.Calls, call)
	}
}

// Version implements ports.Git.
func (FakeGit) Version(context.Context) (string, error) { return "2.52.0", nil }

// TopLevel implements ports.Git.
func (FakeGit) TopLevel(context.Context) (string, error) { return "/repo", nil }

// GitDir implements ports.Git.
func (FakeGit) GitDir(context.Context) (string, error) { return "/repo/.git", nil }

// Status implements ports.Git.
func (f FakeGit) Status(context.Context) (ports.WorkingTreeStatus, error) {
	status := f.WorkingTree
	if f.Moved != nil {
		if f.Moved.Name != "" {
			status.Branch.Name, status.Branch.Detached = f.Moved.Name, false
		}
		if f.Moved.Commit != "" {
			status.Branch.Commit, status.Files = f.Moved.Commit, nil
		}
	}
	return status, nil
}

// DiffFromHead implements ports.Git.
func (f FakeGit) DiffFromHead(context.Context) (string, error) { return f.DiffText, nil }

// ReadWorkingFile implements ports.Git.
func (f FakeGit) ReadWorkingFile(_ context.Context, path string, limit int64) ([]byte, error) {
	content, ok := f.WorkingFiles[path]
	if !ok {
		return nil, errors.New("testkit: no such file " + path)
	}
	if int64(len(content)) > limit {
		return nil, ports.ErrFileTooLarge
	}
	return []byte(content), nil
}

// ConfigValue implements ports.Git.
func (f FakeGit) ConfigValue(_ context.Context, key string) (string, bool, error) {
	v, ok := f.ConfigValues[key]
	return v, ok, nil
}

// ConfigMatching implements ports.Git.
func (f FakeGit) ConfigMatching(context.Context, string) (map[string]string, error) {
	return f.MatchingValues, nil
}

// Remotes implements ports.Git.
func (f FakeGit) Remotes(context.Context) ([]string, error) { return f.RemoteNames, nil }

// OperationInProgress implements ports.Git.
func (f FakeGit) OperationInProgress(context.Context) (string, error) { return f.InProgress, nil }

// LocalBranchExists implements ports.Git.
func (f FakeGit) LocalBranchExists(_ context.Context, name string) (bool, error) {
	return f.ExistingBranches[name], nil
}

// SwitchToNewBranch implements ports.Git.
func (f FakeGit) SwitchToNewBranch(_ context.Context, name string) error {
	f.record("switch " + name)
	if f.Moved != nil {
		f.Moved.Name = name
	}
	return nil
}

// StageAll implements ports.Git.
func (f FakeGit) StageAll(context.Context) error {
	f.record("stage")
	return nil
}

// FakeCommitID is the id FakeGit.Commit returns.
const FakeCommitID = "c0ffee"

// Commit implements ports.Git.
func (f FakeGit) Commit(_ context.Context, message string, opts ports.CommitOptions) (string, error) {
	f.record("commit " + message + optionsNote(opts))
	if f.Moved != nil {
		f.Moved.Commit = FakeCommitID
	}
	return FakeCommitID, nil
}

// RemoteURL implements ports.Git.
func (f FakeGit) RemoteURL(_ context.Context, remote string) (string, error) {
	if url, ok := f.RemoteURLs[remote]; ok {
		return url, nil
	}
	return "", errors.New("testkit: no such remote " + remote)
}

// RemoteBranchExists implements ports.Git.
func (f FakeGit) RemoteBranchExists(_ context.Context, remote, branch string) (bool, error) {
	return f.RemoteBranches[remote+"/"+branch], nil
}

// RecentSubjects implements ports.Git.
func (f FakeGit) RecentSubjects(context.Context, int) ([]string, error) {
	return f.History, nil
}

// CommitMessagesSince implements ports.Git.
func (f FakeGit) CommitMessagesSince(context.Context, string) ([]string, error) {
	return f.CommitMessages, nil
}

// SignedCommits implements ports.Git with Signed.
func (f FakeGit) SignedCommits(_ context.Context, ids []string) (map[string]bool, error) {
	signed := map[string]bool{}
	for _, id := range ids {
		if f.Signed[id] {
			signed[id] = true
		}
	}
	return signed, nil
}

// RenameBranch implements ports.Git; with Moved, Status reports the new name.
func (f FakeGit) RenameBranch(_ context.Context, from, to, expected string) error {
	f.record("rename " + from + " " + to + " at " + expected)
	if f.Moved != nil {
		f.Moved.Name = to
	}
	if commit, ok := f.Refs["refs/heads/"+from]; ok {
		delete(f.Refs, "refs/heads/"+from)
		f.Refs["refs/heads/"+to] = commit
	}
	if f.ExistingBranches[from] {
		delete(f.ExistingBranches, from)
		f.ExistingBranches[to] = true
	}
	return nil
}

// CommitsBetween implements ports.Git with Range.
func (f FakeGit) CommitsBetween(_ context.Context, from, to string) ([]ports.CommitRecord, error) {
	if f.RangeAsked != nil {
		*f.RangeAsked = from + ".." + to
	}
	return f.Range, nil
}

// FilesAt implements ports.Git with the paths of Files directly under dir.
func (f FakeGit) FilesAt(_ context.Context, _, dir string) ([]string, error) {
	var paths []string
	for path := range f.Files {
		rel, under := path, true
		if dir != "" {
			rel, under = strings.CutPrefix(path, dir+"/")
		}
		if under && rel != "" && !strings.Contains(rel, "/") {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

// ReadFileAt implements ports.Git with Files.
func (f FakeGit) ReadFileAt(_ context.Context, _, path string, limit int64) ([]byte, error) {
	content, ok := f.Files[path]
	if !ok {
		return nil, errors.New("testkit: no such file in the tree " + path)
	}
	if int64(len(content)) > limit {
		return nil, ports.ErrFileTooLarge
	}
	return []byte(content), nil
}

// CommitPatches implements ports.Git with PushedDiff.
func (f FakeGit) CommitPatches(context.Context, []string) (string, error) { return f.PushedDiff, nil }

// DiffStatSince implements ports.Git.
func (f FakeGit) DiffStatSince(context.Context, string) ([]ports.FileStat, error) {
	return f.DiffStats, nil
}

// DiffStatBetween implements ports.Git with LayerStats by head, else DiffStats.
func (f FakeGit) DiffStatBetween(_ context.Context, _, head string) ([]ports.FileStat, error) {
	if stats, ok := f.LayerStats[head]; ok {
		return stats, nil
	}
	return f.DiffStats, nil
}

// GeneratedPaths implements ports.Git.
func (f FakeGit) GeneratedPaths(_ context.Context, paths []string) (map[string]bool, error) {
	generated := map[string]bool{}
	for _, p := range paths {
		if f.Generated[p] {
			generated[p] = true
		}
	}
	return generated, nil
}

// CompareWith implements ports.Git with Comparisons[ref] = {ahead, behind};
// an unlisted ref is neither ahead nor behind.
func (f FakeGit) CompareWith(_ context.Context, ref string) (int, int, error) {
	counts := f.Comparisons[ref]
	return counts[0], counts[1], nil
}

// Pushed reports whether a push was recorded in calls.
func Pushed(calls []string) bool {
	return slices.ContainsFunc(calls, func(c string) bool { return strings.HasPrefix(c, "push ") })
}

// RemoteDefaultBranch implements ports.Git.
func (f FakeGit) RemoteDefaultBranch(context.Context, string) (string, error) {
	return f.DefaultBranch, nil
}

// DeleteRemoteBranch implements ports.Git.
func (f FakeGit) DeleteRemoteBranch(_ context.Context, remote, branch, expected string) error {
	f.record("delete-remote " + remote + " " + branch + " at " + expected)
	return nil
}

// Push implements ports.Git.
func (f FakeGit) Push(_ context.Context, remote, branch, lease string) error {
	if lease != "" {
		f.record("push " + remote + " " + branch + " lease " + lease)
		return nil
	}
	f.record("push " + remote + " " + branch)
	return nil
}

// PushCommit implements ports.Git; PushErr fails it, as a lease does.
func (f FakeGit) PushCommit(_ context.Context, remote, branch, commit, expected string) error {
	f.record("push-back " + remote + " " + branch + " " + commit + " lease " + expected)
	return f.PushErr[branch]
}

// Rebase implements ports.Git; RebaseConflicts makes it conflict, and
// Rebased is the commit HEAD moves to.
func (f FakeGit) Rebase(_ context.Context, onto string) (bool, error) {
	f.record("rebase " + onto)
	if f.RebaseConflicts {
		return true, nil
	}
	if f.Moved != nil {
		f.Moved.Commit = f.Rebased
	}
	return false, nil
}

// RebaseOnto implements ports.Git: RebaseConflicts makes it conflict, and
// Refs["refs/heads/<branch>"] moves to Rebased when set.
func (f FakeGit) RebaseOnto(_ context.Context, branch, onto, upstream string, _ ports.CommitOptions) (bool, error) {
	f.record("rebase-onto " + branch + " " + onto + " " + upstream)
	if f.RebaseConflicts {
		return true, nil
	}
	if f.Rebased != "" && f.Refs != nil {
		f.Refs["refs/heads/"+branch] = f.Rebased + "-" + branch
	}
	if f.Moved != nil {
		f.Moved.Name = branch
	}
	return false, nil
}

// RebaseWith implements ports.Git like Rebase, noting the signing mode.
func (f FakeGit) RebaseWith(ctx context.Context, onto string, opts ports.CommitOptions) (bool, error) {
	if opts.Sign != ports.SignDefault {
		f.record("rebase-sign " + string(opts.Sign))
	}
	return f.Rebase(ctx, onto)
}

// Merge implements ports.Git; MergeConflicts makes it conflict, and
// Rebased is the commit HEAD moves to.
func (f FakeGit) Merge(_ context.Context, ref string, opts ports.CommitOptions) (bool, error) {
	f.record("merge " + ref + optionsNote(opts))
	if f.MergeConflicts {
		return true, nil
	}
	if f.Moved != nil {
		f.Moved.Commit = f.Rebased
	}
	return false, nil
}

// MergeBase implements ports.Git with MergeBases ("a..b"), else a.
func (f FakeGit) MergeBase(_ context.Context, a, b string) (string, error) {
	if base, ok := f.MergeBases[a+".."+b]; ok {
		return base, nil
	}
	return b, nil
}

// RecentMessages implements ports.Git with Messages.
func (f FakeGit) RecentMessages(context.Context, int) ([]string, error) { return f.Messages, nil }

// optionsNote is what a recorded commit adds for non-default options.
func optionsNote(opts ports.CommitOptions) string {
	note := ""
	if opts.Signoff {
		note += " [signoff]"
	}
	if opts.Sign != ports.SignDefault {
		note += " [sign " + string(opts.Sign) + "]"
	}
	return note
}

// FetchBranch implements ports.Git; tests give the fetched ref in Refs.
func (f FakeGit) FetchBranch(_ context.Context, url, branch, ref string) error {
	f.record("fetch-url " + url + " " + branch + " into " + ref)
	return f.FetchErr
}

// Fetch implements ports.Git.
func (f FakeGit) Fetch(_ context.Context, remote string) error {
	f.record("fetch " + remote)
	for ref, commit := range f.Fetched {
		f.Refs[ref] = commit
	}
	return f.FetchErr
}

// CommitOf implements ports.Git with Refs.
func (f FakeGit) CommitOf(_ context.Context, ref string) (string, error) {
	return f.Refs[ref], nil
}

// IsAncestor implements ports.Git: Ancestors holds "ancestor..descendant".
func (f FakeGit) IsAncestor(_ context.Context, ancestor, descendant string) (bool, error) {
	return ancestor == descendant || f.Ancestors[ancestor+".."+descendant], nil
}

// LocalBranches implements ports.Git: the branches of Refs under refs/heads/.
func (f FakeGit) LocalBranches(context.Context) (map[string]string, error) {
	branches := map[string]string{}
	for ref, commit := range f.Refs {
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			branches[name] = commit
		}
	}
	return branches, nil
}

// SwitchBranch implements ports.Git.
func (f FakeGit) SwitchBranch(_ context.Context, branch string) error {
	f.record("switch-to " + branch)
	if f.Moved != nil {
		f.Moved.Name = branch
	}
	return nil
}

// FastForward implements ports.Git.
func (f FakeGit) FastForward(_ context.Context, ref string) error {
	f.record("fast-forward " + ref)
	if f.Diverged {
		return errors.New("testkit: not possible to fast-forward")
	}
	return nil
}

// Stash implements ports.Git.
func (f FakeGit) Stash(context.Context, string) error {
	f.record("stash")
	return nil
}

// StashAppliesCleanlyOn implements ports.Git.
func (f FakeGit) StashAppliesCleanlyOn(context.Context, string) (bool, error) {
	return !f.StashConflicts, nil
}

// StashPop implements ports.Git.
func (f FakeGit) StashPop(context.Context) error {
	f.record("stash-pop")
	return nil
}

// RecreateBranch implements ports.Git.
func (f FakeGit) RecreateBranch(_ context.Context, branch, commit string) error {
	f.record("recreate " + branch + " at " + commit)
	return nil
}

// AttachHead implements ports.Git.
func (f FakeGit) AttachHead(_ context.Context, branch string) error {
	f.record("attach " + branch)
	return nil
}

// DetachHead implements ports.Git.
func (f FakeGit) DetachHead(_ context.Context, commit string) error {
	f.record("detach " + commit)
	return nil
}

// MoveBranch implements ports.Git.
func (f FakeGit) MoveBranch(_ context.Context, branch, commit, expected string) error {
	f.record("move " + branch + " " + expected + ".." + commit)
	return nil
}

// DeleteBranch implements ports.Git.
func (f FakeGit) DeleteBranch(_ context.Context, branch, expected string) error {
	f.record("delete " + branch + " at " + expected)
	return nil
}

// ResetIndex implements ports.Git.
func (f FakeGit) ResetIndex(_ context.Context, commit string) error {
	f.record("reset-index " + commit)
	return nil
}

// ScopedConfigValue implements ports.Git with ScopedConfig ("local:key" or
// "global:key").
func (f FakeGit) ScopedConfigValue(_ context.Context, key string, global bool) (string, bool, error) {
	v, ok := f.ScopedConfig[scopeName(global)+":"+key]
	return v, ok, nil
}

func scopeName(global bool) string {
	if global {
		return "global"
	}
	return "local"
}

// SetConfig implements ports.Git.
func (f FakeGit) SetConfig(_ context.Context, key, value string, global bool) error {
	f.record("config " + scopeName(global) + " " + key + "=" + value)
	return nil
}

// UnsetConfig implements ports.Git.
func (f FakeGit) UnsetConfig(_ context.Context, key string, global bool) error {
	f.record("config " + scopeName(global) + " unset " + key)
	return nil
}

// TrackedFiles implements ports.Git.
func (f FakeGit) TrackedFiles(context.Context) ([]string, error) { return f.Tracked, nil }

// Untrack implements ports.Git.
func (f FakeGit) Untrack(_ context.Context, paths []string) error {
	f.record("untrack " + strings.Join(paths, ","))
	return nil
}

// Track implements ports.Git.
func (f FakeGit) Track(_ context.Context, paths []string) error {
	f.record("track " + strings.Join(paths, ","))
	return nil
}

// WriteWorkingFile implements ports.Git.
func (f FakeGit) WriteWorkingFile(_ context.Context, path string, content []byte) error {
	f.record("write " + path + " " + strings.ReplaceAll(string(content), "\n", `\n`))
	if f.WorkingFiles != nil {
		f.WorkingFiles[path] = string(content) // read back as a real tree would
	}
	return nil
}

// RemoveWorkingFile implements ports.Git.
func (f FakeGit) RemoveWorkingFile(_ context.Context, path string) error {
	f.record("remove " + path)
	return nil
}

// CommitPaths implements ports.Git.
func (f FakeGit) CommitPaths(_ context.Context, message string, paths []string, opts ports.CommitOptions) (string, error) {
	f.record("commit-only " + strings.Join(paths, ",") + " " + message + optionsNote(opts))
	if f.Moved != nil {
		f.Moved.Commit = FakeCommitID
	}
	return FakeCommitID, nil
}

// HealthyRepository returns a FakeGit that passes ship doctor, on branch,
// with the given files modified.
func HealthyRepository(branch string, modified ...string) FakeGit {
	g := FakeGit{
		WorkingTree:  ports.WorkingTreeStatus{Branch: ports.BranchState{Name: branch, Commit: "abc"}},
		ConfigValues: map[string]string{"user.name": "Ada", "user.email": "ada@example.invalid"},
		RemoteNames:  []string{"origin"},
		RemoteURLs:   map[string]string{"origin": "git@github.com:acme/shop.git"},
		Moved:        &ports.BranchState{},
	}
	for _, path := range modified {
		g.WorkingTree.Files = append(g.WorkingTree.Files, ports.FileChange{Path: path, State: ports.Tracked, Unstaged: ports.Modified})
	}
	return g
}

// FakeCheckpointStore is an in-memory ports.CheckpointStore.
type FakeCheckpointStore struct {
	Checkpoints []ports.Checkpoint // newest first
	Restored    string
	created     int
}

var _ ports.CheckpointStore = (*FakeCheckpointStore)(nil)

// Create implements ports.CheckpointStore with ids "a000…", "b000…"…
func (s *FakeCheckpointStore) Create(_ context.Context, label string) (ports.Checkpoint, error) {
	s.created++
	cp := ports.Checkpoint{ID: string(rune('a'+s.created-1)) + strings.Repeat("0", 39), Label: label}
	s.Checkpoints = append([]ports.Checkpoint{cp}, s.Checkpoints...)
	return cp, nil
}

// List implements ports.CheckpointStore.
func (s *FakeCheckpointStore) List(_ context.Context, limit int) ([]ports.Checkpoint, error) {
	if limit < len(s.Checkpoints) {
		return s.Checkpoints[:limit], nil
	}
	return s.Checkpoints, nil
}

// RestoreFiles implements ports.CheckpointStore.
func (s *FakeCheckpointStore) RestoreFiles(_ context.Context, id string) error {
	s.Restored = id
	return nil
}

// Remove implements ports.CheckpointStore.
func (s *FakeCheckpointStore) Remove(_ context.Context, ids []string) error {
	s.Checkpoints = slices.DeleteFunc(s.Checkpoints, func(c ports.Checkpoint) bool { return slices.Contains(ids, c.ID) })
	return nil
}

// FakeForge is an in-memory ports.Forge for the repository acme/shop.
type FakeForge struct {
	// Err, when set, is returned by every call.
	Err           error
	DefaultBranch string
	Open          map[string]ports.PullRequest // by head branch
	Created       []ports.PullRequestSpec
	// Latest is the most recent pull request of a branch, any state.
	Latest map[string]ports.PullRequest
	// Merged are recently merged pull requests.
	Merged []ports.PullRequest
	// Closed records ClosePullRequest.
	Closed []int
	// User is the token's account; empty means no token.
	User ports.User
	// RepositoryLabels are the labels the repository has.
	RepositoryLabels []settings.Label
	// LabelCalls records CreateLabel and UpdateLabel.
	LabelCalls []string
	// Added records AddLabels by pull request number.
	Added map[int][]string
	// IssueTitles are the titles of existing issues, by number.
	IssueTitles map[int]string
	// Runs, Statuses and ReviewList answer CheckRuns, CommitStatuses and
	// Reviews, whatever the commit or pull request.
	Runs       []review.CheckRun
	Statuses   []review.CommitStatus
	ReviewList []review.Review
	// Contents are files of other repositories, by "owner/repo/path@commit".
	Contents map[string]string
	// ContentCalls counts FileContent calls.
	ContentCalls int
	// PullRequests are what PullRequestContent returns, by number.
	PullRequests map[int]ports.PullRequestContent
	// Rules are the ruleset rules of each branch name; RulesErr fails
	// BranchRules, and RulesAsked records the names asked for.
	Rules      map[string][]rules.Rule
	RulesErr   error
	RulesAsked []string
	// Parent makes every other repository a fork of it; DefaultBranches
	// are the default branches by "owner/name".
	Parent          *ports.ParentRepository
	DefaultBranches map[string]string
	// CreatedOn and ClosedOn record the repository ("owner/name") of each
	// CreatePullRequest and ClosePullRequest.
	CreatedOn, ClosedOn []string
	// Reviewers records RequestReviewers: "#1 ada,@backend".
	Reviewers []string
	// ReviewersErr fails RequestReviewers only.
	ReviewersErr error
}

// RequestReviewers implements ports.Forge.
func (f *FakeForge) RequestReviewers(_ context.Context, _ forge.Repository, number int, users, teams []string) error {
	if f.ReviewersErr != nil {
		return f.ReviewersErr
	}
	who := slices.Clone(users)
	for _, team := range teams {
		who = append(who, "@"+team)
	}
	f.Reviewers = append(f.Reviewers, "#"+strconv.Itoa(number)+" "+strings.Join(who, ","))
	return f.Err
}

// BranchRules implements ports.Forge with Rules.
func (f *FakeForge) BranchRules(_ context.Context, _ forge.Repository, branch string) ([]rules.Rule, error) {
	f.RulesAsked = append(f.RulesAsked, branch)
	if f.RulesErr != nil {
		return nil, f.RulesErr
	}
	return f.Rules[branch], f.Err
}

// PullRequestContent implements ports.Forge with PullRequests.
func (f *FakeForge) PullRequestContent(_ context.Context, _ forge.Repository, number int) (ports.PullRequestContent, error) {
	if f.Err != nil {
		return ports.PullRequestContent{}, f.Err
	}
	content, ok := f.PullRequests[number]
	if !ok {
		return ports.PullRequestContent{}, ports.ErrNotFound
	}
	return content, nil
}

// CheckRuns implements ports.Forge.
func (f *FakeForge) CheckRuns(context.Context, forge.Repository, string) ([]review.CheckRun, error) {
	return f.Runs, f.Err
}

// CommitStatuses implements ports.Forge.
func (f *FakeForge) CommitStatuses(context.Context, forge.Repository, string) ([]review.CommitStatus, error) {
	return f.Statuses, f.Err
}

// Reviews implements ports.Forge.
func (f *FakeForge) Reviews(context.Context, forge.Repository, int) ([]review.Review, error) {
	return f.ReviewList, f.Err
}

// FileContent implements ports.Forge with Contents.
func (f *FakeForge) FileContent(_ context.Context, repo forge.Repository, path, commit string) ([]byte, error) {
	f.ContentCalls++
	if f.Err != nil {
		return nil, f.Err
	}
	content, ok := f.Contents[repo.Owner+"/"+repo.Name+"/"+path+"@"+commit]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return []byte(content), nil
}

var _ ports.Forge = (*FakeForge)(nil)

// Repository implements ports.Forge.
func (f *FakeForge) Repository(_ context.Context, repo forge.Repository) (ports.RepositoryInfo, error) {
	branch := f.DefaultBranch
	if b, ok := f.DefaultBranches[repo.Slug()]; ok {
		branch = b
	}
	if branch == "" {
		branch = "main"
	}
	info := ports.RepositoryInfo{DefaultBranch: branch}
	if p := f.Parent; p != nil {
		if repo.Slug() == p.Repository.Slug() {
			info.DefaultBranch = p.DefaultBranch
		} else {
			info.Parent = p
		}
	}
	return info, f.Err
}

// FindOpenPullRequest implements ports.Forge.
func (f *FakeForge) FindOpenPullRequest(_ context.Context, _ forge.Repository, branch string) (ports.PullRequest, bool, error) {
	pr, ok := f.Open[branch]
	return pr, ok, f.Err
}

// LatestPullRequest implements ports.Forge: Latest, then Open.
func (f *FakeForge) LatestPullRequest(_ context.Context, _ forge.Repository, branch string) (ports.PullRequest, bool, error) {
	if pr, ok := f.Latest[branch]; ok {
		return pr, true, f.Err
	}
	pr, ok := f.Open[branch]
	return pr, ok, f.Err
}

// MergedPullRequests implements ports.Forge.
func (f *FakeForge) MergedPullRequests(context.Context, forge.Repository, int) ([]ports.PullRequest, error) {
	return f.Merged, f.Err
}

// CreatePullRequest implements ports.Forge with numbers 1, 2…
func (f *FakeForge) CreatePullRequest(_ context.Context, repo forge.Repository, spec ports.PullRequestSpec) (ports.PullRequest, error) {
	if f.Err != nil {
		return ports.PullRequest{}, f.Err
	}
	f.Created = append(f.Created, spec)
	f.CreatedOn = append(f.CreatedOn, repo.Slug())
	n := len(f.Created)
	pr := ports.PullRequest{Number: n, URL: "https://" + repo.Host + "/" + repo.Slug() + "/pull/" + strconv.Itoa(n), Draft: spec.Draft, Author: f.User.Login}
	if f.Open == nil {
		f.Open = map[string]ports.PullRequest{}
	}
	f.Open[spec.Head] = pr
	return pr, nil
}

// Labels implements ports.Forge.
func (f *FakeForge) Labels(context.Context, forge.Repository) ([]settings.Label, error) {
	return f.RepositoryLabels, f.Err
}

// CurrentUser implements ports.Forge.
func (f *FakeForge) CurrentUser(context.Context, forge.Repository) (ports.User, error) {
	if f.User.Login == "" {
		return ports.User{}, ports.ErrNotAuthenticated
	}
	return f.User, f.Err
}

// ClosePullRequest implements ports.Forge.
func (f *FakeForge) ClosePullRequest(_ context.Context, repo forge.Repository, number int, comment string) error {
	f.Closed = append(f.Closed, number)
	f.ClosedOn = append(f.ClosedOn, repo.Slug())
	return f.Err
}

// RemoveLabel implements ports.Forge.
func (f *FakeForge) RemoveLabel(_ context.Context, _ forge.Repository, number int, label string) error {
	f.LabelCalls = append(f.LabelCalls, "remove "+label+" from #"+strconv.Itoa(number))
	return f.Err
}

// IssueTitle implements ports.Forge with IssueTitles.
func (f *FakeForge) IssueTitle(_ context.Context, _ forge.Repository, number int) (string, error) {
	if title, ok := f.IssueTitles[number]; ok {
		return title, f.Err
	}
	return "", ports.ErrNotFound
}

// CreateLabel implements ports.Forge.
func (f *FakeForge) CreateLabel(_ context.Context, _ forge.Repository, label settings.Label) error {
	if f.Err != nil {
		return f.Err
	}
	f.LabelCalls = append(f.LabelCalls, "create "+label.Name)
	f.RepositoryLabels = append(f.RepositoryLabels, label)
	return nil
}

// UpdateLabel implements ports.Forge.
func (f *FakeForge) UpdateLabel(_ context.Context, _ forge.Repository, name string, label settings.Label) error {
	if f.Err != nil {
		return f.Err
	}
	f.LabelCalls = append(f.LabelCalls, "update "+name+" → "+label.Name)
	for i, l := range f.RepositoryLabels {
		if strings.EqualFold(l.Name, name) {
			f.RepositoryLabels[i] = label
		}
	}
	return nil
}

// LabelNames builds labels with only a name, for tests.
func LabelNames(names ...string) []settings.Label {
	labels := make([]settings.Label, len(names))
	for i, n := range names {
		labels[i] = settings.Label{Name: n}
	}
	return labels
}

// AddLabels implements ports.Forge.
func (f *FakeForge) AddLabels(_ context.Context, _ forge.Repository, number int, labels []string) error {
	if f.Added == nil {
		f.Added = map[int][]string{}
	}
	f.Added[number] = append(f.Added[number], labels...)
	return f.Err
}

// FakeConfigFiles is an in-memory ports.ConfigFiles.
type FakeConfigFiles struct {
	User       string // content of the user file, "" when absent
	Repository string // content of .github/ship.json, "" when absent
	Policy     string // content of the system policy, "" when absent
	// Presets are preset files by path, relative to their root.
	Presets map[string]string
	// Cache holds stored GitHub presets by key.
	Cache map[string]string
	// NoRepository simulates running outside a repository.
	NoRepository bool
	// Writes counts the files ReplaceConfigFile wrote or removed.
	Writes int
}

// ReadPreset implements ports.ConfigFiles with Presets.
func (f *FakeConfigFiles) ReadPreset(_ context.Context, _ config.Layer, path string) (config.Layer, error) {
	content, ok := f.Presets[path]
	if !ok {
		return config.Layer{}, errors.New("testkit: no preset " + path)
	}
	return config.Layer{Name: path, Path: path, Kind: config.KindPreset, Data: []byte(content)}, nil
}

// CachedPreset implements ports.ConfigFiles.
func (f *FakeConfigFiles) CachedPreset(_ context.Context, key string) ([]byte, bool) {
	content, ok := f.Cache[key]
	return []byte(content), ok
}

// StorePreset implements ports.ConfigFiles.
func (f *FakeConfigFiles) StorePreset(_ context.Context, key string, data []byte) error {
	if f.Cache == nil {
		f.Cache = map[string]string{}
	}
	f.Cache[key] = string(data)
	return nil
}

var _ ports.ConfigFiles = (*FakeConfigFiles)(nil)

// Layers implements ports.ConfigFiles.
func (f *FakeConfigFiles) Layers(context.Context) ([]config.Layer, error) {
	var layers []config.Layer
	if f.User != "" {
		layers = append(layers, config.Layer{Name: "user", Path: "/home/ada/.config/ship-it/config.json", Data: []byte(f.User)})
	}
	if f.Repository != "" {
		layers = append(layers, config.Layer{Name: "repository", Path: "/repo/.github/ship.json", Data: []byte(f.Repository)})
	}
	if f.Policy != "" {
		layers = append(layers, config.Layer{Name: config.PolicyLayerName, Path: "/etc/ship-it/policy.json", Data: []byte(f.Policy)})
	}
	return layers, nil
}

// CreateRepositoryFile implements ports.ConfigFiles.
func (f *FakeConfigFiles) CreateRepositoryFile(_ context.Context, data []byte) (string, error) {
	if f.Repository != "" {
		return "/repo/.github/ship.json", ports.ErrConfigExists
	}
	f.Repository = string(data)
	return "/repo/.github/ship.json", nil
}

// Paths of the fake configuration files.
const (
	FakeUserConfigPath       = "/home/ada/.config/ship-it/config.json"
	FakeRepositoryConfigPath = "/repo/.github/ship.json"
)

func (f *FakeConfigFiles) scope(scope ports.ConfigScope) (*string, string, error) {
	switch {
	case scope == ports.ScopeUser:
		return &f.User, FakeUserConfigPath, nil
	case scope == ports.ScopeRepository && !f.NoRepository:
		return &f.Repository, FakeRepositoryConfigPath, nil
	}
	return nil, "", ports.ErrNoConfigScope
}

// ReadConfigFile implements ports.ConfigFiles.
func (f *FakeConfigFiles) ReadConfigFile(_ context.Context, scope ports.ConfigScope) (ports.ConfigFile, error) {
	content, path, err := f.scope(scope)
	if err != nil {
		return ports.ConfigFile{}, err
	}
	return ports.ConfigFile{Path: path, Data: []byte(*content), Exists: *content != ""}, nil
}

// ReplaceConfigFile implements ports.ConfigFiles.
func (f *FakeConfigFiles) ReplaceConfigFile(_ context.Context, change ports.ConfigFileChange) (string, error) {
	content, path, err := f.scope(change.Scope)
	if err != nil {
		return "", err
	}
	if (*content != "") != change.OldExists || (change.OldExists && *content != string(change.Old)) {
		return path, ports.ErrConfigChanged
	}
	*content = ""
	if change.NewExists {
		*content = string(change.New)
	}
	f.Writes++
	return path, nil
}

// FakeRunner is a ports.CommandRunner: every command passes unless Failing
// names it ("npm run test" → output); Missing programs are not installed.
// Outputs are the outputs of passing commands; Respond, when set, answers
// first, from the command line and its standard input (a fake commitlint).
type FakeRunner struct {
	Failing map[string]string
	Missing map[string]bool
	Outputs map[string]string
	Respond func(line string, stdin []byte) (ports.CommandResult, bool)
	// Printing names commands that exit 0 but print output (gofmt -l).
	Printing map[string]string
	Calls    []string
	// Inputs records the standard input of each RunWithInput call.
	Inputs []string
}

var _ ports.CommandRunner = (*FakeRunner)(nil)

// Run implements ports.CommandRunner.
func (r *FakeRunner) Run(ctx context.Context, command []string) (ports.CommandResult, error) {
	return r.run(command, nil)
}

// RunWithInput implements ports.CommandRunner.
func (r *FakeRunner) RunWithInput(_ context.Context, command []string, stdin []byte) (ports.CommandResult, error) {
	r.Inputs = append(r.Inputs, string(stdin))
	return r.run(command, stdin)
}

func (r *FakeRunner) run(command []string, stdin []byte) (ports.CommandResult, error) {
	line := strings.Join(command, " ")
	r.Calls = append(r.Calls, line)
	if r.Missing[command[0]] {
		return ports.CommandResult{}, ports.ErrCommandNotFound
	}
	if r.Respond != nil {
		if result, ok := r.Respond(line, stdin); ok {
			return result, nil
		}
	}
	if output, failing := r.Failing[line]; failing {
		return ports.CommandResult{ExitCode: 1, Output: []byte(output)}, nil
	}
	if output, printing := r.Printing[line]; printing {
		return ports.CommandResult{Output: []byte(output)}, nil
	}
	return ports.CommandResult{Output: []byte(r.Outputs[line])}, nil
}

// FakeOperationLog is an in-memory ports.OperationLog.
type FakeOperationLog struct {
	Operations []oplog.Operation // newest first
}

var _ ports.OperationLog = (*FakeOperationLog)(nil)

// Append implements ports.OperationLog with ids "op1", "op2"…
func (l *FakeOperationLog) Append(_ context.Context, op oplog.Operation) (oplog.Operation, error) {
	if op.ID == "" {
		op.ID = "op" + strconv.Itoa(len(l.Operations)+1)
	}
	l.Operations = append([]oplog.Operation{op}, l.Operations...)
	return op, nil
}

// Recent implements ports.OperationLog.
func (l *FakeOperationLog) Recent(_ context.Context, limit int) ([]oplog.Operation, error) {
	return l.Operations[:min(limit, len(l.Operations))], nil
}
