// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import "context"

// RunLogs are the technical journals of past runs (docs/reference/logs.md §5).
type RunLogs interface {
	// Prune keeps the keepRuns newest runs within maxBytes and removes the
	// others, never the current one; dryRun only counts them.
	Prune(ctx context.Context, keepRuns int, maxBytes int64, dryRun bool) (removedRuns int, freedBytes int64, err error)
}
