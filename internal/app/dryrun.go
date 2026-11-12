// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/review"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/core/split"
	"github.com/GregoireF/ship-it/internal/ports"
)

// PlannedStep is one change a dry run would have made: a message key and
// its parameters ("app.dryrun.push", branch and remote), rendered by i18n as
// a "Would …" line.
type PlannedStep struct {
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
	// Remote: the change would reach the remote or GitHub, not only this
	// machine.
	Remote bool `json:"remote,omitempty"`
}

// Rehearsal is what a dry run adds to the result of a use case: the result
// itself is what the real run would have returned, and Planned lists, in
// order, every change it would have made.
type Rehearsal struct {
	DryRun  bool          `json:"dry_run,omitempty"`
	Planned []PlannedStep `json:"planned,omitempty"`
}

// rehearsedCommit stands for a commit a dry run did not make. "HEAD" keeps
// the reads that follow meaningful: the would-be commit's parent is HEAD.
const rehearsedCommit = "HEAD"

// rehearsal is the state of one dry run, shared by every use case that runs
// on the same workspace (ship: tidy, delivery, then publication). It records
// the planned steps and what they would leave behind, so that a later read
// sees the repository as the real run would have left it: the new branch,
// the new commit's message, the operations logged.
type rehearsal struct {
	mu      sync.Mutex
	steps   []PlannedStep
	head    *ports.BranchState // nil: HEAD did not move
	created map[string]bool    // local branches created
	deleted map[string]bool    // local branches deleted
	// clean: a planned commit of everything or a stash leaves no change.
	clean bool
	// committedPaths are the paths of planned partial commits.
	committedPaths []string
	messages       []string         // planned commits, oldest first
	stats          []ports.FileStat // their files
	operations     []oplog.Operation
	stashed        bool
}

func (r *rehearsal) plan(remote bool, key string, params ...string) {
	step := PlannedStep{Key: key, Remote: remote}
	if len(params) > 0 {
		step.Params = make(map[string]string, len(params)/2)
		for i := 0; i+1 < len(params); i += 2 {
			step.Params[params[i]] = params[i+1]
		}
	}
	r.mu.Lock()
	r.steps = append(r.steps, step)
	r.mu.Unlock()
}

// DryRun returns a workspace on which the use cases run every read and every
// check but change nothing: no git write, no GitHub write, no checkpoint, no
// operation logged, no file written. Each write is recorded as a planned
// step instead, which the results carry (Rehearsal). The project's gates are
// listed, not run: they may take minutes and write build outputs.
//
// It is impossible to write by construction: the ports are replaced by
// wrappers that forward the reads only, method by method (no embedding), so
// a write method added to a port does not compile until it is rehearsed.
// The technical journal is still written, its entries marked dry_run.
func DryRun(ws Workspace) Workspace {
	if ws.rehearsal != nil {
		return ws
	}
	r := &rehearsal{created: map[string]bool{}, deleted: map[string]bool{}}
	ws.rehearsal = r
	if ws.Git != nil {
		real := ws
		ws.Git = &dryGit{git: ws.Git, r: r, remote: func(ctx context.Context) string { return gitRemote(ctx, real) }}
	}
	if ws.Forge != nil {
		ws.Forge = &dryForge{forge: ws.Forge, r: r}
	}
	if ws.Stacks != nil {
		ws.Stacks = &dryStacks{stacks: ws.Stacks, r: r}
	}
	if ws.Checkpoints != nil {
		ws.Checkpoints = &dryCheckpoints{store: ws.Checkpoints, r: r}
	}
	if ws.Operations != nil {
		ws.Operations = &dryOperations{log: ws.Operations, r: r}
	}
	if ws.Config != nil {
		ws.Config = &dryConfig{files: ws.Config, r: r}
	}
	if ws.RunLogs != nil {
		ws.RunLogs = dryRunLogs{logs: ws.RunLogs}
	}
	if ws.Tracker != nil {
		ws.Tracker = &dryTracker{tracker: ws.Tracker, r: r}
	}
	if ws.RoadmapMemory != nil {
		ws.RoadmapMemory = dryRoadmapMemory{memory: ws.RoadmapMemory}
	}
	if ws.Projects != nil {
		ws.Projects = &dryProjects{projects: ws.Projects, r: r}
	}
	return ws
}

// IsDryRun reports whether ws comes from DryRun.
func (ws Workspace) IsDryRun() bool { return ws.rehearsal != nil }

// planMark is where the steps of a use case start.
func (ws Workspace) planMark() int {
	if ws.rehearsal == nil {
		return 0
	}
	ws.rehearsal.mu.Lock()
	defer ws.rehearsal.mu.Unlock()
	return len(ws.rehearsal.steps)
}

// rehearsed returns the steps planned since mark; zero outside a dry run.
func (ws Workspace) rehearsed(mark int) Rehearsal {
	if ws.rehearsal == nil {
		return Rehearsal{}
	}
	ws.rehearsal.mu.Lock()
	defer ws.rehearsal.mu.Unlock()
	return Rehearsal{DryRun: true, Planned: slices.Clone(ws.rehearsal.steps[mark:])}
}

// Rehearsed runs fn, a use case on ws that carries no Rehearsal of its own
// (a checkpoint), and returns the steps it planned; zero outside a dry run.
func Rehearsed(ws Workspace, fn func() error) (Rehearsal, error) {
	mark := ws.planMark()
	err := fn()
	return ws.rehearsed(mark), err
}

// planStep records a step that no port write stands for (a gate, a consent).
func (ws Workspace) planStep(key string, params ...string) {
	if ws.rehearsal != nil {
		ws.rehearsal.plan(false, key, params...)
	}
}

func short(commit string) string {
	if len(commit) > shortCommit {
		return commit[:shortCommit]
	}
	return commit
}

func scopeOf(global bool) string {
	if global {
		return "global"
	}
	return "local"
}

func firstLine(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	return line
}

// dryGit forwards the reads of ports.Git and plans its writes.
type dryGit struct {
	git ports.Git
	r   *rehearsal
	// remote is ship.git.remote, read through the real ports (reads only).
	remote func(context.Context) string
}

var _ ports.Git = (*dryGit)(nil)

// Reads, forwarded; a few see the planned changes.

func (d *dryGit) Version(ctx context.Context) (string, error)  { return d.git.Version(ctx) }
func (d *dryGit) TopLevel(ctx context.Context) (string, error) { return d.git.TopLevel(ctx) }
func (d *dryGit) GitDir(ctx context.Context) (string, error)   { return d.git.GitDir(ctx) }

func (d *dryGit) Status(ctx context.Context) (ports.WorkingTreeStatus, error) {
	status, err := d.git.Status(ctx)
	if err != nil {
		return status, err
	}
	d.r.mu.Lock()
	defer d.r.mu.Unlock()
	if h := d.r.head; h != nil {
		status.Branch.Name, status.Branch.Detached, status.Branch.Commit = h.Name, h.Detached, h.Commit
		if h.Commit == rehearsedCommit {
			// A new commit has no upstream position yet.
			status.Branch.Upstream, status.Branch.Ahead, status.Branch.Behind = "", 0, 0
		}
	}
	switch {
	case d.r.clean:
		status.Files = nil
	case len(d.r.committedPaths) > 0:
		status.Files = slices.DeleteFunc(slices.Clone(status.Files), func(f ports.FileChange) bool {
			return split.Matches(f.Path, d.r.committedPaths)
		})
	}
	return status, nil
}

func (d *dryGit) DiffFromHead(ctx context.Context) (string, error) { return d.git.DiffFromHead(ctx) }
func (d *dryGit) ReadWorkingFile(ctx context.Context, path string, limit int64) ([]byte, error) {
	return d.git.ReadWorkingFile(ctx, path, limit)
}
func (d *dryGit) ConfigValue(ctx context.Context, key string) (string, bool, error) {
	return d.git.ConfigValue(ctx, key)
}
func (d *dryGit) ConfigMatching(ctx context.Context, keyPattern string) (map[string]string, error) {
	return d.git.ConfigMatching(ctx, keyPattern)
}
func (d *dryGit) Remotes(ctx context.Context) ([]string, error) { return d.git.Remotes(ctx) }
func (d *dryGit) OperationInProgress(ctx context.Context) (string, error) {
	return d.git.OperationInProgress(ctx)
}
func (d *dryGit) LocalBranchExists(ctx context.Context, name string) (bool, error) {
	d.r.mu.Lock()
	created, deleted := d.r.created[name], d.r.deleted[name]
	d.r.mu.Unlock()
	if created || deleted {
		return created, nil
	}
	return d.git.LocalBranchExists(ctx, name)
}
func (d *dryGit) RemoteURL(ctx context.Context, remote string) (string, error) {
	return d.git.RemoteURL(ctx, remote)
}
func (d *dryGit) RemoteBranchExists(ctx context.Context, remote, branch string) (bool, error) {
	return d.git.RemoteBranchExists(ctx, remote, branch)
}
func (d *dryGit) RecentSubjects(ctx context.Context, limit int) ([]string, error) {
	return d.git.RecentSubjects(ctx, limit)
}

// CommitMessagesSince adds the planned commits to those of HEAD.
func (d *dryGit) CommitMessagesSince(ctx context.Context, base string) ([]string, error) {
	messages, err := d.git.CommitMessagesSince(ctx, base)
	d.r.mu.Lock()
	defer d.r.mu.Unlock()
	if len(d.r.messages) > 0 {
		if base == "" {
			messages = nil // HEAD alone: the last planned commit
			return append(messages, d.r.messages[len(d.r.messages)-1]), err
		}
		messages = append(slices.Clone(messages), d.r.messages...)
	}
	return messages, err
}

// CommitsBetween adds the planned commits when the range ends at HEAD, with
// the identity git would record, so that the rulesets check them too.
func (d *dryGit) CommitsBetween(ctx context.Context, from, to string) ([]ports.CommitRecord, error) {
	records, err := d.git.CommitsBetween(ctx, from, to)
	if err != nil || to != "HEAD" {
		return records, err
	}
	d.r.mu.Lock()
	messages := slices.Clone(d.r.messages)
	d.r.mu.Unlock()
	if len(messages) == 0 {
		return records, nil
	}
	name, _, _ := d.git.ConfigValue(ctx, "user.name")
	email, _, _ := d.git.ConfigValue(ctx, "user.email")
	records = slices.Clone(records)
	for _, m := range messages {
		records = append(records, ports.CommitRecord{ID: rehearsedCommit, AuthorName: name, AuthorEmail: email,
			CommitterName: name, CommitterEmail: email, Message: m})
	}
	return records, nil
}

// SignedCommits says a planned commit would be signed when git signs every
// commit (commit.gpgSign).
func (d *dryGit) SignedCommits(ctx context.Context, ids []string) (map[string]bool, error) {
	var real []string
	planned := false
	for _, id := range ids {
		if id == rehearsedCommit {
			planned = true
		} else {
			real = append(real, id)
		}
	}
	signed, err := d.git.SignedCommits(ctx, real)
	if err != nil || !planned {
		return signed, err
	}
	if v, _, _ := d.git.ConfigValue(ctx, "commit.gpgsign"); strings.EqualFold(v, "true") {
		if signed == nil {
			signed = map[string]bool{}
		}
		signed[rehearsedCommit] = true
	}
	return signed, nil
}
func (d *dryGit) FilesAt(ctx context.Context, rev, dir string) ([]string, error) {
	return d.git.FilesAt(ctx, rev, dir)
}
func (d *dryGit) ReadFileAt(ctx context.Context, rev, path string, limit int64) ([]byte, error) {
	return d.git.ReadFileAt(ctx, rev, path, limit)
}

// CommitPatches leaves the planned commits out: the delivery scanned them.
func (d *dryGit) CommitPatches(ctx context.Context, exclude []string) (string, error) {
	return d.git.CommitPatches(ctx, exclude)
}

// DiffSince reads the branch as it is: the planned commits are not in it,
// so a rehearsed pull request body summarises what is already committed.
func (d *dryGit) DiffSince(ctx context.Context, base string) (string, error) {
	return d.git.DiffSince(ctx, base)
}

// DiffStatSince adds the files of the planned commits.
func (d *dryGit) DiffStatSince(ctx context.Context, base string) ([]ports.FileStat, error) {
	stats, err := d.git.DiffStatSince(ctx, base)
	d.r.mu.Lock()
	defer d.r.mu.Unlock()
	if len(d.r.stats) > 0 {
		if base == "" {
			stats = nil
		}
		stats = append(slices.Clone(stats), d.r.stats...)
	}
	return stats, err
}

// DiffStatBetween reads: a rehearsed layer has no commit, and no size yet.
func (d *dryGit) DiffStatBetween(ctx context.Context, base, head string) ([]ports.FileStat, error) {
	return d.git.DiffStatBetween(ctx, base, head)
}
func (d *dryGit) GeneratedPaths(ctx context.Context, paths []string) (map[string]bool, error) {
	return d.git.GeneratedPaths(ctx, paths)
}
func (d *dryGit) CompareWith(ctx context.Context, ref string) (int, int, error) {
	return d.git.CompareWith(ctx, ref)
}
func (d *dryGit) RemoteDefaultBranch(ctx context.Context, remote string) (string, error) {
	return d.git.RemoteDefaultBranch(ctx, remote)
}
func (d *dryGit) CommitOf(ctx context.Context, ref string) (string, error) {
	if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		d.r.mu.Lock()
		deleted := d.r.deleted[name]
		d.r.mu.Unlock()
		if deleted {
			return "", nil
		}
	}
	return d.git.CommitOf(ctx, ref)
}
func (d *dryGit) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	return d.git.IsAncestor(ctx, ancestor, descendant)
}
func (d *dryGit) LocalBranches(ctx context.Context) (map[string]string, error) {
	branches, err := d.git.LocalBranches(ctx)
	if err != nil {
		return branches, err
	}
	d.r.mu.Lock()
	defer d.r.mu.Unlock()
	out := make(map[string]string, len(branches))
	for name, commit := range branches {
		if !d.r.deleted[name] {
			out[name] = commit
		}
	}
	return out, nil
}

// StashAppliesCleanlyOn cannot be answered for a stash the dry run did not
// make: it says yes, and the plan says the check was not made.
func (d *dryGit) StashAppliesCleanlyOn(ctx context.Context, ref string) (bool, error) {
	d.r.mu.Lock()
	stashed := d.r.stashed
	d.r.mu.Unlock()
	if !stashed {
		return d.git.StashAppliesCleanlyOn(ctx, ref)
	}
	d.r.plan(false, "app.dryrun.conflicts_unchecked", "ref", ref)
	return true, nil
}
func (d *dryGit) ScopedConfigValue(ctx context.Context, key string, global bool) (string, bool, error) {
	return d.git.ScopedConfigValue(ctx, key, global)
}
func (d *dryGit) TrackedFiles(ctx context.Context) ([]string, error) { return d.git.TrackedFiles(ctx) }

// Writes, planned.

func (d *dryGit) moveHead(name, commit string, detached bool) {
	d.r.mu.Lock()
	defer d.r.mu.Unlock()
	if d.r.head == nil {
		d.r.head = &ports.BranchState{}
	}
	if name != "" || detached {
		d.r.head.Name, d.r.head.Detached = name, detached
	}
	if commit != "" {
		d.r.head.Commit = commit
	}
}

// headNow fills the overlay with HEAD as it is, before a first move.
func (d *dryGit) headNow(ctx context.Context) {
	d.r.mu.Lock()
	known := d.r.head != nil
	d.r.mu.Unlock()
	if known {
		return
	}
	status, err := d.git.Status(ctx)
	if err != nil {
		return
	}
	b := status.Branch
	d.r.mu.Lock()
	d.r.head = &ports.BranchState{Name: b.Name, Detached: b.Detached, Commit: b.Commit}
	d.r.mu.Unlock()
}

func (d *dryGit) SwitchToNewBranch(ctx context.Context, name string) error {
	d.headNow(ctx)
	d.r.plan(false, "app.dryrun.create_branch", "branch", name)
	d.r.mu.Lock()
	d.r.created[name] = true
	d.r.mu.Unlock()
	d.moveHead(name, "", false)
	return nil
}

// StageAll is part of the commit that follows: no step of its own.
func (d *dryGit) StageAll(context.Context) error { return nil }

func (d *dryGit) commit(ctx context.Context, message string, paths []string) (string, error) {
	d.headNow(ctx)
	// The files the commit would hold, for the size of the pull request.
	var diffs []snapshot.FileDiff
	if status, err := d.git.Status(ctx); err == nil {
		diffs, _ = selectedWorkIn(ctx, d.git, status, paths)
	}
	stats := make([]ports.FileStat, len(diffs))
	for i, f := range diffs {
		stats[i] = ports.FileStat{Path: f.Path, Added: len(f.Added), Deleted: len(f.Removed), Binary: f.Binary}
	}
	if len(paths) > 0 {
		d.r.plan(false, "app.dryrun.commit_only", "title", firstLine(message), "paths", strings.Join(paths, ", "))
	} else {
		d.r.plan(false, "app.dryrun.commit", "title", firstLine(message))
	}
	d.r.mu.Lock()
	d.r.messages = append(d.r.messages, message)
	d.r.stats = append(d.r.stats, stats...)
	if len(paths) == 0 {
		d.r.clean = true
	} else {
		d.r.committedPaths = append(d.r.committedPaths, paths...)
	}
	d.r.mu.Unlock()
	d.moveHead("", rehearsedCommit, false)
	return rehearsedCommit, nil
}

func (d *dryGit) Commit(ctx context.Context, message string, _ ports.CommitOptions) (string, error) {
	return d.commit(ctx, message, nil)
}
func (d *dryGit) CommitPaths(ctx context.Context, message string, paths []string, _ ports.CommitOptions) (string, error) {
	return d.commit(ctx, message, paths)
}
func (d *dryGit) RecentMessages(ctx context.Context, limit int) ([]string, error) {
	return d.git.RecentMessages(ctx, limit)
}
func (d *dryGit) DeleteRemoteBranch(_ context.Context, remote, branch, _ string) error {
	d.r.plan(true, "app.dryrun.delete_remote_branch", "remote", remote, "branch", branch)
	return nil
}
func (d *dryGit) Push(_ context.Context, remote, branch, lease string) error {
	if lease != "" {
		d.r.plan(true, "app.dryrun.push_lease", "remote", remote, "branch", branch, "lease", short(lease))
		return nil
	}
	d.r.plan(true, "app.dryrun.push", "remote", remote, "branch", branch)
	return nil
}

func (d *dryGit) PushCommit(_ context.Context, remote, branch, commit, _ string) error {
	d.r.plan(true, "app.dryrun.push_back", "remote", remote, "branch", branch, "commit", short(commit))
	return nil
}

// Rebase is planned without knowing whether it would conflict: replaying
// the commits is the only way to know.
func (d *dryGit) Rebase(_ context.Context, onto string) (bool, error) {
	d.r.plan(false, "app.dryrun.rebase", "onto", onto)
	return false, nil
}
func (d *dryGit) RebaseWith(ctx context.Context, onto string, _ ports.CommitOptions) (bool, error) {
	return d.Rebase(ctx, onto)
}

// Merge is planned like Rebase: only merging tells whether it conflicts.
func (d *dryGit) Merge(_ context.Context, ref string, _ ports.CommitOptions) (bool, error) {
	d.r.plan(false, "app.dryrun.merge", "ref", ref)
	return false, nil
}

// RebaseOnto is planned like Rebase; HEAD would end on branch.
func (d *dryGit) RebaseOnto(ctx context.Context, branch, onto, upstream string, _ ports.CommitOptions) (bool, error) {
	d.headNow(ctx)
	d.r.plan(false, "app.dryrun.rebase_onto", "branch", branch, "onto", onto)
	commit, _ := d.git.CommitOf(ctx, "refs/heads/"+branch)
	d.moveHead(branch, commit, false)
	return false, nil
}
func (d *dryGit) MergeBase(ctx context.Context, a, b string) (string, error) {
	return d.git.MergeBase(ctx, a, b)
}

// Fetch updates the remote-tracking branches, a write: a dry run works with
// the ones of the last fetch.
func (d *dryGit) Fetch(_ context.Context, remote string) error {
	d.r.plan(false, "app.dryrun.fetch", "remote", remote)
	return nil
}
func (d *dryGit) FetchBranch(_ context.Context, url, branch, _ string) error {
	d.r.plan(false, "app.dryrun.fetch_url", "url", url, "branch", branch)
	return nil
}
func (d *dryGit) SwitchBranch(ctx context.Context, branch string) error {
	d.headNow(ctx)
	d.r.plan(false, "app.dryrun.switch", "branch", branch)
	commit, _ := d.git.CommitOf(ctx, "refs/heads/"+branch)
	if commit == "" {
		commit, _ = d.git.CommitOf(ctx, "refs/remotes/"+d.remote(ctx)+"/"+branch)
	}
	d.moveHead(branch, commit, false)
	return nil
}
func (d *dryGit) FastForward(_ context.Context, ref string) error {
	d.r.plan(false, "app.dryrun.fast_forward", "ref", ref)
	return nil
}
func (d *dryGit) Stash(context.Context, string) error {
	d.r.plan(false, "app.dryrun.stash")
	d.r.mu.Lock()
	d.r.stashed, d.r.clean = true, true
	d.r.mu.Unlock()
	return nil
}
func (d *dryGit) StashPop(context.Context) error {
	d.r.plan(false, "app.dryrun.stash_pop")
	d.r.mu.Lock()
	d.r.stashed, d.r.clean = false, false
	d.r.mu.Unlock()
	return nil
}
func (d *dryGit) RecreateBranch(_ context.Context, branch, commit string) error {
	d.r.plan(false, "app.dryrun.recreate_branch", "branch", branch, "commit", short(commit))
	d.r.mu.Lock()
	d.r.created[branch], d.r.deleted[branch] = true, false
	d.r.mu.Unlock()
	return nil
}
func (d *dryGit) SetConfig(_ context.Context, key, value string, global bool) error {
	d.r.plan(false, "app.dryrun.set_git_config", "key", key, "value", value, "scope", scopeOf(global))
	return nil
}
func (d *dryGit) UnsetConfig(_ context.Context, key string, global bool) error {
	d.r.plan(false, "app.dryrun.unset_git_config", "key", key, "scope", scopeOf(global))
	return nil
}
func (d *dryGit) Untrack(_ context.Context, paths []string) error {
	d.r.plan(false, "app.dryrun.untrack", "paths", strings.Join(paths, ", "))
	return nil
}
func (d *dryGit) Track(_ context.Context, paths []string) error {
	d.r.plan(false, "app.dryrun.track", "paths", strings.Join(paths, ", "))
	return nil
}
func (d *dryGit) WriteWorkingFile(_ context.Context, path string, _ []byte) error {
	d.r.plan(false, "app.dryrun.write_file", "file", path)
	return nil
}
func (d *dryGit) RemoveWorkingFile(_ context.Context, path string) error {
	d.r.plan(false, "app.dryrun.remove_file", "file", path)
	return nil
}
func (d *dryGit) AttachHead(ctx context.Context, branch string) error {
	d.headNow(ctx)
	d.r.plan(false, "app.dryrun.attach_head", "branch", branch)
	d.moveHead(branch, "", false)
	return nil
}
func (d *dryGit) DetachHead(ctx context.Context, commit string) error {
	d.headNow(ctx)
	d.r.plan(false, "app.dryrun.detach_head", "commit", short(commit))
	d.moveHead("", commit, true)
	return nil
}
func (d *dryGit) MoveBranch(_ context.Context, branch, commit, _ string) error {
	d.r.plan(false, "app.dryrun.move_branch", "branch", branch, "commit", short(commit))
	return nil
}
func (d *dryGit) DeleteBranch(_ context.Context, branch, _ string) error {
	d.r.plan(false, "app.dryrun.delete_branch", "branch", branch)
	d.r.mu.Lock()
	d.r.deleted[branch], d.r.created[branch] = true, false
	d.r.mu.Unlock()
	return nil
}
func (d *dryGit) RenameBranch(ctx context.Context, from, to, _ string) error {
	d.headNow(ctx)
	d.r.plan(false, "app.dryrun.rename_branch", "from", from, "to", to)
	d.r.mu.Lock()
	d.r.created[to], d.r.deleted[from] = true, true
	onFrom := d.r.head != nil && d.r.head.Name == from
	d.r.mu.Unlock()
	if onFrom {
		d.moveHead(to, "", false)
	}
	return nil
}
func (d *dryGit) ResetIndex(_ context.Context, commit string) error {
	d.r.plan(false, "app.dryrun.reset_index", "commit", short(commit))
	return nil
}

// dryForge forwards the reads of ports.Forge and plans its writes.
type dryForge struct {
	forge ports.Forge
	r     *rehearsal
}

var _ ports.Forge = (*dryForge)(nil)

func (d *dryForge) Repository(ctx context.Context, repo forge.Repository) (ports.RepositoryInfo, error) {
	return d.forge.Repository(ctx, repo)
}
func (d *dryForge) FindOpenPullRequest(ctx context.Context, repo forge.Repository, branch string) (ports.PullRequest, bool, error) {
	return d.forge.FindOpenPullRequest(ctx, repo, branch)
}

// CreatePullRequest returns a pull request without a number: the real run
// would get one from GitHub.
func (d *dryForge) CreatePullRequest(_ context.Context, _ forge.Repository, spec ports.PullRequestSpec) (ports.PullRequest, error) {
	key := "app.dryrun.open_pull_request"
	if spec.Draft {
		key = "app.dryrun.open_draft_pull_request"
	}
	d.r.plan(true, key, "title", spec.Title, "head", spec.Head, "base", spec.Base)
	return ports.PullRequest{Draft: spec.Draft, State: "open", HeadBranch: spec.Head}, nil
}
func (d *dryForge) LatestPullRequest(ctx context.Context, repo forge.Repository, branch string) (ports.PullRequest, bool, error) {
	return d.forge.LatestPullRequest(ctx, repo, branch)
}
func (d *dryForge) MergedPullRequests(ctx context.Context, repo forge.Repository, limit int) ([]ports.PullRequest, error) {
	return d.forge.MergedPullRequests(ctx, repo, limit)
}
func (d *dryForge) Labels(ctx context.Context, repo forge.Repository) ([]settings.Label, error) {
	return d.forge.Labels(ctx, repo)
}
func (d *dryForge) AddLabels(_ context.Context, _ forge.Repository, number int, labels []string) error {
	if number == 0 {
		d.r.plan(true, "app.dryrun.add_labels_new", "labels", strings.Join(labels, ", "))
		return nil
	}
	d.r.plan(true, "app.dryrun.add_labels", "labels", strings.Join(labels, ", "), "number", strconv.Itoa(number))
	return nil
}
func (d *dryForge) RequestReviewers(_ context.Context, _ forge.Repository, number int, users, teams []string) error {
	who := strings.Join(append(slices.Clone(users), teams...), ", ")
	if number == 0 {
		d.r.plan(true, "app.dryrun.request_reviewers_new", "reviewers", who)
		return nil
	}
	d.r.plan(true, "app.dryrun.request_reviewers", "reviewers", who, "number", strconv.Itoa(number))
	return nil
}
func (d *dryForge) CurrentUser(ctx context.Context, repo forge.Repository) (ports.User, error) {
	return d.forge.CurrentUser(ctx, repo)
}
func (d *dryForge) ClosePullRequest(_ context.Context, _ forge.Repository, number int, _ string) error {
	d.r.plan(true, "app.dryrun.close_pull_request", "number", strconv.Itoa(number))
	return nil
}
func (d *dryForge) RemoveLabel(_ context.Context, _ forge.Repository, number int, label string) error {
	d.r.plan(true, "app.dryrun.remove_label", "label", label, "number", strconv.Itoa(number))
	return nil
}
func (d *dryForge) CreateLabel(_ context.Context, _ forge.Repository, label settings.Label) error {
	d.r.plan(true, "app.dryrun.create_label", "name", label.Name)
	return nil
}
func (d *dryForge) UpdateLabel(_ context.Context, _ forge.Repository, name string, label settings.Label) error {
	d.r.plan(true, "app.dryrun.update_label", "from", name, "name", label.Name)
	return nil
}
func (d *dryForge) IssueTitle(ctx context.Context, repo forge.Repository, number int) (string, error) {
	return d.forge.IssueTitle(ctx, repo, number)
}
func (d *dryForge) FileContent(ctx context.Context, repo forge.Repository, path, commit string) ([]byte, error) {
	return d.forge.FileContent(ctx, repo, path, commit)
}
func (d *dryForge) BranchRules(ctx context.Context, repo forge.Repository, branch string) ([]rules.Rule, error) {
	return d.forge.BranchRules(ctx, repo, branch)
}
func (d *dryForge) OpenPullRequestPatches(ctx context.Context, repo forge.Repository, base string, limit int) ([]ports.PullRequestPatches, error) {
	return d.forge.OpenPullRequestPatches(ctx, repo, base, limit)
}

func (d *dryForge) PullRequestContent(ctx context.Context, repo forge.Repository, number int) (ports.PullRequestContent, error) {
	return d.forge.PullRequestContent(ctx, repo, number)
}
func (d *dryForge) CheckRuns(ctx context.Context, repo forge.Repository, commit string) ([]review.CheckRun, error) {
	return d.forge.CheckRuns(ctx, repo, commit)
}
func (d *dryForge) CommitStatuses(ctx context.Context, repo forge.Repository, commit string) ([]review.CommitStatus, error) {
	return d.forge.CommitStatuses(ctx, repo, commit)
}
func (d *dryForge) Reviews(ctx context.Context, repo forge.Repository, number int) ([]review.Review, error) {
	return d.forge.Reviews(ctx, repo, number)
}

// dryCheckpoints lists the checkpoints and plans the others.
type dryCheckpoints struct {
	store ports.CheckpointStore
	r     *rehearsal
}

var _ ports.CheckpointStore = (*dryCheckpoints)(nil)

// Create returns a checkpoint without an id: none was taken.
func (d *dryCheckpoints) Create(_ context.Context, label string) (ports.Checkpoint, error) {
	d.r.plan(false, "app.dryrun.checkpoint", "label", label)
	return ports.Checkpoint{Label: label}, nil
}
func (d *dryCheckpoints) List(ctx context.Context, limit int) ([]ports.Checkpoint, error) {
	return d.store.List(ctx, limit)
}
func (d *dryCheckpoints) RestoreFiles(_ context.Context, id string) error {
	d.r.plan(false, "app.dryrun.restore_checkpoint", "id", short(id))
	return nil
}
func (d *dryCheckpoints) Remove(_ context.Context, ids []string) error {
	d.r.plan(false, "app.dryrun.remove_checkpoints", "count", strconv.Itoa(len(ids)))
	return nil
}

// dryOperations reads the log and keeps the planned entries in memory, so
// that a later use case of the same dry run sees them (publish finds the
// branch the delivery would create).
type dryOperations struct {
	log ports.OperationLog
	r   *rehearsal
}

var _ ports.OperationLog = (*dryOperations)(nil)

func (d *dryOperations) Append(_ context.Context, op oplog.Operation) (oplog.Operation, error) {
	d.r.plan(false, "app.dryrun.log", "kind", string(op.Kind))
	// No id: nothing was logged, and nothing can be undone.
	d.r.mu.Lock()
	d.r.operations = append(d.r.operations, op)
	d.r.mu.Unlock()
	return op, nil
}

func (d *dryOperations) Recent(ctx context.Context, limit int) ([]oplog.Operation, error) {
	ops, err := d.log.Recent(ctx, limit)
	if err != nil {
		return ops, err
	}
	d.r.mu.Lock()
	defer d.r.mu.Unlock()
	if len(d.r.operations) == 0 {
		return ops, nil
	}
	planned := make([]oplog.Operation, 0, len(d.r.operations)+len(ops))
	for i := len(d.r.operations) - 1; i >= 0; i-- {
		op := d.r.operations[i]
		op.ID = "planned-" + strconv.Itoa(i+1) // undo plans need an id to point at
		planned = append(planned, op)
	}
	return append(planned, ops...)[:min(limit, len(planned)+len(ops))], nil
}

// dryConfig reads the configuration files and plans the writes.
type dryConfig struct {
	files ports.ConfigFiles
	r     *rehearsal
}

var _ ports.ConfigFiles = (*dryConfig)(nil)

func (d *dryConfig) Layers(ctx context.Context) ([]config.Layer, error) { return d.files.Layers(ctx) }

// CreateRepositoryFile says what the real one would: the path, or that the
// file exists.
func (d *dryConfig) CreateRepositoryFile(ctx context.Context, _ []byte) (string, error) {
	file, err := d.files.ReadConfigFile(ctx, ports.ScopeRepository)
	if err != nil {
		return "", err
	}
	if file.Exists {
		return file.Path, ports.ErrConfigExists
	}
	d.r.plan(false, "app.dryrun.create_config_file", "file", file.Path)
	return file.Path, nil
}
func (d *dryConfig) ReadPreset(ctx context.Context, base config.Layer, path string) (config.Layer, error) {
	return d.files.ReadPreset(ctx, base, path)
}
func (d *dryConfig) CachedPreset(ctx context.Context, key string) ([]byte, bool) {
	return d.files.CachedPreset(ctx, key)
}

// StorePreset is a cache ship-it keeps for itself: skipped, with no step.
func (d *dryConfig) StorePreset(context.Context, string, []byte) error { return nil }
func (d *dryConfig) ReadConfigFile(ctx context.Context, scope ports.ConfigScope) (ports.ConfigFile, error) {
	return d.files.ReadConfigFile(ctx, scope)
}

// ReplaceConfigFile checks the compare-and-swap as the real one would, then
// plans the write.
func (d *dryConfig) ReplaceConfigFile(ctx context.Context, change ports.ConfigFileChange) (string, error) {
	file, err := d.files.ReadConfigFile(ctx, change.Scope)
	if err != nil {
		return "", err
	}
	if file.Exists != change.OldExists || (change.OldExists && string(file.Data) != string(change.Old)) {
		return file.Path, ports.ErrConfigChanged
	}
	if change.NewExists {
		d.r.plan(false, "app.dryrun.write_config_file", "file", file.Path)
	} else {
		d.r.plan(false, "app.dryrun.remove_config_file", "file", file.Path)
	}
	return file.Path, nil
}

// dryRunLogs only counts the journals a prune would remove.
type dryRunLogs struct{ logs ports.RunLogs }

func (d dryRunLogs) Prune(ctx context.Context, keepRuns int, maxBytes int64, _ bool) (int, int64, error) {
	return d.logs.Prune(ctx, keepRuns, maxBytes, true)
}

// dryTracker forwards the reads of ports.Tracker and plans its writes. It
// remembers the issue states it would have changed, so that the milestones it
// reads afterwards count them as the real run would (an undo deletes a
// milestone once its issues are closed).
type dryTracker struct {
	tracker ports.Tracker
	r       *rehearsal
	open    map[int]bool // number → open, for the issues it would close or reopen
}

var _ ports.Tracker = (*dryTracker)(nil)

func (d *dryTracker) Milestones(ctx context.Context, repo forge.Repository) ([]roadmap.Milestone, error) {
	milestones, err := d.tracker.Milestones(ctx, repo)
	if err != nil || len(d.open) == 0 {
		return milestones, err
	}
	numbers := make([]int, 0, len(d.open))
	for number := range d.open {
		numbers = append(numbers, number)
	}
	issues, err := d.tracker.TrackedIssues(ctx, repo, roadmap.Marker, numbers)
	if err != nil {
		return nil, err
	}
	milestones = slices.Clone(milestones)
	for _, is := range issues {
		open, changed := d.open[is.Number]
		if !changed || open == is.Open {
			continue
		}
		for i := range milestones {
			if milestones[i].Number != is.Milestone {
				continue
			}
			delta := 1
			if !open {
				delta = -1
			}
			milestones[i].OpenIssues += delta
			milestones[i].ClosedIssues -= delta
		}
	}
	return milestones, nil
}
func (d *dryTracker) TrackedIssues(ctx context.Context, repo forge.Repository, marker string, numbers []int) ([]roadmap.Issue, error) {
	return d.tracker.TrackedIssues(ctx, repo, marker, numbers)
}
func (d *dryTracker) SubIssues(ctx context.Context, repo forge.Repository, parent int) ([]int, error) {
	return d.tracker.SubIssues(ctx, repo, parent)
}
func (d *dryTracker) Issue(ctx context.Context, repo forge.Repository, number int) (roadmap.Issue, error) {
	return d.tracker.Issue(ctx, repo, number)
}
func (d *dryTracker) AddAssignees(_ context.Context, _ forge.Repository, number int, logins []string) error {
	if number == 0 { // the pull request this dry run would open
		d.r.plan(true, "app.dryrun.assign_new_pull_request", "logins", strings.Join(logins, ", "))
		return nil
	}
	d.r.plan(true, "app.dryrun.assign_issue", "number", strconv.Itoa(number), "logins", strings.Join(logins, ", "))
	return nil
}
func (d *dryTracker) RemoveAssignees(_ context.Context, _ forge.Repository, number int, logins []string) error {
	d.r.plan(true, "app.dryrun.unassign_issue", "number", strconv.Itoa(number), "logins", strings.Join(logins, ", "))
	return nil
}

// CreateMilestone and CreateIssue return no number: the real run would get
// one from GitHub.
func (d *dryTracker) CreateMilestone(_ context.Context, _ forge.Repository, m roadmap.Milestone) (int, error) {
	d.r.plan(true, "app.dryrun.create_milestone", "title", m.Title)
	return 0, nil
}
func (d *dryTracker) UpdateMilestone(_ context.Context, _ forge.Repository, m roadmap.Milestone) error {
	d.r.plan(true, "app.dryrun.update_milestone", "title", m.Title)
	return nil
}
func (d *dryTracker) DeleteMilestone(_ context.Context, _ forge.Repository, number int) error {
	d.r.plan(true, "app.dryrun.delete_milestone", "number", strconv.Itoa(number))
	return nil
}
func (d *dryTracker) CreateIssue(_ context.Context, _ forge.Repository, title, _ string, _ int, labels []string) (roadmap.Issue, error) {
	d.r.plan(true, "app.dryrun.create_issue", "title", title)
	return roadmap.Issue{Title: title, Open: true, Labels: labels}, nil
}
func (d *dryTracker) UpdateIssue(_ context.Context, _ forge.Repository, number int, update ports.IssueUpdate) error {
	if number == 0 && update.Milestone != nil { // the pull request this dry run would open
		d.r.plan(true, "app.dryrun.milestone_new_pull_request", "milestone", strconv.Itoa(*update.Milestone))
		return nil
	}
	key := "app.dryrun.update_issue"
	switch update.State {
	case "closed":
		key = "app.dryrun.close_issue"
	case "open":
		key = "app.dryrun.reopen_issue"
	}
	if update.State != "" {
		if d.open == nil {
			d.open = map[int]bool{}
		}
		d.open[number] = update.State == "open"
	}
	d.r.plan(true, key, "number", strconv.Itoa(number))
	return nil
}
func (d *dryTracker) CommentIssue(_ context.Context, _ forge.Repository, number int, _ string) error {
	d.r.plan(true, "app.dryrun.comment_issue", "number", strconv.Itoa(number))
	return nil
}
func (d *dryTracker) SetParent(_ context.Context, _ forge.Repository, parent int, _ int64) error {
	d.r.plan(true, "app.dryrun.set_parent", "parent", strconv.Itoa(parent))
	return nil
}
func (d *dryTracker) RemoveParent(_ context.Context, _ forge.Repository, parent int, _ int64) error {
	d.r.plan(true, "app.dryrun.remove_parent", "parent", strconv.Itoa(parent))
	return nil
}

// dryRoadmapMemory reads the memory of the last sync and never saves it: it
// is ship-it's own bookkeeping, like the preset cache, not a step to show.
type dryRoadmapMemory struct {
	memory ports.RoadmapMemory
}

func (d dryRoadmapMemory) Load(ctx context.Context) (roadmap.Memory, error) {
	return d.memory.Load(ctx)
}
func (d dryRoadmapMemory) Save(context.Context, roadmap.Memory) error { return nil }

// dryProjects forwards the reads of ports.Projects and plans its writes.
type dryProjects struct {
	projects ports.Projects
	r        *rehearsal
}

var _ ports.Projects = (*dryProjects)(nil)

func (d *dryProjects) ProjectFields(ctx context.Context, repo forge.Repository, project forge.Project) (forge.Project, []roadmap.ProjectField, error) {
	return d.projects.ProjectFields(ctx, repo, project)
}
func (d *dryProjects) ProjectItems(ctx context.Context, repo forge.Repository, project forge.Project, fields []int64) ([]roadmap.ProjectItem, error) {
	return d.projects.ProjectItems(ctx, repo, project, fields)
}

// AddProjectItem returns no item id: the real run would get one from GitHub.
func (d *dryProjects) AddProjectItem(_ context.Context, _ forge.Repository, _ forge.Project, _ int64) (int64, error) {
	d.r.plan(true, "app.dryrun.add_project_item")
	return 0, nil
}
func (d *dryProjects) SetProjectField(_ context.Context, _ forge.Repository, _ forge.Project, _, _ int64, _ string) error {
	d.r.plan(true, "app.dryrun.set_project_field")
	return nil
}
func (d *dryProjects) RemoveProjectItem(_ context.Context, _ forge.Repository, _ forge.Project, _ int64) error {
	d.r.plan(true, "app.dryrun.remove_project_item")
	return nil
}
