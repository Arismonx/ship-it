// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/skill"
	"github.com/GregoireF/ship-it/internal/ports"
)

// SkillFile is one SKILL.md ship skill install writes, or leaves alone.
type SkillFile struct {
	Path string `json:"path"`
	// Created: there was no file; Updated: an older SKILL.md of ship-it's.
	Created bool `json:"created,omitempty"`
	Updated bool `json:"updated,omitempty"`
	// Kept: a SKILL.md ship-it did not write is there; it stays the user's.
	Kept bool `json:"kept,omitempty"`
}

// SkillInstall shows, then applies, the installation of the skill (#50).
type SkillInstall struct {
	Files   []SkillFile `json:"files"`
	Changed bool        `json:"changed"`
	Applied bool        `json:"applied"`
	// Operation is the oplog entry: ship undo removes or restores the files.
	Operation string `json:"operation,omitempty"`
	Rehearsal
}

// skillTargets returns where to write for a client, "" for every agent.
func skillTargets(client string) ([]skill.Target, error) {
	if client == "" {
		return skill.Targets, nil
	}
	t, ok := skill.For(client)
	if !ok {
		return nil, fmt.Errorf("app: unknown skill client %q", client)
	}
	return []skill.Target{t}, nil
}

// planSkill compares the files with the skill to install.
func planSkill(ctx context.Context, ws Workspace, targets []skill.Target) ([]SkillFile, []oplog.FileBefore, error) {
	var files []SkillFile
	var befores []oplog.FileBefore
	want := skill.Content()
	for _, t := range targets {
		f := SkillFile{Path: t.Path()}
		before := oplog.FileBefore{Path: f.Path}
		content, err := ws.Git.ReadWorkingFile(ctx, f.Path, packageJSONLimit)
		switch {
		case err == nil:
			before.Content, before.Existed = string(content), true
		case errors.Is(err, ports.ErrFileTooLarge):
			f.Kept = true // too large to be ours
		}
		switch {
		case f.Kept:
		case !before.Existed:
			f.Created = true
		case bytes.Equal(content, want):
		case skill.Ours(content):
			f.Updated = true
		default:
			f.Kept = true
		}
		files = append(files, f)
		if f.Created || f.Updated {
			befores = append(befores, before)
		}
	}
	return files, befores, nil
}

// writeSkill writes the files planSkill would change.
func writeSkill(ctx context.Context, ws Workspace, files []SkillFile) error {
	for _, f := range files {
		if f.Created || f.Updated {
			if err := ws.Git.WriteWorkingFile(ctx, f.Path, skill.Content()); err != nil {
				return err
			}
		}
	}
	return nil
}

// InstallSkill writes ship-it's Agent Skill where the agents of a client
// read it ("" for every agent: .agents/skills and .claude/skills). A
// SKILL.md ship-it did not write is never replaced. With apply false it only
// computes what would change.
func InstallSkill(ctx context.Context, ws Workspace, client string, apply bool) (view SkillInstall, err error) {
	mark := ws.planMark()
	defer func() { view.Rehearsal = ws.rehearsed(mark) }()
	targets, err := skillTargets(client)
	if err != nil {
		return view, err
	}
	files, befores, err := planSkill(ctx, ws, targets)
	if err != nil {
		return view, err
	}
	view.Files, view.Changed = files, len(befores) > 0
	if !apply || !view.Changed {
		return view, nil
	}
	if err := writeSkill(ctx, ws, files); err != nil {
		return view, err
	}
	view.Applied = true
	op, err := ws.Operations.Append(ctx, oplog.Operation{Kind: oplog.Fix, FilesBefore: befores})
	view.Operation = op.ID
	return view, err
}

// SkillContent is the SKILL.md ship skill install writes.
func SkillContent() []byte { return skill.Content() }
