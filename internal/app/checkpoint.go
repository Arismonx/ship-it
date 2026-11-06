// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
)

// shortIDLength is the abbreviated id shown to humans; full ids stay in JSON.
const shortIDLength = 12

// CheckpointView is a checkpoint as surfaces render it.
type CheckpointView struct {
	ID        string    `json:"id"`
	ShortID   string    `json:"short_id"`
	CreatedAt time.Time `json:"created_at"`
	Label     string    `json:"label"`
	Branch    string    `json:"branch,omitempty"`
}

func viewOf(c ports.Checkpoint) CheckpointView {
	short := c.ID
	if len(short) > shortIDLength {
		short = short[:shortIDLength]
	}
	return CheckpointView{ID: c.ID, ShortID: short, CreatedAt: c.CreatedAt, Label: c.Label, Branch: c.Branch}
}

// CreateCheckpoint snapshots the working tree without changing anything.
func CreateCheckpoint(ctx context.Context, store ports.CheckpointStore, label string) (CheckpointView, error) {
	if strings.TrimSpace(label) == "" {
		label = "manual"
	}
	cp, err := store.Create(ctx, label)
	if err != nil {
		return CheckpointView{}, err
	}
	return viewOf(cp), nil
}

// ListCheckpoints returns the most recent checkpoints, newest first.
func ListCheckpoints(ctx context.Context, store ports.CheckpointStore, limit int) ([]CheckpointView, error) {
	checkpoints, err := store.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	views := make([]CheckpointView, 0, len(checkpoints))
	for _, c := range checkpoints {
		views = append(views, viewOf(c))
	}
	return views, nil
}

// RestoreResult reports a restore and the checkpoint that makes it undoable.
type RestoreResult struct {
	Restored CheckpointView `json:"restored"`
	Safety   CheckpointView `json:"safety"`
}

// RestoreCheckpoint writes back the files of a checkpoint, given by its full
// or abbreviated id. A safety checkpoint is taken first, so a restore can
// itself be undone.
func RestoreCheckpoint(ctx context.Context, store ports.CheckpointStore, id string) (RestoreResult, error) {
	target, err := findCheckpoint(ctx, store, id)
	if err != nil {
		return RestoreResult{}, err
	}
	safety, err := store.Create(ctx, "before-restore")
	if err != nil {
		return RestoreResult{}, err
	}
	if err := store.RestoreFiles(ctx, target.ID); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Restored: viewOf(target), Safety: viewOf(safety)}, nil
}

// checkpointSearchDepth bounds how far an abbreviated id is looked up.
const checkpointSearchDepth = 1000

func findCheckpoint(ctx context.Context, store ports.CheckpointStore, id string) (ports.Checkpoint, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	checkpoints, err := store.List(ctx, checkpointSearchDepth)
	if err != nil {
		return ports.Checkpoint{}, err
	}
	var match *ports.Checkpoint
	for i := range checkpoints {
		if id != "" && strings.HasPrefix(checkpoints[i].ID, id) {
			if match != nil {
				return ports.Checkpoint{}, fault.New(fault.AmbiguousCheckpoint, "checkpoint.ambiguous", "id", id)
			}
			match = &checkpoints[i]
		}
	}
	if match == nil {
		return ports.Checkpoint{}, fault.New(fault.UnknownCheckpoint, "checkpoint.unknown", "id", id)
	}
	return *match, nil
}
