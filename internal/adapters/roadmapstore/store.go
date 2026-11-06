// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package roadmapstore implements ports.RoadmapMemory in the repository's
// git directory: .git/ship/roadmap.json, local to the clone.
package roadmapstore

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

// GitDirLocator returns the repository's git directory.
type GitDirLocator interface {
	GitDir(ctx context.Context) (string, error)
}

// Store implements ports.RoadmapMemory.
type Store struct {
	Git GitDirLocator
}

var _ ports.RoadmapMemory = (*Store)(nil)

// New returns a Store for the repository of git.
func New(git GitDirLocator) *Store { return &Store{Git: git} }

// file is the format on disk: issue number → closed at the last sync.
type file struct {
	Version int             `json:"version"`
	Closed  map[string]bool `json:"closed"`
}

func (s *Store) path(ctx context.Context) (string, error) {
	dir, err := s.Git.GitDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ship", "roadmap.json"), nil
}

// Load implements ports.RoadmapMemory.
func (s *Store) Load(ctx context.Context) (roadmap.Memory, error) {
	path, err := s.path(ctx)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return roadmap.Memory{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return roadmap.Memory{}, nil // unreadable: as if first sync (GitHub wins, nothing reopens)
	}
	m := roadmap.Memory{}
	for k, closed := range f.Closed {
		if n, err := strconv.Atoi(k); err == nil && n > 0 {
			m[n] = closed
		}
	}
	return m, nil
}

// Save implements ports.RoadmapMemory, atomically (temporary file, rename).
func (s *Store) Save(ctx context.Context, m roadmap.Memory) error {
	path, err := s.path(ctx)
	if err != nil {
		return err
	}
	f := file{Version: 1, Closed: map[string]bool{}}
	for n, closed := range m {
		f.Closed[strconv.Itoa(n)] = closed
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".roadmap-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
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
