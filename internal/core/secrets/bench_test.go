// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package secrets_test

import (
	"testing"

	"github.com/GregoireF/ship-it/internal/core/secrets"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// BenchmarkScan scans the added lines of a 500-file diff (about 3,000).
func BenchmarkScan(b *testing.B) {
	diffs := snapshot.ParseUnifiedDiff(benchdata.Diff(500, 6))
	added := 0
	for _, d := range diffs {
		added += len(d.Added)
	}
	b.ReportMetric(float64(added), "lines")
	opts := secrets.Options{IgnorePaths: []string{"test/fixtures/**", "*.snap"}}
	for b.Loop() {
		secrets.Scan(diffs, opts)
	}
}
