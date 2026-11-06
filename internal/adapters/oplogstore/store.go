// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package oplogstore keeps the operation log as one JSON file per operation in
// .git/ship/oplog. File names are time-ordered ids, so listing the directory
// is enough to read the log newest first. Each file is written to a temporary
// name, synced and renamed, so a crash never leaves a half-written entry.
package oplogstore

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/obs"
	"github.com/GregoireF/ship-it/internal/ports"
)

// directory is the log location, relative to the git directory.
const directory = "ship/oplog"

// GitDirLocator returns the absolute path of the git directory.
type GitDirLocator interface {
	GitDir(ctx context.Context) (string, error)
}

// Store is a ports.OperationLog on the local file system.
type Store struct {
	Git GitDirLocator
	// Now and NewID default to the clock and obs.NewRunID.
	Now   func() time.Time
	NewID func() string
}

var _ ports.OperationLog = (*Store)(nil)

// New returns a Store for the repository git belongs to.
func New(git GitDirLocator) *Store { return &Store{Git: git} }

func (s *Store) dir(ctx context.Context) (string, error) {
	gitDir, err := s.Git.GitDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, filepath.FromSlash(directory)), nil
}

// Append implements ports.OperationLog.
func (s *Store) Append(ctx context.Context, op oplog.Operation) (oplog.Operation, error) {
	if op.ID == "" {
		op.ID = s.newID()
	}
	if op.StartedAt.IsZero() {
		op.StartedAt = s.now()
	}
	dir, err := s.dir(ctx)
	if err != nil {
		return op, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return op, err
	}
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return op, err
	}
	return op, writeAtomically(filepath.Join(dir, op.ID+".json"), append(data, '\n'))
}

func writeAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
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
	return os.Rename(tmp.Name(), path)
}

// Recent implements ports.OperationLog. Entries that cannot be decoded are
// skipped: a damaged entry must not block undoing the others.
func (s *Store) Recent(ctx context.Context, limit int) ([]oplog.Operation, error) {
	dir, err := s.dir(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	slices.Reverse(names)

	var ops []oplog.Operation
	for _, name := range names {
		if len(ops) == limit {
			break
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var op oplog.Operation
		if json.Unmarshal(data, &op) != nil || op.ID+".json" != name {
			continue
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return string(obs.NewRunID())
}
