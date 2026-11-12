// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// LogEntry is one operation as `ship log` shows it.
type LogEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	StartedAt time.Time `json:"started_at"`
	Title     string    `json:"title,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	URL       string    `json:"url,omitempty"`
	// Deleted lists the branches a tidy removed.
	Deleted []string `json:"deleted,omitempty"`
	// Undoes names the operation an undo reverted.
	Undoes string `json:"undoes,omitempty"`
	// UndoneBy names the undo that reverted this operation.
	UndoneBy string `json:"undone_by,omitempty"`
	// Next marks the operation `ship undo` would revert now.
	Next bool `json:"next,omitempty"`
	// Agent names the AI agent that made it, empty for a person.
	Agent string `json:"agent,omitempty"`
}

// Log returns the most recent operations, newest first.
func Log(ctx context.Context, ws Workspace, limit int) ([]LogEntry, error) {
	ops, err := ws.Operations.Recent(ctx, limit)
	if err != nil {
		return nil, err
	}
	undoneBy := map[string]string{}
	for _, op := range ops {
		if op.Kind == oplog.Undo {
			undoneBy[op.Undoes] = op.ID
		}
	}
	next, hasNext := oplog.LastUndoable(ops)
	entries := make([]LogEntry, 0, len(ops))
	for _, op := range ops {
		entry := LogEntry{
			ID: op.ID, Kind: string(op.Kind), StartedAt: op.StartedAt, Title: op.Title,
			Branch: op.After.Branch, URL: op.PullRequestURL, Undoes: op.Undoes, UndoneBy: undoneBy[op.ID],
			Next: hasNext && op.ID == next.ID, Agent: op.Agent,
		}
		for _, d := range op.DeletedBranches {
			entry.Deleted = append(entry.Deleted, d.Name)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// AllAgents filters FilterByAgent to the operations of any agent.
const AllAgents = "*"

// FilterByAgent keeps the entries an agent made: all agents with AllAgents,
// one agent by its name, without regard to case.
func FilterByAgent(entries []LogEntry, agent string) []LogEntry {
	var kept []LogEntry
	for _, e := range entries {
		if e.Agent != "" && (agent == AllAgents || strings.EqualFold(e.Agent, agent)) {
			kept = append(kept, e)
		}
	}
	return kept
}

// agentLog stamps an agent's name on every operation it records (#49).
type agentLog struct {
	ports.OperationLog
	name string
}

func (l agentLog) Append(ctx context.Context, op oplog.Operation) (oplog.Operation, error) {
	if op.Agent == "" {
		op.Agent = l.name
	}
	return l.OperationLog.Append(ctx, op)
}

// WithAgent returns ws recording name as the agent of every operation it
// makes; an empty name changes nothing.
func WithAgent(ws Workspace, name string) Workspace {
	if name != "" && ws.Operations != nil {
		ws.Operations = agentLog{OperationLog: ws.Operations, name: name}
	}
	return ws
}
