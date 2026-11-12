// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// parallel runs fns at once and waits for all of them. A panic in one is
// re-raised in the caller's goroutine once every function has returned, so
// that the surfaces' recovery (the black box, docs/reference/logs.md) still sees it instead
// of the process crashing. It is re-raised as a *goroutinePanic, which keeps
// the stack of the goroutine where it happened: the stack the surfaces record
// on recovery is the caller's, which would otherwise hide the cause (#104).
func parallel(fns ...func()) {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		panicked *goroutinePanic
	)
	wg.Add(len(fns))
	for _, fn := range fns {
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					stack := debug.Stack()
					mu.Lock()
					if panicked == nil {
						panicked = &goroutinePanic{value: p, stack: stack}
					}
					mu.Unlock()
				}
			}()
			fn()
		}()
	}
	wg.Wait()
	if panicked != nil {
		panic(panicked)
	}
}

// goroutinePanic is a panic carried out of a goroutine of parallel: the value
// it panicked with, and the stack of that goroutine.
type goroutinePanic struct {
	value any
	stack []byte
}

// Error gives the value first, then where it happened, so that fmt.Sprint of
// the recovered value (what the surfaces log) tells the whole story.
func (p *goroutinePanic) Error() string {
	return fmt.Sprintf("%v\n\npanicked in a goroutine of app.parallel:\n%s", p.value, p.stack)
}

// Unwrap returns the value when it is an error, for errors.Is and errors.As.
func (p *goroutinePanic) Unwrap() error {
	if err, ok := p.value.(error); ok {
		return err
	}
	return nil
}
