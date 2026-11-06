// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/schema"
)

// hostAgent is the AI-assisted mention of plans written by the MCP client's
// model (docs/reference/conventions.md: "AI-assisted (host agent)").
const hostAgent = "host agent"

// logLimit bounds ship_log.
const logLimit = 20

// tool is one MCP tool: its definition and what runs it.
type tool struct {
	name        string
	title       string
	description string
	input       map[string]any
	output      any // a zero value of the result type: its schema is generated
	readOnly    bool
	destructive bool
	idempotent  bool
	openWorld   bool
	run         func(ctx context.Context, s *Server, args json.RawMessage) (any, error)
}

// LogResult wraps ship_log's entries: an object leaves room to grow.
type LogResult struct {
	Operations []app.LogEntry `json:"operations"`
}

func object(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

var onlyProperty = map[string]any{"type": "array", "items": map[string]any{"type": "string"},
	"description": "Deliver only these files or directories (relative to the repository root); the rest stays uncommitted."}

var dryRunProperty = map[string]any{"type": "boolean",
	"description": "Preview: run every check and return the result with the planned steps (planned), changing nothing: no commit, no push, no pull request, no checkpoint, no log entry."}

// workspace is the server's workspace, one that changes nothing on a dry run.
// Without --attended an AI agent drives it, nobody reviewing: it keeps its
// safeguards (app.Workspace.Agent).
func (s *Server) workspace(dryRun bool) app.Workspace {
	ws := s.Workspace
	ws.Agent = !s.Attended
	if dryRun {
		return app.DryRun(ws)
	}
	return ws
}

var issueProperty = map[string]any{"type": "integer", "minimum": 1, "description": "Issue the work closes (default: read from the branch name)."}

// tools are listed in this order: deterministic, as clients cache the list.
var tools = []tool{
	{
		name: "ship_status", title: "Where the work stands",
		description: "Branch, uncommitted files, ahead/behind the default branch, open pull request, what ship_undo would revert, and the one next action.",
		input:       object(map[string]any{"offline": map[string]any{"type": "boolean", "description": "Do not ask GitHub."}}),
		output:      app.StatusView{}, readOnly: true, openWorld: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			var args struct {
				Offline bool `json:"offline"`
			}
			if err := decode(raw, &args); err != nil {
				return nil, err
			}
			return app.Status(ctx, s.Workspace, app.StatusRequest{Offline: args.Offline})
		},
	},
	{
		name: "ship_plan", title: "Plan a delivery",
		description: "What ship_deliver would do with the work in progress: conventional title, branch, files, reasons, secrets found, and a split suggestion when it is large. Changes nothing.",
		input:       object(map[string]any{"only": onlyProperty, "issue": issueProperty}),
		output:      app.PlanView{}, readOnly: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			var args struct {
				Only  []string `json:"only"`
				Issue int      `json:"issue"`
			}
			if err := decode(raw, &args); err != nil {
				return nil, err
			}
			view, err := app.ProposePlan(ctx, s.workspace(false), app.PlanRequest{Only: args.Only, Issue: args.Issue})
			for i, r := range view.Reasons {
				view.Reasons[i].Text = i18n.Render(i18n.English, r.Key, r.Params)
			}
			return view, err
		},
	},
	{
		name: "ship_deliver", title: "Commit the work locally",
		description: "Checkpoint, dedicated branch when on the default branch, then one conventional commit. Optional title (\"feat(scope): subject\", ≤ 50-character subject) and branch (\"feat/short-name\") are validated; without them the deterministic plan decides. Refuses changes containing secrets. Nothing is pushed: see ship_publish.",
		input: object(map[string]any{
			"title":  map[string]any{"type": "string", "description": "Conventional commit header, e.g. \"fix(api): handle empty cart\"."},
			"branch": map[string]any{"type": "string", "description": "Branch to create when on the default branch, e.g. \"fix/empty-cart\"."},
			"only":   onlyProperty, "issue": issueProperty, "dryRun": dryRunProperty,
			"why":    map[string]any{"type": "string", "description": "Why the change was made, in a sentence or two: the commit body and the pull request summary. Only facts the user or the code state."},
			"tested": map[string]any{"type": "string", "description": "How the change was actually tested (commands run, scenarios checked), recorded as a Tested: trailer. Leave empty rather than guess."},
		}),
		output: app.DeliveryResult{},
		run:    deliver,
	},
	{
		name: "ship_publish", title: "Push and open a draft pull request",
		description: "Runs the project's checks (lint, typecheck, test), updates the branch on the default branch, pushes it and opens a DRAFT pull request, or updates the open one. A human marks it ready for review.",
		input:       object(map[string]any{"issue": issueProperty, "dryRun": dryRunProperty}),
		output:      app.PublishResult{}, openWorld: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			var args struct {
				Issue  int  `json:"issue"`
				DryRun bool `json:"dryRun"`
			}
			if err := decode(raw, &args); err != nil {
				return nil, err
			}
			// ADR-0010: every pull request opened by an agent is a draft.
			return app.Publish(ctx, s.workspace(args.DryRun), app.PublishRequest{Issue: args.Issue, Unattended: !s.Attended, ForceDraft: !s.Attended})
		},
	},
	{
		name: "ship_undo", title: "Undo the last operation",
		description: "Reverts the last local operation of ship-it (a delivery, a tidy, a fix, a configuration edit), keeping the files. Undoing a publication changes GitHub: it is refused here and left to a human (ship undo in a terminal).",
		input: object(map[string]any{"allowRemote": map[string]any{"type": "boolean",
			"description": "Close the pull request and delete the remote branch. Honored only for a human interface (ship mcp --attended), after the human confirmed."},
			"dryRun": dryRunProperty}),
		output: app.UndoResult{}, destructive: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			var args struct {
				AllowRemote bool `json:"allowRemote"`
				DryRun      bool `json:"dryRun"`
			}
			if err := decode(raw, &args); err != nil {
				return nil, err
			}
			return app.Undo(ctx, s.workspace(args.DryRun), app.UndoRequest{AllowRemote: args.AllowRemote && s.Attended})
		},
	},
	{
		name: "ship_log", title: "What ship-it did",
		description: "The latest operations of ship-it in this repository, newest first, with the one ship_undo would revert.",
		input:       object(map[string]any{}),
		output:      LogResult{}, readOnly: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			entries, err := app.Log(ctx, s.Workspace, logLimit)
			return LogResult{Operations: entries}, err
		},
	},
	{
		name: "ship_doctor", title: "Check git, configuration and GitHub",
		description: "Every problem that would stop a delivery, each with the exact command that fixes it.",
		input:       object(map[string]any{}),
		output:      app.DoctorReport{}, readOnly: true, openWorld: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			return app.CheckUp(ctx, s.Workspace), nil
		},
	},
	{
		name: "ship_config_get", title: "Read one setting",
		description: "The effective value of one ship-it setting (such as ship.pullRequest.maxLines) and the layer it comes from: default, preset, user file, context, repository file, override, SHIP_* variable or policy. Read-only: settings are changed by a human (ship config set).",
		input:       object(map[string]any{"key": settingKeyProperty}),
		output:      app.SettingValue{}, readOnly: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			key, err := settingKey(raw)
			if err != nil {
				return nil, err
			}
			return app.GetSetting(ctx, s.Workspace, key)
		},
	},
	{
		name: "ship_config_explain", title: "Explain one setting",
		description: "Everything about one ship-it setting: what it does, its type, allowed values and default, its SHIP_* variable, the value every layer gives it and why the effective one wins. Use it to understand why ship-it behaves as it does. Read-only.",
		input:       object(map[string]any{"key": settingKeyProperty}),
		output:      app.SettingExplanation{}, readOnly: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			key, err := settingKey(raw)
			if err != nil {
				return nil, err
			}
			return app.ExplainSetting(ctx, s.Workspace, key)
		},
	},
}

var settingKeyProperty = map[string]any{"type": "string", "minLength": 1,
	"description": `Setting key in dotted form, such as "ship.pullRequest.maxLines" (docs/reference/settings.md lists them).`}

// settingKey reads the required "key" argument of the config tools.
func settingKey(raw json.RawMessage) (string, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := decode(raw, &args); err != nil {
		return "", err
	}
	if args.Key == "" {
		return "", toolInputError{errors.New(`"key" is required`)}
	}
	return args.Key, nil
}

func deliver(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
	var args struct {
		Title  string   `json:"title"`
		Branch string   `json:"branch"`
		Only   []string `json:"only"`
		Issue  int      `json:"issue"`
		Why    string   `json:"why"`
		Tested string   `json:"tested"`
		DryRun bool     `json:"dryRun"`
	}
	if err := decode(raw, &args); err != nil {
		return nil, err
	}
	ws := s.workspace(args.DryRun)
	plan, err := app.ProposePlan(ctx, ws, app.PlanRequest{Only: args.Only, Issue: args.Issue})
	if err != nil {
		return nil, err
	}
	if len(plan.Secrets) > 0 {
		return nil, &app.SecretsFoundError{Findings: plan.Secrets}
	}
	// The delivery follows the plan at once: it reuses what the plan read.
	req := app.DeliveryRequest{Title: plan.Title, Branch: plan.Branch, Only: args.Only, Why: args.Why, Tested: args.Tested, Reads: plan.Reads()}
	// In attended mode a human typed the edits (the extension's review): no
	// AI-assisted mention.
	if (args.Title != "" || args.Branch != "") && !s.Attended {
		req.AIAssistant = hostAgent
	}
	if args.Title != "" {
		req.Title = args.Title
		if args.Branch == "" {
			// A branch that matches the agent's title, not the planner's; an
			// invalid title is reported by the delivery itself.
			conventions, _ := app.LoadConventions(ctx, ws, args.Only)
			if name, err := conventions.BranchForTitleOf(args.Title, app.BranchRequest{Issue: args.Issue}); err == nil {
				req.Branch = name
			}
		}
	}
	if args.Branch != "" {
		req.Branch = args.Branch
	}
	return app.DeliverLocally(ctx, ws, req)
}

// decode reads tool arguments strictly: an unknown argument is a mistake the
// model should see.
func decode(raw json.RawMessage, into any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return toolInputError{err}
	}
	return nil
}

// toolInputError is a bad argument: a tool execution error the model can fix.
type toolInputError struct{ err error }

func (e toolInputError) Error() string { return "invalid arguments: " + e.err.Error() }

// ToolResultTypes maps each tool to a zero value of its result type, for
// tools/gen to generate the TypeScript types of the VS Code extension.
func ToolResultTypes() map[string]any {
	types := make(map[string]any, len(tools))
	for _, t := range tools {
		types[t.name] = t.output
	}
	return types
}

func toolDefinitions() []map[string]any {
	defs := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, map[string]any{
			"name": t.name, "title": t.title, "description": t.description,
			"inputSchema":  t.input,
			"outputSchema": schema.For(t.output),
			"annotations": map[string]any{
				"title": t.title, "readOnlyHint": t.readOnly, "destructiveHint": t.destructive,
				"idempotentHint": t.idempotent || t.readOnly, "openWorldHint": t.openWorld,
			},
		})
	}
	return defs
}

// callTool runs a tool. Errors of ship-it (faults: a secret, a red gate…) are
// tool execution errors the model can act on (isError); an unknown tool is a
// protocol error.
func (s *Server) callTool(ctx context.Context, name string, args json.RawMessage) (map[string]any, error) {
	for _, t := range tools {
		if t.name != name {
			continue
		}
		result, err := t.run(ctx, s, args)
		if err != nil {
			return map[string]any{"content": []map[string]any{{"type": "text", "text": s.explain(err)}}, "isError": true}, nil
		}
		data, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		// structuredContent, and the same JSON as text for older clients.
		return map[string]any{
			"content":           []map[string]any{{"type": "text", "text": string(data)}},
			"structuredContent": json.RawMessage(data),
			"isError":           false,
		}, nil
	}
	return nil, &rpcError{Code: codeInvalidParams, Message: "Unknown tool: " + name}
}

// explain renders an error for the model: code, message and details.
func (s *Server) explain(err error) string {
	err = app.Known(err)
	var f *app.Fault
	if !errors.As(err, &f) {
		return err.Error()
	}
	text := fmt.Sprintf("%s %s", f.Code, i18n.Render(i18n.English, f.Key, f.Params))
	if findings, ok := app.SecretsIn(err); ok {
		for _, x := range findings {
			text += fmt.Sprintf("\n- %s:%d %s %s (fingerprint %s)", x.Path, x.Line, x.Rule, x.Preview, x.Fingerprint)
		}
	}
	if results, ok := app.GatesIn(err); ok {
		text += "\n" + results[len(results)-1].Output
	}
	return text
}
