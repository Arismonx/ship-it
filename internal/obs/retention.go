// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Retention bounds a log directory.
type Retention struct {
	// Runs is how many runs are kept, newest first.
	Runs int
	// Bytes is the total size kept.
	Bytes int64
}

// DefaultRetention keeps the last 20 runs and at most 10 MiB per directory
// (docs/reference/logs.md §5); ship.logs.keepRuns and ship.logs.maxBytes
// change it.
var DefaultRetention = Retention{Runs: 20, Bytes: 10 << 20}

// RunFiles describes the files of one run in a log directory.
type RunFiles struct {
	ID       RunID     `json:"run_id"`
	Time     time.Time `json:"time"`
	Files    []string  `json:"files"`
	Bytes    int64     `json:"bytes"`
	BlackBox bool      `json:"black_box"`
}

// PruneReport tells what Prune removed (or would remove on a dry run).
type PruneReport struct {
	Removed    []RunID `json:"removed"`
	FreedBytes int64   `json:"freed_bytes"`
	Kept       int     `json:"kept"`
}

// ListRuns returns the runs of a log directory, newest first. Files whose
// name does not start with a run id are not ship-it's and are ignored.
func ListRuns(dir string) ([]RunFiles, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byID := map[RunID]*RunFiles{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !(strings.HasSuffix(name, JournalSuffix) || strings.HasSuffix(name, ReportSuffix)) {
			continue
		}
		id, _, _ := strings.Cut(name, ".")
		at, ok := RunIDTime(RunID(id))
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		run := byID[RunID(id)]
		if run == nil {
			run = &RunFiles{ID: RunID(id), Time: at}
			byID[RunID(id)] = run
		}
		run.Files = append(run.Files, name)
		run.Bytes += info.Size()
		run.BlackBox = run.BlackBox || strings.HasSuffix(name, BlackBoxSuffix)
	}
	runs := make([]RunFiles, 0, len(byID))
	for _, r := range byID {
		sort.Strings(r.Files)
		runs = append(runs, *r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID > runs[j].ID })
	return runs, nil
}

// Prune removes the oldest runs of dir beyond the retention limits. The run
// keep (the current one) is never removed and counts towards the limits.
// Once a limit is reached, every older run goes too.
func Prune(dir string, keep RunID, r Retention, dryRun bool) (PruneReport, error) {
	runs, err := ListRuns(dir)
	if err != nil {
		return PruneReport{}, err
	}
	report := PruneReport{Removed: []RunID{}}
	var keptRuns int
	var keptBytes int64
	for _, run := range runs {
		if run.ID == keep {
			keptRuns++
			keptBytes += run.Bytes
		}
	}
	full := false
	var errs []error
	for _, run := range runs {
		if run.ID == keep {
			continue
		}
		if !full && keptRuns+1 <= r.Runs && keptBytes+run.Bytes <= r.Bytes {
			keptRuns++
			keptBytes += run.Bytes
			continue
		}
		full = true
		report.Removed = append(report.Removed, run.ID)
		report.FreedBytes += run.Bytes
		if dryRun {
			continue
		}
		for _, name := range run.Files {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	report.Kept = keptRuns
	return report, errors.Join(errs...)
}

// RunIDTime returns the creation time encoded in a run id, and whether id is
// a well-formed run id.
func RunIDTime(id RunID) (time.Time, bool) {
	if len(id) != 26 || id[0] > '7' {
		return time.Time{}, false
	}
	var ms uint64
	for i := 0; i < 10; i++ { // 10 characters × 5 bits = 50 bits ⊇ 48-bit time
		v := strings.IndexByte(crockford, id[i])
		if v < 0 {
			return time.Time{}, false
		}
		ms = ms<<5 | uint64(v)
	}
	for i := 10; i < 26; i++ {
		if strings.IndexByte(crockford, id[i]) < 0 {
			return time.Time{}, false
		}
	}
	return time.UnixMilli(int64(ms)).UTC(), true
}
