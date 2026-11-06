// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"errors"

	"github.com/GregoireF/ship-it/internal/core/config"
)

// RepositoryConfigPath is the configuration file of a repository, relative to
// its root (ADR-0009, decision Q2).
const RepositoryConfigPath = ".github/ship.json"

// ConfigFiles reads and creates configuration files.
type ConfigFiles interface {
	// Layers returns the files that exist, weakest first: user, repository.
	Layers(ctx context.Context) ([]config.Layer, error)
	// CreateRepositoryFile writes .github/ship.json and returns its absolute
	// path. It never overwrites: ErrConfigExists when the file is there.
	CreateRepositoryFile(ctx context.Context, data []byte) (string, error)
	// ReadPreset reads a preset file named in the "extends" of base, relative
	// to base's root: the repository root for the repository file and its
	// presets, the user configuration directory for the user file. It never
	// reads outside that root.
	ReadPreset(ctx context.Context, base config.Layer, path string) (config.Layer, error)
	// CachedPreset returns a GitHub preset stored by StorePreset; key names
	// an immutable file (repository, path and commit).
	CachedPreset(ctx context.Context, key string) ([]byte, bool)
	StorePreset(ctx context.Context, key string, data []byte) error
	// ReadConfigFile returns the file of scope as it is on disk (Exists is
	// false when there is none yet). ErrNoConfigScope: the scope has no
	// location (the repository scope outside a repository).
	ReadConfigFile(ctx context.Context, scope ConfigScope) (ConfigFile, error)
	// ReplaceConfigFile makes the file of change.Scope hold change.New, or
	// removes it when change.NewExists is false, only if it still holds
	// change.Old (compare-and-swap; ErrConfigChanged otherwise). The new
	// content is written to a temporary file then renamed over the old one,
	// so that a reader never sees half a file. It returns the file's path.
	ReplaceConfigFile(ctx context.Context, change ConfigFileChange) (string, error)
}

// ConfigScope names a configuration file ship-it writes.
type ConfigScope string

// Writable scopes.
const (
	// ScopeUser is the user file (config.json in the user configuration
	// directory).
	ScopeUser ConfigScope = "user"
	// ScopeRepository is .github/ship.json of the current repository.
	ScopeRepository ConfigScope = "repository"
)

// ConfigFile is a configuration file as read.
type ConfigFile struct {
	Path   string
	Data   []byte
	Exists bool
}

// ConfigFileChange is a compare-and-swap of a whole configuration file.
type ConfigFileChange struct {
	Scope ConfigScope
	// Old is the content the file must still hold; OldExists false: the
	// file must not exist.
	Old       []byte
	OldExists bool
	// New is the content to write; NewExists false removes the file.
	New       []byte
	NewExists bool
}

// ErrConfigExists is returned by CreateRepositoryFile.
var ErrConfigExists = errors.New("ports: configuration file already exists")

// ErrNoConfigScope is returned for a scope that has no file here.
var ErrNoConfigScope = errors.New("ports: no configuration file for this scope here")

// ErrConfigChanged is returned by ReplaceConfigFile when the file no longer
// holds what the caller read.
var ErrConfigChanged = errors.New("ports: configuration file changed since it was read")
