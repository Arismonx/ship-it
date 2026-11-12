// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import "context"

// Kinds of leftovers.
const (
	// LeftoverRepository is .git/ship: checkpoints, the operation log, the
	// run logs and the roadmap memory of one repository.
	LeftoverRepository = "repository"
	// LeftoverUser is one directory of the user's: configuration and salt,
	// cache, logs.
	LeftoverUser = "user"
	// LeftoverAgentFile is a project file of a coding agent that only held
	// ship-it (ship mcp install); Path is relative to the repository root.
	LeftoverAgentFile = "agent"
)

// Leftover is something ship-it wrote, found by Footprint (#55).
type Leftover struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	// Holds names what a user directory holds: config, cache, logs.
	Holds []string `json:"holds,omitempty"`
	Bytes int64    `json:"bytes"`
	Files int      `json:"files"`
}

// Footprint finds and removes what ship-it wrote outside the working tree's
// tracked content. Only what it finds can be removed: Remove refuses any
// other path.
type Footprint interface {
	// Repository returns .git/ship of the current repository, if any.
	Repository(ctx context.Context) ([]Leftover, error)
	// User returns ship-it's directories of the user, those that exist.
	User(ctx context.Context) ([]Leftover, error)
	// Remove deletes a leftover: a directory with everything in it, or an
	// agent file.
	Remove(ctx context.Context, l Leftover) error
}
