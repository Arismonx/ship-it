// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package footprint implements ports.Footprint on the file system: it finds
// what ship-it wrote (.git/ship, the user's ship-it directories, agent
// files) and removes it for ship uninstall (#55). It never removes a path it
// did not find itself, and never the organisation policy.
package footprint

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/GregoireF/ship-it/internal/core/mcpconfig"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/ports"
)

// Locator gives the repository's directories; gitexec.Client is one.
type Locator interface {
	TopLevel(ctx context.Context) (string, error)
	GitDir(ctx context.Context) (string, error)
}

// Files implements ports.Footprint.
type Files struct {
	git Locator
	env platform.Environment
}

// New returns Files for the repository of git and the user of env.
func New(git Locator, env platform.Environment) *Files { return &Files{git: git, env: env} }

// Repository implements ports.Footprint.
func (f *Files) Repository(ctx context.Context) ([]ports.Leftover, error) {
	gitDir, err := f.git.GitDir(ctx)
	if err != nil {
		return nil, err
	}
	l, ok, err := measure(ports.LeftoverRepository, filepath.Join(gitDir, "ship"))
	if err != nil || !ok {
		return nil, err
	}
	return []ports.Leftover{l}, nil
}

// User implements ports.Footprint. The cache and the logs may share one
// ship-it directory (%LocalAppData%\ship-it on Windows): it is listed once.
func (f *Files) User(_ context.Context) ([]ports.Leftover, error) {
	type place struct {
		holds string
		dir   func() (string, error)
	}
	var found []ports.Leftover
	for _, p := range []place{{"config", f.env.ConfigDir}, {"cache", f.env.CacheDir}, {"logs", f.env.LogDir}} {
		dir, err := p.dir()
		if err != nil {
			continue
		}
		root := appRoot(dir)
		if i := slices.IndexFunc(found, func(l ports.Leftover) bool { return l.Path == root }); i >= 0 {
			found[i].Holds = append(found[i].Holds, p.holds)
			continue
		}
		l, ok, err := measure(ports.LeftoverUser, root)
		if err != nil {
			return nil, err
		}
		if ok {
			l.Holds = []string{p.holds}
			found = append(found, l)
		}
	}
	return found, nil
}

// appRoot is the ship-it directory that holds dir: ~/.local/state/ship-it
// for ~/.local/state/ship-it/logs.
func appRoot(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if filepath.Base(d) == platform.AppName {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

// Remove implements ports.Footprint.
func (f *Files) Remove(ctx context.Context, l ports.Leftover) error {
	if l.Kind == ports.LeftoverAgentFile {
		return f.removeAgentFile(ctx, l.Path)
	}
	var known []ports.Leftover
	var err error
	switch l.Kind {
	case ports.LeftoverRepository:
		known, err = f.Repository(ctx)
	case ports.LeftoverUser:
		known, err = f.User(ctx)
	}
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(known, func(k ports.Leftover) bool { return k.Path == l.Path }) {
		return fmt.Errorf("footprint: %s is not ship-it's", l.Path)
	}
	if policy, err := f.env.PolicyFile(); err == nil && within(policy, l.Path) {
		return fmt.Errorf("footprint: %s holds the organisation policy", l.Path)
	}
	return os.RemoveAll(l.Path)
}

// removeAgentFile deletes one of the files ship mcp install writes.
func (f *Files) removeAgentFile(ctx context.Context, path string) error {
	if !slices.ContainsFunc(mcpconfig.Targets, func(t mcpconfig.Target) bool { return t.Path == path }) {
		return fmt.Errorf("footprint: %s is no agent file of ship mcp install", path)
	}
	top, err := f.git.TopLevel(ctx)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(top, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && (rel == "." || !startsWithParent(rel))
}

func startsWithParent(rel string) bool {
	return len(rel) >= 3 && rel[:2] == ".." && os.IsPathSeparator(rel[2])
}

// measure returns a leftover for dir with its size, and whether it exists.
func measure(kind, dir string) (ports.Leftover, bool, error) {
	l := ports.Leftover{Kind: kind, Path: dir}
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return l, false, nil
	}
	if err != nil {
		return l, false, err
	}
	if !info.IsDir() {
		return l, false, fmt.Errorf("footprint: %s is not a directory", dir)
	}
	err = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if info, err := d.Info(); err == nil {
			l.Bytes += info.Size()
		}
		l.Files++
		return nil
	})
	return l, true, err
}
