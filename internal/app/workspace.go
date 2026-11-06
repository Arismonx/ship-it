// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"log/slog"

	"github.com/GregoireF/ship-it/internal/ports"
)

// Workspace is the repository the use cases act on, reached through ports.
type Workspace struct {
	Git         ports.Git
	Checkpoints ports.CheckpointStore
	Operations  ports.OperationLog
	Forge       ports.Forge
	// Stacks is GitHub's stacks API; nil means none (a classic chain).
	Stacks ports.Stacks
	// Tracker is GitHub's milestones and issues, and RoadmapMemory the state
	// of the last sync, for ship roadmap sync.
	Tracker       ports.Tracker
	RoadmapMemory ports.RoadmapMemory
	// Projects is the GitHub Project (v2) of github.project; nil skips it.
	Projects        ports.Projects
	Credentials     ports.Credentials
	Config          ports.ConfigFiles
	Runner          ports.CommandRunner
	OperatingSystem string
	// Environ is the process environment, for the SHIP_* settings; Home is
	// the user's home directory, for "~/" in configuration contexts.
	Environ []string
	Home    string
	// SettingFlags are the --set key=value assignments of this invocation.
	SettingFlags []string
	// Agent: an AI agent drives this workspace (MCP, nobody reviewing). It
	// keeps its safeguards whatever the configuration says
	// (docs/design/configurability.md): see scanOptions.
	Agent bool
	// RunLogs are the journals of past runs, pruned by `ship gc`; nil skips.
	RunLogs ports.RunLogs
	// Log is the technical journal (docs/reference/logs.md); nil logs
	// nothing. Use cases log stable message keys, never sentences.
	Log *slog.Logger

	// rehearsal is set by DryRun: the ports then change nothing.
	rehearsal *rehearsal
}
