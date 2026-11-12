// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParallelRunsEverythingAndRaisesPanicsInTheCaller(t *testing.T) {
	var ran atomic.Int32
	defer func() {
		p := recover()
		gp, ok := p.(*goroutinePanic)
		if !ok || gp.value != "boom" || ran.Load() != 2 {
			t.Fatalf("recovered %v after %d functions", p, ran.Load())
		}
	}()
	parallel(func() { ran.Add(1) }, func() { ran.Add(1); explode() })
	t.Fatal("the panic must reach the caller")
}

func explode() { panic("boom") }

// TestParallelKeepsTheStackOfThePanic: the surfaces log fmt.Sprint of the
// recovered value next to their own stack, which is the caller's. The stack
// of the goroutine that panicked must be in the value, or the cause is lost
// (#104).
func TestParallelKeepsTheStackOfThePanic(t *testing.T) {
	defer func() {
		text := fmt.Sprint(recover())
		if !strings.HasPrefix(text, "boom\n") || !strings.Contains(text, "app.explode") {
			t.Fatalf("recovered value lost the cause:\n%s", text)
		}
	}()
	parallel(explode)
}

func TestParallelUnwrapsAnErrorPanic(t *testing.T) {
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("recovered %v, want an error wrapping io.ErrUnexpectedEOF", err)
		}
	}()
	parallel(func() { panic(io.ErrUnexpectedEOF) })
}
