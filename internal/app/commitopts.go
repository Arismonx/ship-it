// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
)

// contributingGuides are where GitHub looks for a contributing guide, in its
// order (GitHub Docs, "Setting guidelines for repository contributors").
var contributingGuides = []string{".github/CONTRIBUTING.md", "CONTRIBUTING.md", "docs/CONTRIBUTING.md", "CONTRIBUTING"}

// dcoAppConfig is the configuration file of the DCO GitHub App
// (github.com/dcoapp/app): its presence says the repository checks sign-offs.
const dcoAppConfig = ".github/dco.yml"

// guideReadLimit bounds a contributing guide; real ones are a few KB.
const guideReadLimit = 256 << 10

// commitOptions are how ship commits, rebases and merges for the settings:
// the sign-off (ship.commits.signoff) and the signature (ship.commits.sign).
// A signature asked for without a key is refused here, before anything
// changes.
func commitOptions(ctx context.Context, ws Workspace, eff config.Effective) (ports.CommitOptions, error) {
	c := eff.Settings.Ship.Commits
	var opts ports.CommitOptions
	switch c.Signoff {
	case config.ModeAlways:
		opts.Signoff = true
	case config.ModeNever:
	default:
		opts.Signoff = repositoryWantsSignoff(ctx, ws)
	}
	switch c.Sign {
	case config.ModeAlways:
		if !hasSigningKey(ctx, ws.Git) {
			return opts, fault.New(fault.SigningKeyMissing, "convention.sign.no_key")
		}
		opts.Sign = ports.SignAlways
	case config.ModeNever:
		opts.Sign = ports.SignNever
	}
	return opts, nil
}

// repositoryWantsSignoff reads what tells whether the repository asks for
// sign-offs (convention.WantsSignoff): its last commits, its contributing
// guide, the DCO app's configuration. The files are read from the working
// tree, so a guide being written counts.
func repositoryWantsSignoff(ctx context.Context, ws Workspace) bool {
	_, dcoErr := ws.Git.ReadWorkingFile(ctx, dcoAppConfig, guideReadLimit)
	if dcoErr == nil {
		return true
	}
	var guides []string
	for _, path := range contributingGuides {
		if content, err := ws.Git.ReadWorkingFile(ctx, path, guideReadLimit); err == nil {
			guides = append(guides, string(content))
		}
	}
	messages, _ := ws.Git.RecentMessages(ctx, habitDepth)
	return convention.WantsSignoff(messages, guides, false)
}

// hasSigningKey reports whether git can sign: a user.signingKey, or for SSH
// signatures a gpg.ssh.defaultKeyCommand that finds one.
func hasSigningKey(ctx context.Context, git ports.Git) bool {
	if key, _, _ := git.ConfigValue(ctx, "user.signingkey"); strings.TrimSpace(key) != "" {
		return true
	}
	format, _, _ := git.ConfigValue(ctx, "gpg.format")
	command, _, _ := git.ConfigValue(ctx, "gpg.ssh.defaultkeycommand")
	return strings.EqualFold(format, "ssh") && strings.TrimSpace(command) != ""
}
