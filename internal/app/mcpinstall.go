// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/mcpconfig"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// MCPInstall shows, then applies, the configuration change.
type MCPInstall struct {
	// Client is the MCP client configured: claude, vscode, cursor or codex.
	Client  string `json:"client"`
	Path    string `json:"path"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Changed bool   `json:"changed"`
	// Kept: the file already declares a ship-it server launched another way;
	// it stays as the user wrote it.
	Kept    bool `json:"kept,omitempty"`
	Applied bool `json:"applied"`
	// Operation is the oplog entry: ship undo restores the file.
	Operation string `json:"operation,omitempty"`
	// Rehearsal: on a dry run, Applied says the file would be written.
	Rehearsal
}

// MCPClients lists the clients ship mcp install configures, the default first.
func MCPClients() []string {
	names := make([]string, 0, len(mcpconfig.Targets))
	for _, t := range mcpconfig.Targets {
		names = append(names, string(t.Client))
	}
	return names
}

// MCPSnippet is what declares ship-it for a client, and the file it goes in.
func MCPSnippet(client string) (path, snippet string, err error) {
	target, err := mcpTarget(client)
	return target.Path, string(target.Snippet()), err
}

func mcpTarget(client string) (mcpconfig.Target, error) {
	target, ok := mcpconfig.TargetOf(client)
	if !ok {
		// Surfaces check the name against MCPClients first.
		return target, fmt.Errorf("app: unknown MCP client %q (%s)", client, strings.Join(MCPClients(), ", "))
	}
	return target, nil
}

// InstallMCP adds ship-it to the project MCP configuration of a client
// ("" is Claude Code), keeping everything else; with apply false it only
// computes the result to show.
func InstallMCP(ctx context.Context, ws Workspace, client string, apply bool) (view MCPInstall, err error) {
	mark := ws.planMark()
	defer func() { view.Rehearsal = ws.rehearsed(mark) }()
	target, err := mcpTarget(client)
	if err != nil {
		return view, err
	}
	view = MCPInstall{Client: string(target.Client), Path: target.Path}
	before := oplog.FileBefore{Path: target.Path}
	content, err := ws.Git.ReadWorkingFile(ctx, target.Path, packageJSONLimit)
	switch {
	case err == nil:
		before.Content, before.Existed = string(content), true
	case errors.Is(err, ports.ErrFileTooLarge):
		return view, err
	}
	result, err := target.Add(content)
	switch {
	case errors.Is(err, mcpconfig.ErrManualEdit):
		return view, fault.New(fault.MCPManualEdit, "mcp.manual_edit", "file", target.Path)
	case err != nil:
		return view, fault.New(fault.ConfigInvalidJSON, "config.invalid_json",
			"file", target.Path, "line", "1", "column", "1", "detail", err.Error())
	}
	view.Before, view.After, view.Changed, view.Kept = before.Content, string(result.Content), result.Changed, result.Kept
	if !apply || !result.Changed {
		return view, nil
	}
	if err := ws.Git.WriteWorkingFile(ctx, target.Path, result.Content); err != nil {
		return view, err
	}
	view.Applied = true
	op, err := ws.Operations.Append(ctx, oplog.Operation{Kind: oplog.Fix, FilesBefore: []oplog.FileBefore{before}})
	view.Operation = op.ID
	return view, err
}
