// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/ports"
)

// StatusArgs produces the output parsed by ParseStatus: porcelain v2 with
// branch headers, NUL separators and untracked files listed one by one.
var StatusArgs = []string{"status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all"}

// Number of space-separated fields before the path in porcelain v2 entries.
const (
	ordinaryFields = 8  // 1 XY sub mH mI mW hH hI
	renameFields   = 9  // 2 XY sub mH mI mW hH hI Xscore
	unmergedFields = 10 // u XY sub m1 m2 m3 mW h1 h2 h3
)

// ParseStatus parses `git status --porcelain=v2 --branch -z` output, as
// documented in git-status(1) "Porcelain Format Version 2".
func ParseStatus(out []byte) (ports.WorkingTreeStatus, error) {
	var status ports.WorkingTreeStatus
	records := bytes.Split(out, []byte{0})
	for i := 0; i < len(records); i++ {
		record := string(records[i])
		if record == "" {
			continue
		}
		switch record[0] {
		case '#':
			parseBranchHeader(record, &status.Branch)
		case '1':
			change, err := parseTracked(record, ordinaryFields)
			if err != nil {
				return status, err
			}
			status.Files = append(status.Files, change)
		case '2':
			change, err := parseTracked(record, renameFields)
			if err != nil {
				return status, err
			}
			// With -z, the original path is the next NUL-separated record.
			if i+1 >= len(records) {
				return status, fmt.Errorf("gitexec: rename entry without original path: %q", record)
			}
			i++
			change.OriginalPath = string(records[i])
			status.Files = append(status.Files, change)
		case 'u':
			change, err := parseTracked(record, unmergedFields)
			if err != nil {
				return status, err
			}
			change.State = ports.Conflicted
			status.Files = append(status.Files, change)
		case '?', '!':
			// "? <path>" and "! <path>": a marker, a space, a non-empty path.
			if len(record) < 3 || record[1] != ' ' {
				return status, fmt.Errorf("gitexec: malformed status record %q", record)
			}
			state := ports.Untracked
			if record[0] == '!' {
				state = ports.Ignored
			}
			status.Files = append(status.Files, ports.FileChange{Path: record[2:], State: state,
				Staged: ports.Unchanged, Unstaged: ports.Unchanged})
		default:
			return status, fmt.Errorf("gitexec: unknown status record %q", record)
		}
	}
	return status, nil
}

// parseTracked splits the fixed fields; the path is everything after them,
// so paths containing spaces stay intact.
func parseTracked(record string, fields int) (ports.FileChange, error) {
	parts := strings.SplitN(record, " ", fields+1)
	if len(parts) != fields+1 || len(parts[1]) != 2 {
		return ports.FileChange{}, fmt.Errorf("gitexec: malformed status record %q", record)
	}
	return ports.FileChange{
		Path:      parts[fields],
		Staged:    ports.ChangeKind(parts[1][0]),
		Unstaged:  ports.ChangeKind(parts[1][1]),
		State:     ports.Tracked,
		Submodule: strings.HasPrefix(parts[2], "S"),
	}, nil
}

func parseBranchHeader(record string, branch *ports.BranchState) {
	key, value, _ := strings.Cut(strings.TrimPrefix(record, "# "), " ")
	switch key {
	case "branch.oid":
		if value != "(initial)" {
			branch.Commit = value
		}
	case "branch.head":
		if value == "(detached)" {
			branch.Detached = true
		} else {
			branch.Name = value
		}
	case "branch.upstream":
		branch.Upstream = value
	case "branch.ab":
		ahead, behind, _ := strings.Cut(value, " ")
		branch.Ahead, _ = strconv.Atoi(strings.TrimPrefix(ahead, "+"))
		branch.Behind, _ = strconv.Atoi(strings.TrimPrefix(behind, "-"))
	}
	// Unknown headers are ignored, as git-status(1) requires.
}
