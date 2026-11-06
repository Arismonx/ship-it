// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package snapshot_test

import (
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// BenchmarkParseUnifiedDiff parses a diff of 500 files, about 10,000 lines.
func BenchmarkParseUnifiedDiff(b *testing.B) {
	diff := benchdata.Diff(500, 6)
	b.ReportMetric(float64(strings.Count(diff, "\n")), "lines")
	b.SetBytes(int64(len(diff)))
	for b.Loop() {
		if files := snapshot.ParseUnifiedDiff(diff); len(files) != 500 {
			b.Fatalf("%d files", len(files))
		}
	}
}

// BenchmarkNew classifies the files of that diff.
func BenchmarkNew(b *testing.B) {
	diffs := snapshot.ParseUnifiedDiff(benchdata.Diff(500, 6))
	for b.Loop() {
		snapshot.New(diffs, snapshot.Options{})
	}
}
