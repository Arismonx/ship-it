// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import "context"

// Browser opens a page in the user's default browser, after a human agreed
// (ship bug, #53).
type Browser interface {
	Open(ctx context.Context, url string) error
}
