// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"time"
)

// Checkpoint is an invisible snapshot of the whole working tree, taken
// before ship-it changes anything (ADR-0005).
type Checkpoint struct {
	// ID is the commit object holding the snapshot.
	ID        string
	CreatedAt time.Time
	// Label names the operation that triggered the checkpoint ("deliver"…).
	Label string
	// Branch and Head describe where the user was.
	Branch string
	Head   string
}

// CheckpointStore keeps checkpoints local and never pushes them.
type CheckpointStore interface {
	// Create snapshots the working tree, untracked files included and
	// ignored files excluded, without touching the index, HEAD or files.
	Create(ctx context.Context, label string) (Checkpoint, error)
	// List returns checkpoints, newest first, at most limit.
	List(ctx context.Context, limit int) ([]Checkpoint, error)
	// RestoreFiles writes back every file of the checkpoint into the
	// working tree. Files created after the checkpoint are left untouched.
	RestoreFiles(ctx context.Context, id string) error
	// Remove deletes checkpoints and frees their storage. Unknown ids are
	// ignored; the other checkpoints keep their ids.
	Remove(ctx context.Context, ids []string) error
	// Changes lists the files that differ from checkpoint from to checkpoint
	// to; from "" compares with nothing (every file of to is added).
	Changes(ctx context.Context, from, to string) ([]CheckpointChange, error)
}

// CheckpointChange is one file that differs between two checkpoints.
type CheckpointChange struct {
	Path string `json:"path"`
	// Status is "added", "modified" or "deleted", going from one to the other.
	Status string `json:"status"`
}

// Statuses of a CheckpointChange.
const (
	ChangeAdded    = "added"
	ChangeModified = "modified"
	ChangeDeleted  = "deleted"
)
