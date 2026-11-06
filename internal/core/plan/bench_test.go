// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package plan_test

import (
	"testing"

	"github.com/GregoireF/ship-it/internal/core/plan"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// BenchmarkPropose plans a 500-file work in progress.
func BenchmarkPropose(b *testing.B) {
	snap := snapshot.New(snapshot.ParseUnifiedDiff(benchdata.Diff(500, 6)), snapshot.Options{})
	for b.Loop() {
		if _, err := plan.Propose(snap, plan.Options{Issue: 42}); err != nil {
			b.Fatal(err)
		}
	}
}
