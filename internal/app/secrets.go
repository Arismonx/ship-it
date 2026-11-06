// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strconv"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/secrets"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/ports"
)

// SecretView is a detected secret as surfaces show it; the value itself is
// never part of it.
type SecretView struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
	Preview string `json:"preview,omitempty"`
	Length  int    `json:"length,omitempty"`
	// Fingerprint accepts the finding in ship.secrets.allow: the SHA-256 of
	// the rule id, a NUL byte and the value (the path for a whole file).
	Fingerprint string `json:"fingerprint"`
}

// SecretsFoundError refuses a delivery that would commit secrets. It wraps
// the fault SHIP-E0901 and carries every finding.
type SecretsFoundError struct {
	Findings []SecretView
	// Unpushed: the secrets are in commits a push would send (since Base),
	// not in the work in progress; the remedy is to clean the history.
	Unpushed bool
	Base     string
}

func (e *SecretsFoundError) Error() string { return e.Unwrap().Error() }

// Unwrap exposes the fault, so that errors.As(err, *Fault) renders it.
func (e *SecretsFoundError) Unwrap() error {
	if e.Unpushed {
		base := e.Base
		if base == "" {
			base = "--root"
		}
		return fault.New(fault.SecretsFound, "secrets.unpushed", "count", strconv.Itoa(len(e.Findings)), "base", base)
	}
	return fault.New(fault.SecretsFound, "secrets.found", "count", strconv.Itoa(len(e.Findings)))
}

// scanPushed scans every commit a push would send: reachable from HEAD and
// from none of already. Each commit's own patch is scanned, so a secret
// added then removed within the branch is still found — the push sends it.
func scanPushed(ctx context.Context, ws Workspace, already []string) ([]SecretView, error) {
	raw, err := ws.Git.CommitPatches(ctx, already)
	if err != nil {
		return nil, err
	}
	return scanSecrets(ctx, ws, snapshot.ParseUnifiedDiff(raw))
}

// SecretsIn returns the findings an error carries, if it is a refusal for
// secrets.
func SecretsIn(err error) ([]SecretView, bool) {
	var found *SecretsFoundError
	if errors.As(err, &found) {
		return found.Findings, true
	}
	return nil, false
}

// IsGateRefusal reports whether err is a gate that refused the work
// (secrets, a red lint or test): surfaces exit with code 3.
func IsGateRefusal(err error) bool {
	_, secrets := SecretsIn(err)
	_, red := GatesIn(err)
	_, commitlintRefused := CommitlintOutputIn(err)
	return secrets || red || commitlintRefused || errors.Is(err, &fault.Fault{Code: fault.GateSkipRefused}) ||
		errors.Is(err, &fault.Fault{Code: fault.CommitlintUnsatisfiable})
}

// scanSecrets checks the added lines, honouring ship.secrets.
func scanSecrets(ctx context.Context, ws Workspace, diffs []snapshot.FileDiff) ([]SecretView, error) {
	eff, err := LoadSettingsFor(ctx, ws, diffPaths(diffs))
	if err != nil {
		return nil, err
	}
	opts, err := scanOptions(eff.Settings.Ship.Secrets)
	if err != nil {
		return nil, err
	}
	if ws.Agent {
		opts.IgnorePaths, opts.Disabled, opts.Allow = committedExemptions(ctx, ws, diffPaths(diffs))
	}
	var views []SecretView
	for _, f := range secrets.Scan(diffs, opts) {
		views = append(views, SecretView{Path: f.Path, Line: f.Line, Rule: string(f.Rule), Preview: f.Preview, Length: f.Length,
			Fingerprint: f.Fingerprint})
	}
	return views, nil
}

// committedSettingsLimit bounds the repository settings file read from HEAD.
const committedSettingsLimit = 1 << 20

// committedExemptions are the scan exemptions an unattended AI agent gets:
// ignorePaths, disabledRules and allow as .github/ship.json holds them in
// HEAD, the team's committed and reviewed choices, its overrides that match
// the delivered paths included (decision of 2026-11-05). Nothing else counts:
// not the working tree (an agent could have changed it), the user file,
// presets, contexts, SHIP_* or --set. Without a committed file, or with one
// that does not read, the agent gets none.
func committedExemptions(ctx context.Context, ws Workspace, paths []string) (ignore []string, disabled []secrets.RuleID, allow []string) {
	if ws.Git == nil {
		return nil, nil, nil
	}
	data, err := ws.Git.ReadFileAt(ctx, "HEAD", ports.RepositoryConfigPath, committedSettingsLimit)
	if err != nil {
		ws.logger().Info("scan.agent_strict", "file", ports.RepositoryConfigPath, "reason", "not committed")
		return nil, nil, nil
	}
	layer := config.Layer{Name: "repository", Kind: config.KindRepository, Path: ports.RepositoryConfigPath + " (HEAD)", Data: data}
	all, _ := configLayers(ctx, ws)
	target := settingsTarget(ctx, ws, all)
	target.Paths = paths
	eff, err := config.MergeFor([]config.Layer{layer}, target)
	if err != nil {
		ws.logger().Info("scan.agent_strict", "file", ports.RepositoryConfigPath, "reason", "unreadable at HEAD")
		return nil, nil, nil
	}
	s := eff.Settings.Ship.Secrets
	for _, id := range s.DisabledRules {
		disabled = append(disabled, secrets.RuleID(id))
	}
	return s.IgnorePaths, disabled, s.Allow
}

// scanOptions turns ship.secrets into scan options. Extra rules apply from
// every layer, to an AI agent too: they only add checks. An agent's
// exemptions come from the committed file alone (committedExemptions).
func scanOptions(s config.SecretsSettings) (secrets.Options, error) {
	opts := secrets.Options{IgnorePaths: s.IgnorePaths, Allow: s.Allow}
	for _, id := range s.DisabledRules {
		opts.Disabled = append(opts.Disabled, secrets.RuleID(id))
	}
	for _, def := range s.Rules {
		r, err := secrets.NewRule(def.ID, def.Pattern, def.Keywords)
		if err != nil { // validated when the settings were read
			return opts, fault.New(fault.ConfigInvalidValue, "config.invalid_value",
				"file", "ship.secrets.rules", "key", "ship.secrets.rules", "value", def.ID, "allowed", err.Error())
		}
		opts.Rules = append(opts.Rules, r)
	}
	return opts, nil
}

// diffPaths are the paths of diffs, for the per-path configuration overrides.
func diffPaths(diffs []snapshot.FileDiff) []string {
	paths := make([]string, len(diffs))
	for i, d := range diffs {
		paths[i] = d.Path
	}
	return paths
}
