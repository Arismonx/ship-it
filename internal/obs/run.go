// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// File name suffixes of a run in a log directory.
const (
	JournalSuffix  = ".jsonl"
	BlackBoxSuffix = ".blackbox.jsonl"
)

// Log files hold technical details of the user's work: only they may read them.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Attach opens <dir>/<run_id>.jsonl and writes there every INFO record,
// including those logged before. Logging never stops the work: on error the
// caller should Discard and go on.
func (j *Journal) Attach(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, string(j.RunID)+JournalSuffix), os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return err
	}
	j.mu.Lock()
	j.dir, j.handle = dir, f
	j.mu.Unlock()
	return j.file.attach(f)
}

// Discard keeps the journal in memory only: nothing is written to disk.
func (j *Journal) Discard() { j.file.drop() }

// Dir returns the log directory, "" when the journal is not attached.
func (j *Journal) Dir() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.dir
}

// Elapsed returns the time since the journal was created.
func (j *Journal) Elapsed() time.Duration { return time.Since(j.started) }

// Close closes the journal file; later records stay in the black box only.
func (j *Journal) Close() error {
	j.mu.Lock()
	f := j.handle
	j.handle = nil
	j.mu.Unlock()
	if f == nil {
		return nil
	}
	j.file.drop()
	return f.Close()
}

// SaveBlackBox writes the black box next to the journal, as
// <dir>/<run_id>.blackbox.jsonl, and returns its path. It returns "" when
// the journal is not attached to a directory.
func (j *Journal) SaveBlackBox() (string, error) {
	dir := j.Dir()
	if dir == "" {
		return "", nil
	}
	path := filepath.Join(dir, string(j.RunID)+BlackBoxSuffix)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return "", err
	}
	if _, err := j.BlackBox.WriteTo(f); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}

// Prune applies a retention to the journal's directory, never removing the
// current run. It implements ports.RunLogs for `ship gc`.
func (j *Journal) Prune(_ context.Context, keepRuns int, maxBytes int64, dryRun bool) (removedRuns int, freedBytes int64, err error) {
	dir := j.Dir()
	if dir == "" {
		return 0, 0, nil
	}
	report, err := Prune(dir, j.RunID, Retention{Runs: keepRuns, Bytes: maxBytes}, dryRun)
	return len(report.Removed), report.FreedBytes, err
}

// SetLimits applies the configured retention (used after the run) and black
// box capacity (ship.logs). Non-positive values keep the current ones.
func (j *Journal) SetLimits(r Retention, blackBoxBytes int) {
	j.mu.Lock()
	if r.Runs > 0 {
		j.retention.Runs = r.Runs
	}
	if r.Bytes > 0 {
		j.retention.Bytes = r.Bytes
	}
	j.mu.Unlock()
	if blackBoxBytes > 0 {
		j.BlackBox.SetCapacity(blackBoxBytes)
	}
}

// Retention returns the retention applied after the run.
func (j *Journal) Retention() Retention {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.retention
}

// Events returns the records of this run kept by the black box.
func (j *Journal) Events() []Event {
	var buf bytes.Buffer
	_, _ = j.BlackBox.WriteTo(&buf)
	events, _ := ReadEvents(&buf)
	return events
}

// RepoHash identifies a repository in logs without naming it: the first 16
// hex digits of the HMAC-SHA256 of its top-level path, keyed by the
// installation's salt (InstallSalt). Without the salt, nobody can test likely
// paths against it. It returns "" when the salt or the path is missing.
func RepoHash(salt []byte, topLevel string) string {
	if len(salt) == 0 || topLevel == "" {
		return ""
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(filepath.Clean(topLevel)))
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil)[:8])
}

// ErrNoRun is returned when a run cannot be found in a log directory.
var ErrNoRun = errors.New("obs: no such run")
