// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"testing"

	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// TestMakeBenchRepository writes the repository of the end-to-end benchmarks
// to $SHIP_BENCH_REPO, to run ship there by hand (`ship --debug status`
// shows every git process and its duration):
//
//	SHIP_BENCH_REPO=/tmp/widget go test -run TestMakeBenchRepository ./cmd/ship
func TestMakeBenchRepository(t *testing.T) {
	dir := os.Getenv("SHIP_BENCH_REPO")
	if dir == "" {
		t.Skip("SHIP_BENCH_REPO not set")
	}
	if err := benchdata.NewRepository(dir, benchRepo); err != nil {
		t.Fatal(err)
	}
}
