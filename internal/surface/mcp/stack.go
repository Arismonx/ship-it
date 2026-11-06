// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"context"
	"encoding/json"

	"github.com/GregoireF/ship-it/internal/app"
)

// StackResult is what ship_stack returns: the layers committed and, unless
// local, their pull requests.
type StackResult struct {
	Delivery    app.StackDeliveryResult `json:"delivery"`
	Publication *app.StackPublishResult `json:"publication,omitempty"`
}

// stackTools create a stack and show it. Merging one is a human's decision
// (ADR-0006, revision of 2026-11-02): no tool merges.
var stackTools = []tool{
	{
		name: "ship_stack", title: "Ship the work as a stack of draft pull requests",
		description: "Splits the work in progress into layers by area (or the given layers, bottom first), commits each layer on its own branch based on the previous one, then pushes them and opens one DRAFT pull request per layer, grouped in a GitHub stack (a classic chain where the server has none). Run it from the default branch. Refuses changes containing secrets. A human reviews and merges.",
		input: object(map[string]any{
			"layers": map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"description": "Layers bottom first, each a list of files or directories (relative to the repository root). Default: by area, at most ship.stack.maxLayers."},
			"local":  map[string]any{"type": "boolean", "description": "Commit the layers only: no push, no pull request."},
			"dryRun": dryRunProperty,
		}),
		output: StackResult{}, openWorld: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			var args struct {
				Layers [][]string `json:"layers"`
				Local  bool       `json:"local"`
				DryRun bool       `json:"dryRun"`
			}
			if err := decode(raw, &args); err != nil {
				return nil, err
			}
			ws := s.workspace(args.DryRun)
			view, err := app.ProposeStack(ctx, ws, app.StackPlanRequest{Layers: args.Layers})
			if err != nil {
				return nil, err
			}
			if len(view.Secrets) > 0 {
				return nil, &app.SecretsFoundError{Findings: view.Secrets}
			}
			var req app.StackDeliveryRequest
			for _, l := range view.Layers {
				req.Layers = append(req.Layers, app.StackLayerSpec{Title: l.Title, Branch: l.Branch, Files: l.Files})
			}
			delivery, err := app.DeliverStack(ctx, ws, req)
			if err != nil {
				return nil, err
			}
			result := StackResult{Delivery: delivery}
			if args.Local {
				return result, nil
			}
			// ADR-0010: every pull request opened by an agent is a draft.
			publication, err := app.PublishStack(ctx, ws, app.StackPublishRequest{Unattended: !s.Attended, ForceDraft: !s.Attended})
			if app.IsLocalRepository(err) {
				return result, nil
			}
			if err != nil {
				return nil, err
			}
			result.Publication = &publication
			return result, nil
		},
	},
	{
		name: "ship_stack_status", title: "Where the stack stands",
		description: "The layers of the stack the current branch belongs to, bottom first, with their pull requests and checks. Read-only.",
		input:       object(map[string]any{"offline": map[string]any{"type": "boolean", "description": "Do not ask GitHub."}}),
		output:      app.StackStatusView{}, readOnly: true, openWorld: true,
		run: func(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
			var args struct {
				Offline bool `json:"offline"`
			}
			if err := decode(raw, &args); err != nil {
				return nil, err
			}
			return app.StackStatus(ctx, s.Workspace, args.Offline)
		},
	},
}

func init() { tools = append(tools, stackTools...) }
