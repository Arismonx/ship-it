// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

func TestRestoreTakesASafetyCheckpointFirst(t *testing.T) {
	store := &testkit.FakeCheckpointStore{}
	ctx := context.Background()
	first, _ := CreateCheckpoint(ctx, store, "")
	if first.Label != "manual" || len(first.ShortID) != shortIDLength {
		t.Fatalf("first = %+v", first)
	}
	result, err := RestoreCheckpoint(ctx, store, first.ShortID[:4])
	if err != nil {
		t.Fatal(err)
	}
	if store.Restored != first.ID || result.Safety.Label != "before-restore" {
		t.Fatalf("restore = %+v, restored %q", result, store.Restored)
	}
	if list, _ := ListCheckpoints(ctx, store, 10); len(list) != 2 || list[0].Label != "before-restore" {
		t.Fatalf("list = %+v", list)
	}
}

func TestRestoreUnknownOrAmbiguous(t *testing.T) {
	store := &testkit.FakeCheckpointStore{Checkpoints: []ports.Checkpoint{{ID: "abc1"}, {ID: "abc2"}}}
	ctx := context.Background()
	if _, err := RestoreCheckpoint(ctx, store, "abc"); !errors.Is(err, &fault.Fault{Code: fault.AmbiguousCheckpoint}) {
		t.Errorf("ambiguous id error = %v", err)
	}
	if _, err := RestoreCheckpoint(ctx, store, "zzz"); !errors.Is(err, &fault.Fault{Code: fault.UnknownCheckpoint}) {
		t.Errorf("unknown id error = %v", err)
	}
	if store.Restored != "" {
		t.Error("nothing may be restored on error")
	}
}
