// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/core/rules"
	"github.com/GregoireF/ship-it/internal/ports"
)

// shortCommit is how many characters of a commit id messages show.
const shortCommit = 7

// rulesetRename is the rename a ruleset asks for, decided before the gates
// and applied right before the push.
type rulesetRename struct {
	from, to string
	broken   rules.Pattern
}

// checkRulesets reads the GitHub ruleset rules of the branch and refuses,
// before anything is pushed, what GitHub would reject (rule zero: what the
// project imposes wins). A branch name that breaks a pattern is renamed only
// when ship-it created the branch and never pushed it; commits are never
// rewritten. base "" (nothing on the remote yet) leaves the commits to GitHub.
func checkRulesets(ctx context.Context, ws Workspace, repo forge.Repository, branch ports.BranchState, base string) (*rulesetRename, error) {
	return checkRulesetsAt(ctx, ws, repo, branch, base, "HEAD")
}

// checkRulesetsAt is checkRulesets for the commits base..head: a layer of a
// stack is checked on its own branch, whatever is checked out.
func checkRulesetsAt(ctx context.Context, ws Workspace, repo forge.Repository, branch ports.BranchState, base, head string) (*rulesetRename, error) {
	found, err := branchRules(ctx, ws, repo, branch.Name)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	var rename *rulesetRename
	patterns := rules.Of(found, rules.BranchNamePattern)
	if broken, ok := rules.FirstViolated(patterns, branch.Name); ok {
		candidate, compliant := rules.CompliantBranch(branch.Name, patterns)
		if !compliant {
			return nil, branchRefused(branch.Name, broken, "")
		}
		if reason := renameBlocked(ctx, ws, branch.Name, candidate); reason != "" {
			ws.logger().LogAttrs(ctx, slog.LevelDebug, "rules.rename_blocked", slog.String("reason", reason))
			return nil, branchRefused(branch.Name, broken, candidate)
		}
		// The new name may fall under other rulesets: its own rules decide.
		if found, err = branchRules(ctx, ws, repo, candidate); err != nil {
			return nil, err
		}
		if other, ok := rules.FirstViolated(rules.Of(found, rules.BranchNamePattern), candidate); ok {
			return nil, branchRefused(branch.Name, other, "")
		}
		rename = &rulesetRename{from: branch.Name, to: candidate, broken: broken}
	}
	if base == "" {
		return rename, nil
	}
	return rename, checkCommitRules(ctx, ws, found, base, head)
}

// branchRules returns the rules of a branch name. No access to the rules
// (404: GHES without rulesets, a token without the right; 403) means none:
// GitHub still enforces them at push time.
func branchRules(ctx context.Context, ws Workspace, repo forge.Repository, name string) ([]rules.Rule, error) {
	found, err := ws.Forge.BranchRules(ctx, repo, name)
	if errors.Is(err, ports.ErrNotFound) || errors.Is(err, ports.ErrForbidden) {
		ws.logger().LogAttrs(ctx, slog.LevelDebug, "rules.unavailable", slog.String("error", lastLine(err.Error())))
		return nil, nil
	}
	if err != nil {
		return nil, forgeFault(err, repo)
	}
	return found, nil
}

// renameBlocked says why ship-it may not rename the branch, "" when it may:
// only a branch it created, never pushed, to a free name.
func renameBlocked(ctx context.Context, ws Workspace, from, to string) string {
	publishRemote := gitRemote(ctx, ws)
	if pushed, err := ws.Git.RemoteBranchExists(ctx, publishRemote, from); err != nil || pushed {
		return "pushed"
	}
	if !createdByShip(ctx, ws, from) {
		return "not_created_by_ship"
	}
	if exists, err := ws.Git.LocalBranchExists(ctx, to); err != nil || exists {
		return "target_exists"
	}
	if exists, err := ws.Git.RemoteBranchExists(ctx, publishRemote, to); err != nil || exists {
		return "target_exists"
	}
	return ""
}

// createdByShip reports whether a delivery not undone created the branch,
// and no publication pushed it since.
func createdByShip(ctx context.Context, ws Workspace, name string) bool {
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return false
	}
	for _, op := range oplog.Undoable(ops) {
		switch {
		case op.Kind == oplog.Publish && op.After.Branch == name:
			return false
		case op.Kind == oplog.Rename && op.After.Branch == name:
			return true
		case op.Kind == oplog.Deliver && op.After.Branch == name && op.CreatedBranch:
			return true
		case op.Kind == oplog.StackPublish && op.Stack != nil && stackHolds(op.Stack, name):
			return false // pushed
		case op.Kind == oplog.Stack && op.Stack != nil && stackHolds(op.Stack, name):
			return true
		}
	}
	return false
}

// stackHolds reports whether a stack operation names branch as a layer.
func stackHolds(record *oplog.StackRecord, branch string) bool {
	for _, l := range record.Layers {
		if l.Branch == branch {
			return true
		}
	}
	return false
}

func branchRefused(branch string, broken rules.Pattern, candidate string) error {
	if candidate == "" {
		return fault.New(fault.BranchNameRefused, "publish.rules.branch_no_name",
			"branch", branch, "rule", broken.Label(), "requirement", broken.Describe())
	}
	return fault.New(fault.BranchNameRefused, "publish.rules.branch_refused",
		"branch", branch, "rule", broken.Label(), "requirement", broken.Describe(), "command", "git branch -m "+branch+" "+candidate)
}

// checkCommitRules refuses commits of base..head that break a message or
// e-mail pattern, or are unsigned where signatures are required.
func checkCommitRules(ctx context.Context, ws Workspace, found []rules.Rule, base, head string) error {
	if !rules.Has(found, rules.CommitMessagePattern) && !rules.Has(found, rules.CommitAuthorEmailPattern) &&
		!rules.Has(found, rules.CommitterEmailPattern) && !rules.Has(found, rules.RequiredSignatures) {
		return nil
	}
	records, err := ws.Git.CommitsBetween(ctx, base, head)
	if err != nil {
		return err
	}
	signed := map[string]bool{}
	if rules.Has(found, rules.RequiredSignatures) {
		ids := make([]string, len(records))
		for i, r := range records {
			ids[i] = r.ID
		}
		if signed, err = ws.Git.SignedCommits(ctx, ids); err != nil {
			return err
		}
	}
	commits := make([]rules.Commit, len(records))
	for i, r := range records {
		commits[i] = rules.Commit{ID: r.ID, Message: r.Message, AuthorEmail: r.AuthorEmail,
			CommitterEmail: r.CommitterEmail, Unsigned: !signed[r.ID]}
	}
	v, broken := rules.CheckCommits(found, commits)
	if !broken {
		return nil
	}
	id := v.Commit.ID
	if len(id) > shortCommit {
		id = id[:shortCommit]
	}
	subject, _ := pullrequest.ParseMessage(v.Commit.Message)
	switch v.Type {
	case rules.CommitMessagePattern:
		return fault.New(fault.CommitMessageRefused, "publish.rules.commit_message",
			"commit", id, "subject", subject, "rule", v.Pattern.Label(), "requirement", v.Pattern.Describe(), "base", base)
	case rules.CommitAuthorEmailPattern, rules.CommitterEmailPattern:
		key := "publish.rules.author_email"
		if v.Type == rules.CommitterEmailPattern {
			key = "publish.rules.committer_email"
		}
		return fault.New(fault.CommitEmailRefused, key,
			"commit", id, "email", v.Value, "rule", v.Pattern.Label(), "requirement", v.Pattern.Describe(), "base", base)
	}
	return fault.New(fault.UnsignedCommit, "publish.rules.unsigned", "commit", id, "subject", subject, "base", base)
}

// applyRename renames the branch as decided, records it so that ship undo
// gives the old name back, and says so.
func applyRename(ctx context.Context, ws Workspace, branch ports.BranchState, rename *rulesetRename, result *PublishResult) (ports.BranchState, error) {
	renamed := ws.begin(ctx, "publish/rename")
	err := ws.Git.RenameBranch(ctx, rename.from, rename.to, branch.Commit)
	renamed(err)
	if err != nil {
		return branch, err
	}
	before := repositoryViewOf(branch)
	branch.Name = rename.to
	if _, err := ws.Operations.Append(ctx, oplog.Operation{
		Kind: oplog.Rename, Before: before, After: repositoryViewOf(branch), Title: rename.from,
	}); err != nil {
		return branch, err
	}
	result.Branch = rename.to
	result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.BranchRenamed), Key: "publish.rules.renamed",
		Params: map[string]string{"from": rename.from, "to": rename.to, "rule": rename.broken.Label(), "requirement": rename.broken.Describe()}})
	return branch, nil
}
