// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

//go:build unix

package checkpointstore

import (
	"io/fs"
	"strconv"
	"syscall"
)

// inodeOf tells a file replaced by another one of the same size and time.
func inodeOf(info fs.FileInfo) string {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return strconv.FormatUint(uint64(st.Ino), 10)
	}
	return "-"
}
