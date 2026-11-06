// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package fsconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/ports"
)

// maxConfigSize bounds a configuration file: anything larger is a mistake.
const maxConfigSize = 1 << 20

// TopLevelLocator returns the root of the working tree.
type TopLevelLocator interface {
	TopLevel(ctx context.Context) (string, error)
}

// Files implements ports.ConfigFiles.
type Files struct {
	Git TopLevelLocator
	Env platform.Environment
}

var _ ports.ConfigFiles = (*Files)(nil)

// New returns Files for the current user and the repository of git.
func New(git TopLevelLocator) *Files {
	return &Files{Git: git, Env: platform.CurrentEnvironment()}
}

// Layers implements ports.ConfigFiles: user, repository, then the system
// policy. A missing file is simply absent; outside a repository there is no
// repository layer.
func (f *Files) Layers(ctx context.Context) ([]config.Layer, error) {
	var layers []config.Layer
	if dir, err := f.Env.ConfigDir(); err == nil {
		layer, err := read("user", config.KindUser, filepath.Join(dir, "config.json"))
		if err != nil {
			return nil, err
		}
		if layer != nil {
			layers = append(layers, *layer)
		}
	}
	if top, err := f.Git.TopLevel(ctx); err == nil {
		layer, err := read("repository", config.KindRepository, filepath.Join(top, filepath.FromSlash(ports.RepositoryConfigPath)))
		if err != nil {
			return nil, err
		}
		if layer != nil {
			layers = append(layers, *layer)
		}
	}
	if path, err := f.Env.PolicyFile(); err == nil {
		layer, err := read(config.PolicyLayerName, config.KindPolicy, path)
		if err != nil {
			return nil, err
		}
		if layer != nil {
			layers = append(layers, *layer)
		}
	}
	return layers, nil
}

func read(name string, kind config.LayerKind, path string) (*config.Layer, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Size() > maxConfigSize {
		return nil, errors.New("fsconfig: " + path + " is larger than 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &config.Layer{Name: name, Kind: kind, Path: path, Data: data}, nil
}

// CreateRepositoryFile implements ports.ConfigFiles. O_EXCL makes the
// "never overwrite" promise atomic.
func (f *Files) CreateRepositoryFile(ctx context.Context, data []byte) (string, error) {
	top, err := f.Git.TopLevel(ctx)
	if err != nil {
		return "", err
	}
	path := filepath.Join(top, filepath.FromSlash(ports.RepositoryConfigPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return path, ports.ErrConfigExists
	}
	if err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return path, err
	}
	return path, file.Close()
}

// ReadPreset implements ports.ConfigFiles. The root of a preset is that of
// the file extending it: the user configuration directory for the user file
// and its presets, the repository root otherwise. The resolved file, links
// followed, must stay inside that root.
func (f *Files) ReadPreset(ctx context.Context, base config.Layer, path string) (config.Layer, error) {
	root, err := f.presetRoot(ctx, base)
	if err != nil {
		return config.Layer{}, err
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := inside(root, full); err != nil {
		return config.Layer{}, err
	}
	layer, err := read(path, config.KindPreset, full)
	if err != nil {
		return config.Layer{}, err
	}
	if layer == nil {
		return config.Layer{}, fmt.Errorf("%s: %w", full, fs.ErrNotExist)
	}
	return *layer, nil
}

func (f *Files) presetRoot(ctx context.Context, base config.Layer) (string, error) {
	if dir, err := f.Env.ConfigDir(); err == nil && inside(dir, base.Path) == nil {
		return dir, nil
	}
	return f.Git.TopLevel(ctx)
}

// inside fails unless path, symbolic links resolved, is under root.
func inside(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path // missing: reported by the read
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("fsconfig: %s is outside %s", path, root)
	}
	return nil
}

// presetCacheDir holds the GitHub presets, one file per immutable key.
func (f *Files) presetCacheDir() (string, error) {
	dir, err := f.Env.CacheDir()
	return filepath.Join(dir, "presets"), err
}

// cacheFile names a key's file: its SHA-256, so any key is a safe name.
func (f *Files) cacheFile(key string) (string, error) {
	dir, err := f.presetCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json"), nil
}

// CachedPreset implements ports.ConfigFiles.
func (f *Files) CachedPreset(_ context.Context, key string) ([]byte, bool) {
	path, err := f.cacheFile(key)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	return data, err == nil && len(data) <= maxConfigSize
}

// StorePreset implements ports.ConfigFiles, atomically (temporary file, then
// rename), so that a concurrent run never reads half a preset.
func (f *Files) StorePreset(_ context.Context, key string, data []byte) error {
	path, err := f.cacheFile(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".preset-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
