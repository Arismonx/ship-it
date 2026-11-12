// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/mcpconfig"
	"github.com/GregoireF/ship-it/internal/ports"
)

// UninstallPlan is what ship uninstall removes, then what it removed (#55).
type UninstallPlan struct {
	// User: the user's files, not the repository's.
	User bool `json:"user"`
	// Remove are the directories and files that go.
	Remove []ports.Leftover `json:"remove,omitempty"`
	// Edit are agent files shared with other servers: only ship-it's entry
	// goes.
	Edit []string `json:"edit,omitempty"`
	// ByHand are agent files whose ship-it entry the user wrote some other
	// way: ship-it leaves them alone.
	ByHand []string `json:"byHand,omitempty"`
	// Checkpoints is how many checkpoints go with .git/ship: ship undo
	// cannot restore anything afterwards.
	Checkpoints int  `json:"checkpoints"`
	Applied     bool `json:"applied"`
}

// Empty reports whether there is nothing to remove.
func (p UninstallPlan) Empty() bool { return len(p.Remove) == 0 && len(p.Edit) == 0 }

// checkpointsCounted bounds the count of checkpoints shown.
const checkpointsCounted = 10000

// Uninstall finds what ship-it wrote in the repository (.git/ship and the
// agent files of ship mcp install) or, with user, in the user's folders;
// with apply, it removes it. Nothing is recorded: the operation log goes too,
// so it cannot be undone. Never an AI agent's call: surfaces confirm with a
// human first.
func Uninstall(ctx context.Context, ws Workspace, user, apply bool) (UninstallPlan, error) {
	plan := UninstallPlan{User: user}
	if ws.Footprint == nil {
		return plan, errors.New("app: no footprint to uninstall")
	}
	type edit struct {
		path    string
		content []byte
	}
	var edits []edit
	if user {
		found, err := ws.Footprint.User(ctx)
		if err != nil {
			return plan, err
		}
		plan.Remove = found
	} else {
		if _, err := ws.Git.TopLevel(ctx); err != nil {
			return plan, fault.New(fault.NotInRepository, "app.uninstall_outside", "command", "ship uninstall")
		}
		for _, target := range mcpconfig.Targets {
			content, err := ws.Git.ReadWorkingFile(ctx, target.Path, packageJSONLimit)
			if err != nil {
				continue // no such file: nothing to take out
			}
			removal, err := target.Remove(content)
			switch {
			case err != nil:
				plan.ByHand = append(plan.ByHand, target.Path)
			case removal.Empty:
				plan.Remove = append(plan.Remove, ports.Leftover{Kind: ports.LeftoverAgentFile, Path: target.Path, Files: 1, Bytes: int64(len(content))})
			case removal.Changed:
				plan.Edit = append(plan.Edit, target.Path)
				edits = append(edits, edit{target.Path, removal.Content})
			}
		}
		found, err := ws.Footprint.Repository(ctx)
		if err != nil {
			return plan, err
		}
		if len(found) > 0 && ws.Checkpoints != nil {
			if list, err := ws.Checkpoints.List(ctx, checkpointsCounted); err == nil {
				plan.Checkpoints = len(list)
			}
		}
		plan.Remove = append(plan.Remove, found...)
	}
	if !apply {
		return plan, nil
	}
	for _, e := range edits {
		if err := ws.Git.WriteWorkingFile(ctx, e.path, e.content); err != nil {
			return plan, err
		}
	}
	for _, l := range plan.Remove {
		if err := ws.Footprint.Remove(ctx, l); err != nil {
			return plan, err
		}
	}
	plan.Applied = true
	return plan, nil
}
