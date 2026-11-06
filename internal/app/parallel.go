// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import "sync"

// parallel runs fns at once and waits for all of them. A panic in one is
// re-raised in the caller's goroutine once every function has returned, so
// that the surfaces' recovery (the black box, docs/reference/logs.md) still sees it instead
// of the process crashing.
func parallel(fns ...func()) {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		panicked any
	)
	wg.Add(len(fns))
	for _, fn := range fns {
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					mu.Lock()
					if panicked == nil {
						panicked = p
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
