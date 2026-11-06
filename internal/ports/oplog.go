// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"

	"github.com/GregoireF/ship-it/internal/core/oplog"
)

// OperationLog persists the append-only log of ship-it operations.
type OperationLog interface {
	// Append records op and returns it with its ID and StartedAt set when
	// they were empty.
	Append(ctx context.Context, op oplog.Operation) (oplog.Operation, error)
	// Recent returns at most limit operations, newest first.
	Recent(ctx context.Context, limit int) ([]oplog.Operation, error)
}
