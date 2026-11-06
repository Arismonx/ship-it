// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"io"
	"sync"
)

// DefaultBlackBoxCapacity bounds the in-memory black box. It is an initial
// value to calibrate with benchmarks (docs/reference/logs.md §2).
const DefaultBlackBoxCapacity = 1 << 20 // 1 MiB

// BlackBox is a bounded ring of complete log lines. When full, the oldest
// lines are dropped so that the most recent context is always kept.
// It is safe for concurrent use and implements io.Writer.
type BlackBox struct {
	mu       sync.Mutex
	capacity int
	lines    [][]byte
	size     int
	dropped  int
}

// NewBlackBox returns a black box holding at most capacity bytes.
func NewBlackBox(capacity int) *BlackBox {
	if capacity <= 0 {
		capacity = DefaultBlackBoxCapacity
	}
	return &BlackBox{capacity: capacity}
}

// Write stores p as one line. slog handlers write exactly one record per call.
func (b *BlackBox) Write(p []byte) (int, error) {
	line := bytes.Clone(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	b.size += len(line)
	b.trim()
	return len(p), nil
}

// SetCapacity changes the bound (ship.logs.blackBoxBytes), dropping the
// oldest lines when the box now holds too much.
func (b *BlackBox) SetCapacity(capacity int) {
	if capacity <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.capacity = capacity
	b.trim()
}

// trim drops the oldest lines beyond the capacity; b.mu must be held.
func (b *BlackBox) trim() {
	for b.size > b.capacity && len(b.lines) > 1 {
		b.size -= len(b.lines[0])
		b.lines[0] = nil
		b.lines = b.lines[1:]
		b.dropped++
	}
}

// Dropped returns how many old lines were evicted to respect the capacity.
func (b *BlackBox) Dropped() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// WriteTo writes the retained lines, oldest first.
func (b *BlackBox) WriteTo(w io.Writer) (int64, error) {
	b.mu.Lock()
	lines := make([][]byte, len(b.lines))
	copy(lines, b.lines)
	b.mu.Unlock()
	var total int64
	for _, line := range lines {
		n, err := w.Write(line)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
