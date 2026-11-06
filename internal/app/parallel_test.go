// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"sync/atomic"
	"testing"
)

func TestParallelRunsEverythingAndRaisesPanicsInTheCaller(t *testing.T) {
	var ran atomic.Int32
	defer func() {
		if p := recover(); p != "boom" || ran.Load() != 2 {
			t.Fatalf("recovered %v after %d functions", p, ran.Load())
		}
	}()
	parallel(func() { ran.Add(1) }, func() { ran.Add(1); panic("boom") })
	t.Fatal("the panic must reach the caller")
}
