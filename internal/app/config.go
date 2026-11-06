// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/ports"
)

// LoadSettings resolves the settings of the repository, for no particular
// set of files: per-path overrides do not apply (see LoadSettingsFor).
func LoadSettings(ctx context.Context, ws Workspace) (config.Effective, error) {
	return LoadSettingsFor(ctx, ws, nil)
}

// LoadSettingsFor resolves the settings of a delivery of paths: every layer
// in precedence order (presets, user file, repository file, SHIP_* variables,
// --set, policy), with the user contexts that match the repository and the
// overrides that match every path. Without any file it returns the defaults.
func LoadSettingsFor(ctx context.Context, ws Workspace, paths []string) (config.Effective, error) {
	layers, err := configLayers(ctx, ws)
	if err != nil {
		return config.Effective{}, err
	}
	target := settingsTarget(ctx, ws, layers)
	target.Paths = paths
	return config.MergeFor(layers, target)
}

// gitRemote is ship.git.remote: the remote ship fetches from and pushes to.
// Settings that cannot be read give the default remote; the use case that
// needs them reports the problem itself.
func gitRemote(ctx context.Context, ws Workspace) string {
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return config.DefaultRemote
	}
	return remoteOf(eff)
}

// remoteOf is ship.git.remote of resolved settings.
func remoteOf(eff config.Effective) string {
	if r := eff.Settings.Ship.Git.Remote; r != "" {
		return r
	}
	return config.DefaultRemote
}

// configLayers lists every layer in precedence order, presets expanded right
// before the file that extends them. The policy stays where the port put it:
// Merge applies it last anyway.
func configLayers(ctx context.Context, ws Workspace) ([]config.Layer, error) {
	var files []config.Layer
	if ws.Config != nil {
		var err error
		if files, err = ws.Config.Layers(ctx); err != nil {
			return nil, err
		}
	}
	var layers []config.Layer
	for _, file := range files {
		presets, err := expandPresets(ctx, ws, file, nil)
		if err != nil {
			return nil, err
		}
		layers = append(append(layers, presets...), file)
	}
	env, err := config.EnvironmentLayers(ws.Environ)
	if err != nil {
		return nil, err
	}
	flags, err := config.FlagLayers(ws.SettingFlags)
	if err != nil {
		return nil, err
	}
	return append(append(layers, env...), flags...), nil
}

// maxPresetDepth bounds chains of presets: deeper is a design smell, and the
// bound also stops loops the visited set would miss across sources.
const maxPresetDepth = 4

// expandPresets returns the presets file extends, each preceded by its own,
// depth first, in the order of "extends".
func expandPresets(ctx context.Context, ws Workspace, file config.Layer, chain []string) ([]config.Layer, error) {
	var out []config.Layer
	for _, e := range config.ExtendsOf(file) {
		name := e.String()
		if len(chain) >= maxPresetDepth || slices.Contains(chain, name) {
			return nil, fault.New(fault.ConfigPresetCycle, "config.preset_cycle",
				"file", file.Path, "preset", name, "max", strconv.Itoa(maxPresetDepth))
		}
		preset, err := loadPreset(ctx, ws, file, e)
		if err != nil {
			return nil, fault.New(fault.ConfigPresetUnavailable, "config.preset_unavailable",
				"file", file.Path, "preset", name, "detail", err.Error())
		}
		nested, err := expandPresets(ctx, ws, preset, append(slices.Clone(chain), name))
		if err != nil {
			return nil, err
		}
		out = append(append(out, nested...), preset)
	}
	return out, nil
}

// loadPreset reads one preset. A GitHub preset is pinned to a commit, hence
// immutable: it is read from the cache, else fetched once and cached. A file
// preset named inside a GitHub preset is a file of that repository.
func loadPreset(ctx context.Context, ws Workspace, base config.Layer, e config.Extends) (config.Layer, error) {
	switch e.Kind {
	case config.BuiltinPreset:
		layer, _ := config.BuiltinPresetLayer(e.Name)
		return layer, nil
	case config.FilePreset:
		if owner, repository, commit, ok := gitHubOrigin(base); ok {
			return loadPreset(ctx, ws, base, config.Extends{Kind: config.GitHubPreset,
				Owner: owner, Repository: repository, Path: e.Path, Commit: commit})
		}
		if ws.Config == nil {
			return config.Layer{}, errors.New("no configuration files")
		}
		return ws.Config.ReadPreset(ctx, base, e.Path)
	}
	key := "github:" + e.Owner + "/" + e.Repository + "/" + e.Path + "@" + e.Commit
	layer := config.Layer{Name: e.String(), Path: key, Kind: config.KindPreset}
	if ws.Config != nil {
		if data, ok := ws.Config.CachedPreset(ctx, key); ok {
			layer.Data = data
			return layer, nil
		}
	}
	if ws.Forge == nil {
		return config.Layer{}, errors.New("GitHub is not reachable")
	}
	host := "github.com"
	// The settings are being read: the default remote names the server (the
	// settings cannot name their own remote here without a loop).
	if repo, err := repositoryOf(ctx, ws, config.DefaultRemote); err == nil {
		host = repo.Host // a GitHub Enterprise preset lives on the same server
	}
	data, err := ws.Forge.FileContent(ctx, forge.Repository{Host: host, Owner: e.Owner, Name: e.Repository}, e.Path, e.Commit)
	if err != nil {
		return config.Layer{}, err
	}
	if ws.Config != nil {
		_ = ws.Config.StorePreset(ctx, key, data) // a cache miss next time only costs a request
	}
	layer.Data = data
	return layer, nil
}

// gitHubOrigin tells whether a preset layer came from GitHub, and where.
func gitHubOrigin(layer config.Layer) (owner, repository, commit string, ok bool) {
	rest, found := strings.CutPrefix(layer.Path, "github:")
	if !found {
		return "", "", "", false
	}
	// The commit follows the last "@": a path may hold one ("@team/x.json").
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return "", "", "", false
	}
	rest, commit = rest[:at], rest[at+1:]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], commit, true
}

// settingsTarget describes the repository for contexts: owner and host of
// the remote (ship.git.remote) when it is on GitHub, root of the working
// tree, home directory. The remote is read from the layers resolved for the
// directory alone: a context that matches by owner cannot move the remote
// it is matched on.
func settingsTarget(ctx context.Context, ws Workspace, layers []config.Layer) config.Target {
	target := config.Target{Home: ws.Home, FoldCase: ws.OperatingSystem == "windows" || ws.OperatingSystem == "darwin"}
	if ws.Git == nil {
		return target
	}
	if top, err := ws.Git.TopLevel(ctx); err == nil {
		target.Dir = top
	}
	remote := config.DefaultRemote
	if eff, err := config.MergeFor(layers, target); err == nil {
		remote = remoteOf(eff)
	}
	if repo, err := repositoryOf(ctx, ws, remote); err == nil {
		target.Owner, target.Host = repo.Owner, repo.Host
	}
	return target
}

// SettingView is one effective setting and where it comes from.
type SettingView struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"` // "default", "user" or "repository"
}

// ConfigView is what `ship config` shows.
type ConfigView struct {
	Files    []string      `json:"files"`
	Settings []SettingView `json:"settings"`
	// Policy names the organisation imposing settings, if any.
	Policy *PolicyView `json:"policy,omitempty"`
}

// PolicyView is the organisation policy as surfaces show it.
type PolicyView struct {
	Organization string   `json:"organization"`
	Contact      string   `json:"contact,omitempty"`
	Path         string   `json:"path"`
	Locked       []string `json:"locked"`
}

// ShowConfig returns every effective setting with its source.
func ShowConfig(ctx context.Context, ws Workspace) (ConfigView, error) {
	view := ConfigView{Files: []string{}}
	if ws.Config != nil {
		layers, err := ws.Config.Layers(ctx)
		if err != nil {
			return view, err
		}
		for _, l := range layers {
			view.Files = append(view.Files, l.Path)
		}
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return view, err
	}
	for _, e := range config.Flatten(eff.Settings) {
		view.Settings = append(view.Settings, SettingView{Key: e.Key, Value: e.Value, Source: eff.Source(e.Key)})
	}
	if p := eff.Policy; p != nil {
		view.Policy = &PolicyView{Organization: p.Organization, Contact: p.Contact, Path: p.Path, Locked: p.Locked}
	}
	return view, nil
}

// PublishesAfterShip reports whether `ship` publishes after committing
// (ship.publish), so that a configuration error surfaces before any change.
func PublishesAfterShip(ctx context.Context, ws Workspace) (bool, error) {
	eff, err := LoadSettings(ctx, ws)
	return eff.Settings.Ship.Publish.Mode == config.PublishAuto, err
}

// initScopeMinUses is how often a scope must appear to become an area.
const initScopeMinUses = 3

// initHistoryDepth is how many commits `ship init` reads for scopes.
const initHistoryDepth = 500

// InitResult reports the file `ship init` wrote.
type InitResult struct {
	Path  string   `json:"path"`
	Areas []string `json:"areas"`
	Rehearsal
}

// Init writes .github/ship.json with the defaults and the areas found in the
// history (scopes used at least three times). It never overwrites a file and
// changes nothing on GitHub: `ship sync` does, after review.
func Init(ctx context.Context, ws Workspace) (result InitResult, err error) {
	mark := ws.planMark()
	defer func() { result.Rehearsal = ws.rehearsed(mark) }()
	subjects, err := ws.Git.RecentSubjects(ctx, initHistoryDepth)
	if err != nil {
		return InitResult{}, err
	}
	s := config.Defaults()
	// The types a user file or SHIP_* already adds count; a broken
	// configuration only loses them.
	var rules convention.Rules
	if eff, err := LoadSettings(ctx, ws); err == nil {
		rules = eff.Settings.Conventions()
	}
	if areas := rules.FrequentScopes(subjects, initScopeMinUses); len(areas) > 0 {
		s.GitHub.Labels.Areas = areas
	}
	path, err := ws.Config.CreateRepositoryFile(ctx, config.Render(s))
	if errors.Is(err, ports.ErrConfigExists) {
		return InitResult{}, fault.New(fault.ConfigAlreadyThere, "config.already_there", "file", path)
	}
	return InitResult{Path: path, Areas: s.GitHub.Labels.Areas}, err
}

// SyncPlan is what `ship sync` would change on GitHub.
type SyncPlan struct {
	Repository forge.Repository       `json:"repository"`
	Labels     []settings.LabelChange `json:"labels"`
	// ReadOnly: another tool manages the repository (github.managedBy); the
	// plan only reports differences.
	ReadOnly  bool   `json:"read_only"`
	ManagedBy string `json:"managed_by,omitempty"`
}

// Empty reports whether the repository already matches the settings.
func (p SyncPlan) Empty() bool { return len(p.Labels) == 0 }

// PlanSync compares the repository with the settings. It only reads.
func PlanSync(ctx context.Context, ws Workspace) (SyncPlan, error) {
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return SyncPlan{}, err
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return SyncPlan{}, err
	}
	existing, err := ws.Forge.Labels(ctx, repo)
	if err != nil {
		return SyncPlan{}, forgeFault(err, repo)
	}
	return SyncPlan{
		Repository: repo,
		Labels:     settings.PlanLabels(settings.WantedLabels(eff.Settings), existing),
		ReadOnly:   eff.Settings.ReadOnlySync(),
		ManagedBy:  eff.Settings.GitHub.ManagedBy,
	}, nil
}

// SyncResult reports what was applied.
type SyncResult struct {
	Operation string `json:"operation,omitempty"`
	Applied   int    `json:"applied"`
	Rehearsal
}

// ApplySync applies a plan the human reviewed, in order, stopping at the
// first failure. What was applied is recorded in the operation log.
func ApplySync(ctx context.Context, ws Workspace, plan SyncPlan) (result SyncResult, err error) {
	if plan.ReadOnly {
		return SyncResult{}, fault.New(fault.SyncReadOnly, "sync.read_only", "tool", plan.ManagedBy)
	}
	mark := ws.planMark()
	defer func() { result.Rehearsal = ws.rehearsed(mark) }()
	var applyErr error
	for _, change := range plan.Labels {
		switch change.Kind {
		case settings.CreateLabel:
			applyErr = ws.Forge.CreateLabel(ctx, plan.Repository, change.Label)
		default:
			applyErr = ws.Forge.UpdateLabel(ctx, plan.Repository, change.From, change.Label)
		}
		if applyErr != nil {
			applyErr = forgeFault(applyErr, plan.Repository)
			break
		}
		result.Applied++
	}
	if result.Applied > 0 {
		op, err := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.Sync, Remote: gitRemote(ctx, ws), LabelChanges: plan.Labels[:result.Applied],
		})
		result.Operation = op.ID
		applyErr = errors.Join(applyErr, err)
	}
	return result, applyErr
}

// remoteRepository returns the GitHub repository of the remote
// (ship.git.remote).
func remoteRepository(ctx context.Context, ws Workspace) (forge.Repository, error) {
	return repositoryOf(ctx, ws, gitRemote(ctx, ws))
}

// repositoryOf returns the GitHub repository a remote names.
func repositoryOf(ctx context.Context, ws Workspace, remote string) (forge.Repository, error) {
	url, err := ws.Git.RemoteURL(ctx, remote)
	if err != nil {
		return forge.Repository{}, fault.New(fault.NoGitHubRemote, "publish.no_remote", "remote", remote)
	}
	repo, ok := forge.ParseRemoteURL(url)
	if !ok {
		return forge.Repository{}, fault.New(fault.NoGitHubRemote, "publish.unsupported_remote", "url", url, "remote", remote)
	}
	return repo, nil
}

// RemoteProblem says why ship publish cannot work here: the remote
// (ship.git.remote) is missing or not a GitHub repository, SHIP-E0602; nil
// when it is on GitHub.
func RemoteProblem(ctx context.Context, ws Workspace) error {
	_, err := remoteRepository(ctx, ws)
	return err
}
