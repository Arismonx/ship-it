// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// importEntries writes the blobs of entries into the store with one
// `git fast-import` run and returns their ids in order. fast-import stores
// the bytes of a `data` command as they are ("raw file data (which Git does
// not interpret)", https://git-scm.com/docs/git-fast-import): no clean filter,
// no end-of-line conversion, the blob id `git hash-object --no-filters` gives.
//
// The stream is, per entry i, as documented there:
//
//	blob
//	mark :<i+1>
//	data <exact byte count>
//	<raw bytes>
//
// and ends with `done`, required by --done, so a stream cut short by a read
// error fails the import instead of storing part of it. --export-marks writes
// the marks table on completion, one `:<mark> <id>` per line; --quiet
// disables the statistics output. Contents are streamed one file at a time:
// the snapshot never holds the working tree in memory.
func (s *Store) importEntries(ctx context.Context, loc location, entries []fileEntry) ([]string, error) {
	marks := filepath.Join(loc.store, "checkpoint-"+strconv.Itoa(os.Getpid())+".marks")
	defer os.Remove(marks)

	reader, writer := io.Pipe()
	produced := make(chan error, 1)
	go func() {
		err := writeImportStream(writer, loc.top, entries)
		writer.CloseWithError(err)
		produced <- err
	}()
	_, err := s.Git.RunWithReader(ctx, loc.storeEnv(), reader, "fast-import", "--quiet", "--done", "--export-marks="+marks)
	// git may stop reading early (failure, cancellation): unblock the writer.
	reader.CloseWithError(io.ErrClosedPipe)
	if streamErr := <-produced; streamErr != nil {
		return nil, streamErr
	}
	if err != nil {
		return nil, err
	}
	return readMarks(marks, len(entries))
}

// writeImportStream writes the fast-import stream of entries.
func writeImportStream(w io.Writer, top string, entries []fileEntry) error {
	out := bufio.NewWriterSize(w, 64<<10)
	for i, e := range entries {
		content, err := entryContent(top, e)
		if err != nil {
			return err
		}
		// A bufio.Writer error is sticky: checking the last write is enough.
		fmt.Fprintf(out, "blob\nmark :%d\ndata %d\n", i+1, len(content))
		_, _ = out.Write(content)
		if err := out.WriteByte('\n'); err != nil {
			return err
		}
	}
	if _, err := out.WriteString("done\n"); err != nil {
		return err
	}
	return out.Flush()
}

// entryContent is what the blob of a working tree entry holds: the file's
// bytes, or a symlink's target with forward slashes, as git stores it.
func entryContent(top string, e fileEntry) ([]byte, error) {
	path := filepath.Join(top, filepath.FromSlash(e.path))
	if e.mode == modeSymlink {
		target, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		return []byte(filepath.ToSlash(target)), nil
	}
	return os.ReadFile(path)
}

// readMarks reads the marks table fast-import exported: ids[i] is the blob
// of mark i+1.
func readMarks(path string, count int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ids := make([]string, count)
	found := 0
	for _, line := range strings.Split(string(data), "\n") {
		mark, id, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || !strings.HasPrefix(mark, ":") {
			continue
		}
		n, err := strconv.Atoi(mark[1:])
		if err != nil || n < 1 || n > count || ids[n-1] != "" {
			return nil, fmt.Errorf("checkpointstore: unexpected fast-import mark %q", line)
		}
		ids[n-1] = id
		found++
	}
	if found != count {
		return nil, fmt.Errorf("checkpointstore: fast-import stored %d of %d files", found, count)
	}
	return ids, nil
}
