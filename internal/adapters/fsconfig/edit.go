// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fsconfig

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/ports"
)

// UserConfigFile is the name of the user file in the configuration directory.
const UserConfigFile = "config.json"

// pathOf returns the file of a writable scope.
func (f *Files) pathOf(ctx context.Context, scope ports.ConfigScope) (string, error) {
	switch scope {
	case ports.ScopeUser:
		dir, err := f.Env.ConfigDir()
		if err != nil {
			return "", ports.ErrNoConfigScope
		}
		return filepath.Join(dir, UserConfigFile), nil
	case ports.ScopeRepository:
		top, err := f.Git.TopLevel(ctx)
		if err != nil {
			return "", ports.ErrNoConfigScope
		}
		return filepath.Join(top, filepath.FromSlash(ports.RepositoryConfigPath)), nil
	}
	return "", errors.New("fsconfig: unknown scope " + string(scope))
}

// ReadConfigFile implements ports.ConfigFiles.
func (f *Files) ReadConfigFile(ctx context.Context, scope ports.ConfigScope) (ports.ConfigFile, error) {
	path, err := f.pathOf(ctx, scope)
	if err != nil {
		return ports.ConfigFile{}, err
	}
	layer, err := read(string(scope), config.LayerKind(scope), path)
	if err != nil || layer == nil {
		return ports.ConfigFile{Path: path}, err
	}
	return ports.ConfigFile{Path: path, Data: layer.Data, Exists: true}, nil
}

// ReplaceConfigFile implements ports.ConfigFiles. The comparison and the
// rename are not one atomic step: two ship processes editing the same file in
// the same millisecond could both pass the check. The rename still leaves one
// whole file, never a mix.
func (f *Files) ReplaceConfigFile(ctx context.Context, change ports.ConfigFileChange) (string, error) {
	path, err := f.pathOf(ctx, change.Scope)
	if err != nil {
		return "", err
	}
	current, err := read(string(change.Scope), config.LayerKind(change.Scope), path)
	if err != nil {
		return path, err
	}
	if (current != nil) != change.OldExists || (current != nil && !bytes.Equal(current.Data, change.Old)) {
		return path, ports.ErrConfigChanged
	}
	if !change.NewExists {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return path, err
		}
		return path, nil
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	return path, writeAtomically(path, change.New, mode)
}

// writeAtomically writes a temporary file next to path, then renames it over
// path (os.Rename replaces the file on Windows too).
func writeAtomically(path string, data []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
