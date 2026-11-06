// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package testkit

import (
	"context"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/ports"
)

// The guards wrap a port so that any write fails the test: a dry run that
// reaches one of them changed something. Reads go through untouched.

func wrote(t testing.TB, method string) {
	t.Helper()
	t.Errorf("dry run wrote: %s", method)
}

// GuardGit fails t on every write of g.
func GuardGit(t testing.TB, g ports.Git) ports.Git { return guardGit{Git: g, t: t} }

type guardGit struct {
	ports.Git
	t testing.TB
}

func (g guardGit) SwitchToNewBranch(context.Context, string) error {
	wrote(g.t, "SwitchToNewBranch")
	return nil
}
func (g guardGit) StageAll(context.Context) error { wrote(g.t, "StageAll"); return nil }
func (g guardGit) Commit(context.Context, string, ports.CommitOptions) (string, error) {
	wrote(g.t, "Commit")
	return "", nil
}
func (g guardGit) RebaseWith(context.Context, string, ports.CommitOptions) (bool, error) {
	wrote(g.t, "RebaseWith")
	return false, nil
}
func (g guardGit) Merge(context.Context, string, ports.CommitOptions) (bool, error) {
	wrote(g.t, "Merge")
	return false, nil
}
func (g guardGit) CommitPaths(context.Context, string, []string, ports.CommitOptions) (string, error) {
	wrote(g.t, "CommitPaths")
	return "", nil
}
func (g guardGit) DeleteRemoteBranch(context.Context, string, string, string) error {
	wrote(g.t, "DeleteRemoteBranch")
	return nil
}
func (g guardGit) Push(context.Context, string, string, string) error { wrote(g.t, "Push"); return nil }
func (g guardGit) PushCommit(context.Context, string, string, string, string) error {
	wrote(g.t, "PushCommit")
	return nil
}
func (g guardGit) Rebase(context.Context, string) (bool, error) {
	wrote(g.t, "Rebase")
	return false, nil
}
func (g guardGit) RebaseOnto(context.Context, string, string, string, ports.CommitOptions) (bool, error) {
	wrote(g.t, "RebaseOnto")
	return false, nil
}
func (g guardGit) Fetch(context.Context, string) error { wrote(g.t, "Fetch"); return nil }
func (g guardGit) FetchBranch(context.Context, string, string, string) error {
	wrote(g.t, "FetchBranch")
	return nil
}
func (g guardGit) SwitchBranch(context.Context, string) error { wrote(g.t, "SwitchBranch"); return nil }
func (g guardGit) FastForward(context.Context, string) error  { wrote(g.t, "FastForward"); return nil }
func (g guardGit) Stash(context.Context, string) error        { wrote(g.t, "Stash"); return nil }
func (g guardGit) StashPop(context.Context) error             { wrote(g.t, "StashPop"); return nil }
func (g guardGit) RecreateBranch(context.Context, string, string) error {
	wrote(g.t, "RecreateBranch")
	return nil
}
func (g guardGit) SetConfig(context.Context, string, string, bool) error {
	wrote(g.t, "SetConfig")
	return nil
}
func (g guardGit) UnsetConfig(context.Context, string, bool) error {
	wrote(g.t, "UnsetConfig")
	return nil
}
func (g guardGit) Untrack(context.Context, []string) error { wrote(g.t, "Untrack"); return nil }
func (g guardGit) Track(context.Context, []string) error   { wrote(g.t, "Track"); return nil }
func (g guardGit) WriteWorkingFile(context.Context, string, []byte) error {
	wrote(g.t, "WriteWorkingFile")
	return nil
}
func (g guardGit) RemoveWorkingFile(context.Context, string) error {
	wrote(g.t, "RemoveWorkingFile")
	return nil
}
func (g guardGit) AttachHead(context.Context, string) error { wrote(g.t, "AttachHead"); return nil }
func (g guardGit) DetachHead(context.Context, string) error { wrote(g.t, "DetachHead"); return nil }
func (g guardGit) MoveBranch(context.Context, string, string, string) error {
	wrote(g.t, "MoveBranch")
	return nil
}
func (g guardGit) DeleteBranch(context.Context, string, string) error {
	wrote(g.t, "DeleteBranch")
	return nil
}
func (g guardGit) RenameBranch(context.Context, string, string, string) error {
	wrote(g.t, "RenameBranch")
	return nil
}
func (g guardGit) ResetIndex(context.Context, string) error { wrote(g.t, "ResetIndex"); return nil }

// GuardForge fails t on every write of f.
func GuardForge(t testing.TB, f ports.Forge) ports.Forge { return guardForge{Forge: f, t: t} }

type guardForge struct {
	ports.Forge
	t testing.TB
}

func (g guardForge) CreatePullRequest(context.Context, forge.Repository, ports.PullRequestSpec) (ports.PullRequest, error) {
	wrote(g.t, "CreatePullRequest")
	return ports.PullRequest{}, nil
}
func (g guardForge) AddLabels(context.Context, forge.Repository, int, []string) error {
	wrote(g.t, "AddLabels")
	return nil
}
func (g guardForge) ClosePullRequest(context.Context, forge.Repository, int, string) error {
	wrote(g.t, "ClosePullRequest")
	return nil
}
func (g guardForge) RemoveLabel(context.Context, forge.Repository, int, string) error {
	wrote(g.t, "RemoveLabel")
	return nil
}
func (g guardForge) CreateLabel(context.Context, forge.Repository, settings.Label) error {
	wrote(g.t, "CreateLabel")
	return nil
}
func (g guardForge) UpdateLabel(context.Context, forge.Repository, string, settings.Label) error {
	wrote(g.t, "UpdateLabel")
	return nil
}

// GuardCheckpoints fails t on every write of s.
func GuardCheckpoints(t testing.TB, s ports.CheckpointStore) ports.CheckpointStore {
	return guardCheckpoints{CheckpointStore: s, t: t}
}

type guardCheckpoints struct {
	ports.CheckpointStore
	t testing.TB
}

func (g guardCheckpoints) Create(context.Context, string) (ports.Checkpoint, error) {
	wrote(g.t, "Checkpoints.Create")
	return ports.Checkpoint{}, nil
}
func (g guardCheckpoints) RestoreFiles(context.Context, string) error {
	wrote(g.t, "Checkpoints.RestoreFiles")
	return nil
}
func (g guardCheckpoints) Remove(context.Context, []string) error {
	wrote(g.t, "Checkpoints.Remove")
	return nil
}

// GuardOperations fails t on every write of l.
func GuardOperations(t testing.TB, l ports.OperationLog) ports.OperationLog {
	return guardOperations{OperationLog: l, t: t}
}

type guardOperations struct {
	ports.OperationLog
	t testing.TB
}

func (g guardOperations) Append(_ context.Context, op oplog.Operation) (oplog.Operation, error) {
	wrote(g.t, "Operations.Append")
	return op, nil
}

// GuardConfig fails t on every write of c.
func GuardConfig(t testing.TB, c ports.ConfigFiles) ports.ConfigFiles {
	return guardConfig{ConfigFiles: c, t: t}
}

type guardConfig struct {
	ports.ConfigFiles
	t testing.TB
}

func (g guardConfig) CreateRepositoryFile(context.Context, []byte) (string, error) {
	wrote(g.t, "CreateRepositoryFile")
	return "", nil
}
func (g guardConfig) StorePreset(context.Context, string, []byte) error {
	wrote(g.t, "StorePreset")
	return nil
}
func (g guardConfig) ReplaceConfigFile(context.Context, ports.ConfigFileChange) (string, error) {
	wrote(g.t, "ReplaceConfigFile")
	return "", nil
}

// GuardTracker fails t on every write of tr.
func GuardTracker(t testing.TB, tr ports.Tracker) ports.Tracker {
	return guardTracker{Tracker: tr, t: t}
}

type guardTracker struct {
	ports.Tracker
	t testing.TB
}

func (g guardTracker) CreateMilestone(context.Context, forge.Repository, roadmap.Milestone) (int, error) {
	wrote(g.t, "CreateMilestone")
	return 0, nil
}
func (g guardTracker) UpdateMilestone(context.Context, forge.Repository, roadmap.Milestone) error {
	wrote(g.t, "UpdateMilestone")
	return nil
}
func (g guardTracker) DeleteMilestone(context.Context, forge.Repository, int) error {
	wrote(g.t, "DeleteMilestone")
	return nil
}
func (g guardTracker) CreateIssue(context.Context, forge.Repository, string, string, int, []string) (roadmap.Issue, error) {
	wrote(g.t, "CreateIssue")
	return roadmap.Issue{}, nil
}
func (g guardTracker) UpdateIssue(context.Context, forge.Repository, int, ports.IssueUpdate) error {
	wrote(g.t, "UpdateIssue")
	return nil
}
func (g guardTracker) CommentIssue(context.Context, forge.Repository, int, string) error {
	wrote(g.t, "CommentIssue")
	return nil
}
func (g guardTracker) SetParent(context.Context, forge.Repository, int, int64) error {
	wrote(g.t, "SetParent")
	return nil
}
func (g guardTracker) RemoveParent(context.Context, forge.Repository, int, int64) error {
	wrote(g.t, "RemoveParent")
	return nil
}

// GuardRoadmapMemory fails t when m is saved.
func GuardRoadmapMemory(t testing.TB, m ports.RoadmapMemory) ports.RoadmapMemory {
	return guardRoadmapMemory{RoadmapMemory: m, t: t}
}

type guardRoadmapMemory struct {
	ports.RoadmapMemory
	t testing.TB
}

func (g guardRoadmapMemory) Save(context.Context, roadmap.Memory) error {
	wrote(g.t, "RoadmapMemory.Save")
	return nil
}
func (g guardTracker) AddAssignees(context.Context, forge.Repository, int, []string) error {
	wrote(g.t, "AddAssignees")
	return nil
}
func (g guardTracker) RemoveAssignees(context.Context, forge.Repository, int, []string) error {
	wrote(g.t, "RemoveAssignees")
	return nil
}

// GuardProjects fails t on every write of p.
func GuardProjects(t testing.TB, p ports.Projects) ports.Projects {
	return guardProjects{Projects: p, t: t}
}

type guardProjects struct {
	ports.Projects
	t testing.TB
}

func (g guardProjects) AddProjectItem(context.Context, forge.Repository, forge.Project, int64) (int64, error) {
	wrote(g.t, "AddProjectItem")
	return 0, nil
}
func (g guardProjects) SetProjectField(context.Context, forge.Repository, forge.Project, int64, int64, string) error {
	wrote(g.t, "SetProjectField")
	return nil
}
func (g guardProjects) RemoveProjectItem(context.Context, forge.Repository, forge.Project, int64) error {
	wrote(g.t, "RemoveProjectItem")
	return nil
}

func (g guardForge) RequestReviewers(context.Context, forge.Repository, int, []string, []string) error {
	wrote(g.t, "RequestReviewers")
	return nil
}
