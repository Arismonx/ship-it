// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// RewindEntry is one checkpoint of ship rewind's list, with the files that
// changed since the checkpoint before it (#39).
type RewindEntry struct {
	CheckpointView
	Changes []ports.CheckpointChange `json:"changes"`
}

// RewindList returns the limit most recent checkpoints, newest first, each
// with what changed since the one before.
func RewindList(ctx context.Context, ws Workspace, limit int) ([]RewindEntry, error) {
	checkpoints, err := ws.Checkpoints.List(ctx, limit+1)
	if err != nil {
		return nil, err
	}
	var entries []RewindEntry
	for i, c := range checkpoints {
		if i == limit {
			break
		}
		previous := ""
		if i+1 < len(checkpoints) {
			previous = checkpoints[i+1].ID
		}
		changes, err := ws.Checkpoints.Changes(ctx, previous, c.ID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, RewindEntry{CheckpointView: viewOf(c), Changes: changes})
	}
	return entries, nil
}

// RewindPlan is a rewind ready to apply: the safety checkpoint is taken, and
// the files the restore would write are known.
type RewindPlan struct {
	Target CheckpointView `json:"target"`
	Safety CheckpointView `json:"safety"`
	// Rewrites are the files the restore writes: modified ones and those
	// that came back. Files created since the target stay as they are.
	Rewrites []ports.CheckpointChange `json:"rewrites"`
	Kept     []string                 `json:"kept,omitempty"`
	Rehearsal
}

// PrepareRewind finds the checkpoint (an id, or its number in the list,
// 1 the newest), takes the safety checkpoint and compares. Nothing is
// written yet: ApplyRewind writes, CancelRewind drops the safety checkpoint.
func PrepareRewind(ctx context.Context, ws Workspace, choice string) (plan RewindPlan, err error) {
	mark := ws.planMark()
	defer func() { plan.Rehearsal = ws.rehearsed(mark) }()
	target, err := rewindTarget(ctx, ws, choice)
	if err != nil {
		return plan, err
	}
	plan.Target = viewOf(target)
	safety, err := ws.Checkpoints.Create(ctx, "before-rewind")
	if err != nil {
		return plan, err
	}
	plan.Safety = viewOf(safety)
	if safety.ID == "" {
		return plan, nil // a dry run takes no checkpoint, so cannot compare
	}
	changes, err := ws.Checkpoints.Changes(ctx, safety.ID, target.ID)
	if err != nil {
		return plan, err
	}
	for _, c := range changes {
		if c.Status == ports.ChangeDeleted {
			plan.Kept = append(plan.Kept, c.Path)
			continue
		}
		plan.Rewrites = append(plan.Rewrites, c)
	}
	return plan, nil
}

// rewindTarget reads a choice: a number of the list, or a checkpoint id.
func rewindTarget(ctx context.Context, ws Workspace, choice string) (ports.Checkpoint, error) {
	choice = strings.TrimSpace(choice)
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && len(choice) < 6 {
		checkpoints, err := ws.Checkpoints.List(ctx, n)
		if err != nil {
			return ports.Checkpoint{}, err
		}
		if n > len(checkpoints) {
			return ports.Checkpoint{}, fault.New(fault.UnknownCheckpoint, "checkpoint.unknown", "id", choice)
		}
		return checkpoints[n-1], nil
	}
	return findCheckpoint(ctx, ws.Checkpoints, choice)
}

// CancelRewind drops the safety checkpoint of a rewind not applied: nothing
// changed, so there is nothing to keep.
func CancelRewind(ctx context.Context, ws Workspace, plan RewindPlan) error {
	if plan.Safety.ID == "" {
		return nil
	}
	return ws.Checkpoints.Remove(ctx, []string{plan.Safety.ID})
}

// RewindResult is a rewind applied, and the operation ship undo reverts.
type RewindResult struct {
	RewindPlan
	Operation string `json:"operation,omitempty"`
}

// ApplyRewind writes back the files of the target and records the
// operation: ship undo restores the safety checkpoint and removes the files
// the rewind recreated.
func ApplyRewind(ctx context.Context, ws Workspace, plan RewindPlan) (RewindResult, error) {
	result := RewindResult{RewindPlan: plan}
	if err := ws.Checkpoints.RestoreFiles(ctx, plan.Target.ID); err != nil {
		return result, err
	}
	op := oplog.Operation{Kind: oplog.Rewind, Checkpoint: plan.Safety.ID, Title: plan.Target.ShortID}
	for _, c := range plan.Rewrites {
		if c.Status == ports.ChangeAdded {
			op.FilesBefore = append(op.FilesBefore, oplog.FileBefore{Path: c.Path})
		}
	}
	recorded, err := ws.Operations.Append(ctx, op)
	result.Operation = recorded.ID
	return result, err
}
