// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fix"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// gitignorePath is the file the untrack remedy completes.
const gitignorePath = ".gitignore"

// FixRequest says whether to apply the remedies, once the human agreed.
type FixRequest struct {
	Apply bool
}

// FixReport lists what ship fix found and, after Apply, what it did.
type FixReport struct {
	Remedies []fix.Remedy `json:"remedies"`
	// Manual are problems a person must solve (an unfinished merge…).
	Manual  []FindingView `json:"manual,omitempty"`
	Applied bool          `json:"applied"`
	// Operation is the oplog entry of the applied remedies.
	Operation string      `json:"operation,omitempty"`
	Tidy      *TidyResult `json:"tidy,omitempty"`
	Rehearsal
}

// Fix runs the detectors of the playbook (docs/reference/error-playbook.md) and, with Apply, applies
// every remedy after a checkpoint, as one operation that ship undo reverts.
func Fix(ctx context.Context, ws Workspace, req FixRequest) (FixReport, error) {
	done := ws.begin(ctx, "fix")
	mark := ws.planMark()
	report, err := applyFix(ctx, ws, req)
	report.Rehearsal = ws.rehearsed(mark)
	done(err, operation(report.Operation)...)
	return report, err
}

func applyFix(ctx context.Context, ws Workspace, req FixRequest) (FixReport, error) {
	facts := fix.FixFacts{Facts: gatherFacts(ctx, ws.Git, ws.OperatingSystem, gitRemote(ctx, ws), nil)}
	if !facts.InsideRepository {
		return FixReport{Manual: CheckUp(ctx, ws).Findings}, nil
	}
	repo, repoErr := remoteRepository(ctx, ws)
	if repoErr == nil && ws.Forge != nil {
		if user, err := ws.Forge.CurrentUser(ctx, repo); err == nil {
			facts.GitHubUser = &fix.GitHubUser{Login: user.Login, ID: user.ID, Name: user.Name}
		}
		facts.MergedBranches = countMergedBranches(ctx, ws)
	}
	facts.Tracked, _ = ws.Git.TrackedFiles(ctx)
	if content, err := ws.Git.ReadWorkingFile(ctx, gitignorePath, packageJSONLimit); err == nil {
		facts.Gitignore = string(content)
	}

	report := FixReport{Remedies: fix.Remedies(facts)}
	covered := map[string]bool{}
	for _, r := range report.Remedies {
		covered[string(r.Code)] = true
	}
	for _, f := range CheckUp(ctx, ws).Findings {
		if f.Severity != "ok" && !covered[f.Code] {
			report.Manual = append(report.Manual, f)
		}
	}
	if !req.Apply || len(report.Remedies) == 0 {
		return report, nil
	}
	return applyRemedies(ctx, ws, report)
}

func applyRemedies(ctx context.Context, ws Workspace, report FixReport) (FixReport, error) {
	if _, err := ws.Checkpoints.Create(ctx, "before-fix"); err != nil {
		return report, err
	}
	var op oplog.Operation
	op.Kind = oplog.Fix
	var applyErr error
	for _, r := range report.Remedies {
		switch r.Kind {
		case fix.SetIdentity:
			if applyErr = setConfig(ctx, ws, &op, "user.name", r.Params["name"], true); applyErr == nil {
				applyErr = setConfig(ctx, ws, &op, "user.email", r.Params["email"], true)
			}
		case fix.EnableLongPaths:
			applyErr = setConfig(ctx, ws, &op, r.ConfigKey, r.ConfigValue, false)
		case fix.DisableMirror:
			applyErr = setConfig(ctx, ws, &op, r.ConfigKey, "", false)
		case fix.UntrackFiles:
			applyErr = untrack(ctx, ws, &op, r)
		case fix.TidyBranches:
			tidy, err := Tidy(ctx, ws)
			report.Tidy, applyErr = &tidy, err
		}
		if applyErr != nil {
			break
		}
	}
	// What was applied is logged even after a failure, so that it can be undone.
	if len(op.ConfigChanges) > 0 || len(op.Untracked) > 0 || len(op.FilesBefore) > 0 {
		logged, err := ws.Operations.Append(ctx, op)
		report.Operation = logged.ID
		applyErr = errors.Join(applyErr, err)
	}
	report.Applied = applyErr == nil
	return report, applyErr
}

// setConfig writes (or, with an empty value, removes) a key and records what
// that scope held before.
func setConfig(ctx context.Context, ws Workspace, op *oplog.Operation, key, value string, global bool) error {
	old, had, err := ws.Git.ScopedConfigValue(ctx, key, global)
	if err != nil {
		return err
	}
	if value == "" {
		err = ws.Git.UnsetConfig(ctx, key, global)
	} else {
		err = ws.Git.SetConfig(ctx, key, value, global)
	}
	if err != nil {
		return err
	}
	op.ConfigChanges = append(op.ConfigChanges, oplog.ConfigChange{Key: key, Global: global, Old: old, HadOld: had})
	return nil
}

// untrack completes .gitignore, then removes the files from the index; they
// stay on disk.
func untrack(ctx context.Context, ws Workspace, op *oplog.Operation, r fix.Remedy) error {
	if len(r.Patterns) > 0 {
		before := oplog.FileBefore{Path: gitignorePath}
		content, err := ws.Git.ReadWorkingFile(ctx, gitignorePath, packageJSONLimit)
		switch {
		case err == nil:
			before.Content, before.Existed = string(content), true
		case !errors.Is(err, ports.ErrFileTooLarge):
			// absent: it will be created
		default:
			return err
		}
		updated := before.Content
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += "# Added by ship fix: never commit these.\n" + strings.Join(r.Patterns, "\n") + "\n"
		if err := ws.Git.WriteWorkingFile(ctx, gitignorePath, []byte(updated)); err != nil {
			return err
		}
		op.FilesBefore = append(op.FilesBefore, before)
	}
	if err := ws.Git.Untrack(ctx, r.Paths); err != nil {
		return err
	}
	op.Untracked = append(op.Untracked, r.Paths...)
	return nil
}

// countMergedBranches counts local branches whose pull request is merged and
// that hold nothing more: what a tidy would delete.
func countMergedBranches(ctx context.Context, ws Workspace) int {
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return 0
	}
	merged, err := ws.Forge.MergedPullRequests(ctx, repo, mergedPullRequestScan)
	if err != nil {
		return 0
	}
	locals, err := ws.Git.LocalBranches(ctx)
	if err != nil {
		return 0
	}
	count := 0
	var seen []string
	trunk := defaultBranchOf(ctx, ws)
	for _, pr := range merged {
		tip, local := locals[pr.HeadBranch]
		if !local || isTrunk(pr.HeadBranch, trunk) || slices.Contains(seen, pr.HeadBranch) {
			continue
		}
		if tip == pr.HeadSHA {
			count++
			seen = append(seen, pr.HeadBranch)
		} else if ok, err := ws.Git.IsAncestor(ctx, tip, pr.HeadSHA); err == nil && ok {
			count++
			seen = append(seen, pr.HeadBranch)
		}
	}
	return count
}
