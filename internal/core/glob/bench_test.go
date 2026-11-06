// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package glob_test

import (
	"testing"

	"github.com/GregoireF/ship-it/internal/core/glob"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// patterns are those of overrides and secret ignores in practice.
var patterns = []string{"docs/**", "*.md", "apps/web/**/*.vue", "**/*_test.go", ".github/workflows/*.yml", "packages/*/src/**"}

// BenchmarkAny matches 50,000 paths against six patterns.
func BenchmarkAny(b *testing.B) {
	paths := benchdata.Paths(50_000)
	for b.Loop() {
		matched := 0
		for _, p := range paths {
			if glob.Any(patterns, p) {
				matched++
			}
		}
		if matched == 0 {
			b.Fatal("no path matched")
		}
	}
}
