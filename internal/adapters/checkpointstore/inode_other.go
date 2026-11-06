// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

//go:build !unix

package checkpointstore

import "io/fs"

// inodeOf: Windows reports no inode through Lstat (reading the file index
// would open every file, the cost the cache avoids).
func inodeOf(fs.FileInfo) string { return "-" }
