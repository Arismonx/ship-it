// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// cacheFile, in the store, remembers the blob of every file of the last
// checkpoint with the file's stat data, as git's index does: a file whose
// mode, size, modification time (and inode, where there is one) are
// unchanged is not read and hashed again. Hashing every file is what made a
// checkpoint cost a second on 2,000 files on Windows (docs/design/performance.md).
const cacheFile = "checkpoint-cache"

// cacheHeader starts the file, followed by the time it was written.
const cacheHeader = "ship-it checkpoint cache 1 "

// racyWindow: a file modified this close to the writing of the cache, or
// later, is hashed again. A change made right after it was hashed could keep
// the same size and modification time (coarse clocks, FAT's two seconds),
// git's "racy clean" problem.
const racyWindow = 3 * time.Second

// stamp is what must not change for a cached blob to be reused.
type stamp struct {
	mode  string
	size  int64
	mtime int64 // UnixNano
	inode string
}

func stampOf(mode string, info fs.FileInfo) stamp {
	return stamp{mode: mode, size: info.Size(), mtime: info.ModTime().UnixNano(), inode: inodeOf(info)}
}

// blobCache maps a path to its stamp and blob, as read from the store.
type blobCache struct {
	written int64 // UnixNano
	entries map[string]cachedBlob
}

type cachedBlob struct {
	stamp
	blob string
}

// lookup returns the cached blob of path when its stamp still holds.
func (c blobCache) lookup(path string, s stamp) (string, bool) {
	e, ok := c.entries[path]
	if !ok || e.stamp != s || s.mtime >= c.written-int64(racyWindow) {
		return "", false
	}
	return e.blob, true
}

// readCache reads the cache of the store; a missing or unreadable one is
// empty: everything is hashed, as without a cache.
func readCache(store string) blobCache {
	data, err := os.ReadFile(filepath.Join(store, cacheFile))
	header, records, ok := bytes.Cut(data, []byte("\n"))
	if err != nil || !ok || !bytes.HasPrefix(header, []byte(cacheHeader)) {
		return blobCache{}
	}
	written, err := strconv.ParseInt(string(header[len(cacheHeader):]), 10, 64)
	if err != nil {
		return blobCache{}
	}
	c := blobCache{written: written, entries: map[string]cachedBlob{}}
	for _, record := range strings.Split(string(records), "\x00") {
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Split(meta, " ")
		if !ok || len(fields) != 5 {
			continue
		}
		size, errSize := strconv.ParseInt(fields[2], 10, 64)
		mtime, errTime := strconv.ParseInt(fields[3], 10, 64)
		if errSize != nil || errTime != nil {
			continue
		}
		c.entries[path] = cachedBlob{stamp{mode: fields[1], size: size, mtime: mtime, inode: fields[4]}, fields[0]}
	}
	return c
}

// writeCache replaces the cache of the store with entries. It is only an
// optimisation: a failure (another process renaming at the same time) is
// ignored, and the next checkpoint hashes more.
func writeCache(store string, entries map[string]cachedBlob, now time.Time) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s%d\n", cacheHeader, now.UnixNano())
	for path, e := range entries {
		fmt.Fprintf(&b, "%s %s %d %d %s\t%s\x00", e.blob, e.mode, e.size, e.mtime, e.inode, path)
	}
	tmp := filepath.Join(store, cacheFile+"-"+strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, filepath.Join(store, cacheFile)); err != nil {
		_ = os.Remove(tmp)
	}
}

// forgetCache removes the cache: the objects it names may be gone.
func forgetCache(store string) { _ = os.Remove(filepath.Join(store, cacheFile)) }
