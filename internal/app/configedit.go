// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
)

// settingsLayers are the layers and the target LoadSettings resolves, so
// that get and explain show presets, contexts, SHIP_* and --set with their
// sources.
func settingsLayers(ctx context.Context, ws Workspace) ([]config.Layer, config.Target, error) {
	layers, err := configLayers(ctx, ws)
	return layers, settingsTarget(ctx, ws, layers), err
}

// SettingExplanation is what `ship config explain` shows.
type SettingExplanation = config.Explanation

// ExplainSetting returns everything about one key: its documentation, type
// and default, the value every layer gives it, and the layer that wins.
func ExplainSetting(ctx context.Context, ws Workspace, key string) (SettingExplanation, error) {
	if !config.IsKey(key) {
		return config.Explanation{}, config.UnknownSetting(key)
	}
	layers, target, err := settingsLayers(ctx, ws)
	if err != nil {
		return config.Explanation{}, err
	}
	return config.Explain(layers, target, key)
}

// SettingValue is one effective setting, as `ship config get` shows it.
type SettingValue struct {
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
	Source string          `json:"source"`
	// Path is the file of the winning layer; empty for a default, an
	// environment variable or --set.
	Path string `json:"path,omitempty"`
}

// GetSetting returns the effective value of key and where it comes from.
func GetSetting(ctx context.Context, ws Workspace, key string) (SettingValue, error) {
	x, err := ExplainSetting(ctx, ws, key)
	if err != nil {
		return SettingValue{}, err
	}
	return SettingValue{Key: key, Value: json.RawMessage(x.Value), Source: x.Source, Path: x.SourcePath}, nil
}

// ConfigEditRequest is a `ship config set` or `ship config unset`.
type ConfigEditRequest struct {
	Key string
	// Value is read by the key's type, as --set reads it; unset ignores it.
	Value string
	// Scope is the file to edit. Empty: the repository file inside a
	// repository, the user file outside.
	Scope ports.ConfigScope
}

// ConfigEditResult reports an edit.
type ConfigEditResult struct {
	Key string `json:"key"`
	// Value is the JSON value set; empty for an unset.
	Value json.RawMessage `json:"value,omitempty"`
	Scope string          `json:"scope"`
	Path  string          `json:"path"`
	// Changed is false when the file already said so (nothing written).
	Changed bool `json:"changed"`
	// Created: the file did not exist.
	Created bool `json:"created,omitempty"`
	// Operation is the entry `ship undo` reverts. Empty outside a repository,
	// where there is no operation log: the edit cannot be undone.
	Operation string `json:"operation,omitempty"`
	// Rehearsal: on a dry run, Changed says the file would change.
	Rehearsal
}

// SetSetting writes key into the configuration file of the request's scope,
// editing the file as text so that its comments, key order, indentation and
// line endings stay. The edited file is validated as Merge reads it before
// anything is written, a key the organisation policy imposes is refused, and
// the edit is recorded for `ship undo`.
func SetSetting(ctx context.Context, ws Workspace, req ConfigEditRequest) (ConfigEditResult, error) {
	done := ws.begin(ctx, "config_set")
	mark := ws.planMark()
	result, err := editSetting(ctx, ws, req, false)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, append(operation(result.Operation), slog.String("key", req.Key), slog.String("scope", result.Scope))...)
	return result, err
}

// UnsetSetting removes key, and the objects left empty, from the file of the
// request's scope, with the same care as SetSetting. A key the file does not
// hold changes nothing.
func UnsetSetting(ctx context.Context, ws Workspace, req ConfigEditRequest) (ConfigEditResult, error) {
	done := ws.begin(ctx, "config_unset")
	mark := ws.planMark()
	result, err := editSetting(ctx, ws, req, true)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, append(operation(result.Operation), slog.String("key", req.Key), slog.String("scope", result.Scope))...)
	return result, err
}

func editSetting(ctx context.Context, ws Workspace, req ConfigEditRequest, unset bool) (ConfigEditResult, error) {
	result := ConfigEditResult{Key: req.Key}
	if ws.Config == nil {
		return result, errors.New("app: no configuration files")
	}
	if !config.IsKey(req.Key) {
		return result, config.UnknownSetting(req.Key)
	}
	scope, inRepository, err := configScope(ctx, ws, req.Key, req.Scope)
	if err != nil {
		return result, err
	}
	result.Scope = string(scope)
	file, err := ws.Config.ReadConfigFile(ctx, scope)
	if err != nil {
		return result, err
	}
	result.Path = file.Path
	layer := config.Layer{Name: string(scope), Kind: config.KindUser, Path: file.Path, Data: file.Data}
	if scope == ports.ScopeRepository {
		layer.Kind = config.KindRepository
	}
	path := strings.Split(req.Key, ".")

	var edited []byte
	if unset {
		if !file.Exists {
			return result, nil
		}
		edited, err = config.Unset(file.Data, path)
	} else {
		var value string
		if value, err = config.AssignmentValue(req.Key, req.Value); err != nil {
			return result, err
		}
		result.Value = json.RawMessage(value)
		if err := refuseImposed(ctx, ws, req.Key, scope, value); err != nil {
			return result, err
		}
		base := file.Data
		if !file.Exists {
			// What `ship init` writes: $schema and version.
			base, result.Created = config.Render(config.Defaults()), true
		}
		edited, err = config.SetValue(base, path, value)
	}
	if err != nil {
		// The file is no JSONC object: Merge says where.
		if _, mergeErr := config.Merge([]config.Layer{layer}); mergeErr != nil {
			return result, mergeErr
		}
		return result, err
	}
	// The file alone, as Merge reads it: an unknown key, a wrong type or a
	// value outside the allowed ones is refused before anything is written.
	layer.Data = edited
	if _, err := config.Merge([]config.Layer{layer}); err != nil {
		return result, err
	}
	if file.Exists && bytes.Equal(edited, file.Data) {
		return result, nil
	}
	_, err = ws.Config.ReplaceConfigFile(ctx, ports.ConfigFileChange{
		Scope: scope, Old: file.Data, OldExists: file.Exists, New: edited, NewExists: true,
	})
	if errors.Is(err, ports.ErrConfigChanged) {
		return result, fault.New(fault.ConfigFileChanged, "config.changed_meanwhile", "file", file.Path)
	}
	if err != nil {
		return result, err
	}
	result.Changed = true
	if !inRepository || ws.Operations == nil {
		return result, nil
	}
	op, err := ws.Operations.Append(ctx, oplog.Operation{
		Kind: oplog.Configure, Title: req.Key,
		ConfigFile: &oplog.ConfigFileEdit{
			Scope: string(scope), Path: file.Path, Key: req.Key,
			Before: string(file.Data), BeforeExisted: file.Exists, After: string(edited),
		},
	})
	result.Operation = op.ID
	return result, err
}

// configScope resolves the requested scope: by default the repository file
// inside a repository (settings the team shares, reviewed in a pull
// request), the user file outside one, and always for a personal setting
// (ui.*), which only the user file holds.
func configScope(ctx context.Context, ws Workspace, key string, requested ports.ConfigScope) (ports.ConfigScope, bool, error) {
	_, err := ws.Config.ReadConfigFile(ctx, ports.ScopeRepository)
	inRepository := err == nil
	if err != nil && !errors.Is(err, ports.ErrNoConfigScope) {
		return "", false, err
	}
	switch requested {
	case "":
		if inRepository && !config.IsPersonalKey(key) {
			return ports.ScopeRepository, true, nil
		}
		return ports.ScopeUser, false, nil
	case ports.ScopeRepository:
		if !inRepository {
			return "", false, fault.New(fault.NotARepository, "config.no_repository")
		}
	}
	return requested, inRepository, nil
}

// refuseImposed refuses to set a key the organisation policy enforces: the
// value would either have no effect or conflict with it.
func refuseImposed(ctx context.Context, ws Workspace, key string, scope ports.ConfigScope, value string) error {
	layers, err := ws.Config.Layers(ctx)
	if err != nil {
		return err
	}
	for _, layer := range layers {
		if layer.Name != config.PolicyLayerName && layer.Kind != config.KindPolicy {
			continue
		}
		eff, err := config.Merge([]config.Layer{layer})
		if err != nil {
			return err
		}
		if p := eff.Policy; p != nil {
			for _, locked := range p.Locked {
				if locked == key {
					imposed := ""
					for _, e := range config.Flatten(eff.Settings) {
						if e.Key == key {
							imposed = e.Value
						}
					}
					return fault.New(fault.PolicyConflict, "config.policy_locked",
						"key", key, "value", value, "imposed", imposed, "source", string(scope),
						"organization", p.Organization, "contact", p.Contact)
				}
			}
		}
	}
	return nil
}
