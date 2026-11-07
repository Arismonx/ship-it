// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"slices"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// everyKind is every kind of operation the log records.
var everyKind = []oplog.Kind{
	oplog.Deliver, oplog.Publish, oplog.Sync, oplog.Tidy, oplog.Fix, oplog.Rebase, oplog.Merge,
	oplog.Configure, oplog.Rename, oplog.Roadmap, oplog.Undo,
	oplog.Stack, oplog.StackPublish, oplog.StackSync, oplog.StackMerge,
}

// TestEveryOperationHasItsMessages: `ship log` names every kind of operation,
// and `ship status` says what `ship undo` would do for every kind it can undo.
// Both build their key at run time, so a missing one would show the raw key.
func TestEveryOperationHasItsMessages(t *testing.T) {
	known := i18n.Keys()
	for _, kind := range everyKind {
		if key := "cli.log.kind." + string(kind); !slices.Contains(known, key) {
			t.Errorf("%s is missing from the catalogue", key)
		}
		// A tidy is undoable only when it deleted a branch.
		op := oplog.Operation{ID: "op", Kind: kind, DeletedBranches: []oplog.BranchRef{{Name: "feat/a"}}}
		if _, undoable := oplog.LastUndoable([]oplog.Operation{op}); !undoable {
			continue
		}
		if key := "cli.status.undoable." + string(kind); !slices.Contains(known, key) {
			t.Errorf("%s is missing from the catalogue", key)
		}
	}
}
