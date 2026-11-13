// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/ports"
)

// RefPrefix starts the ref of every checkpoint inside the private store:
// refs/checkpoints/<UTC nanoseconds, 19 digits>-<id>. Names sort by creation
// time, and each checkpoint is a parentless commit, so removing one never
// changes the id of another (ADR-0005, revision U2).
const RefPrefix = "refs/checkpoints/"

// storeDirectory is the private bare repository holding checkpoints, inside
// the user's git directory (option C of docs/design/undo-storage-optimizations.md):
// its refs never appear in `git log --all` or GUIs, `git push --mirror` cannot
// send them, and the user's `git gc` cannot prune their objects, which are
// copies, not shared.
const storeDirectory = "ship/store.git"

// GitRunner is the subset of the git adapter this store needs. It is
// declared here so that adapters never import each other (ARCHITECTURE.md).
type GitRunner interface {
	RunWithInput(ctx context.Context, env []string, stdin []byte, args ...string) ([]byte, error)
	RunWithReader(ctx context.Context, env []string, stdin io.Reader, args ...string) ([]byte, error)
	GitDir(ctx context.Context) (string, error)
	TopLevel(ctx context.Context) (string, error)
}

// Store implements ports.CheckpointStore.
//
// Checkpoints are byte-exact: blobs are hashed with --no-filters, or stored
// raw by fast-import (pack.go), and restored raw, so neither core.autocrlf nor
// .gitattributes can alter what an undo gives back.
type Store struct {
	Git GitRunner
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

var _ ports.CheckpointStore = (*Store)(nil)

// New returns a store using git.
func New(git GitRunner) *Store { return &Store{Git: git} }

// Git file modes stored in trees.
const (
	modeRegular    = "100644"
	modeExecutable = "100755"
	modeSymlink    = "120000"
)

// fileEntry is a working tree file to store, with its git mode and the
// stat data the blob cache compares.
type fileEntry struct {
	path, mode string
	stamp      stamp
}

// Message trailers storing checkpoint metadata.
const (
	trailerLabel  = "Ship-Label: "
	trailerBranch = "Ship-Branch: "
	trailerHead   = "Ship-Head: "
)

// location is where the user's working tree and the private store live.
type location struct {
	top   string // working tree root
	store string // private bare repository
}

// storeEnv makes git commands act on the private store.
func (l location) storeEnv(extra ...string) []string {
	return append([]string{"GIT_DIR=" + l.store}, extra...)
}

// locate finds the store, creating it when create is true.
func (s *Store) locate(ctx context.Context, create bool) (location, bool, error) {
	top, err := s.Git.TopLevel(ctx)
	if err != nil {
		return location{}, false, err
	}
	gitDir, err := s.Git.GitDir(ctx)
	if err != nil {
		return location{}, false, err
	}
	loc := location{top: top, store: filepath.Join(gitDir, filepath.FromSlash(storeDirectory))}
	if _, err := os.Stat(filepath.Join(loc.store, "objects")); err == nil {
		return loc, true, nil
	}
	if !create {
		return loc, false, nil
	}
	if _, err := s.Git.RunWithInput(ctx, nil, nil, "init", "--bare", "--quiet", loc.store); err != nil {
		return loc, false, err
	}
	return loc, true, nil
}

// Create implements ports.CheckpointStore.
func (s *Store) Create(ctx context.Context, label string) (ports.Checkpoint, error) {
	loc, _, err := s.locate(ctx, true)
	if err != nil {
		return ports.Checkpoint{}, err
	}
	tree, err := s.snapshotTree(ctx, loc)
	if err != nil {
		return ports.Checkpoint{}, err
	}
	// Where the user is, read from the user's repository.
	head := s.output(ctx, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	branch := s.output(ctx, nil, "branch", "--show-current")
	now := s.now()
	date := now.Format(time.RFC3339)
	identity := loc.storeEnv(
		"GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date,
		"GIT_AUTHOR_NAME=ship-it", "GIT_AUTHOR_EMAIL=checkpoint@ship-it.invalid",
		"GIT_COMMITTER_NAME=ship-it", "GIT_COMMITTER_EMAIL=checkpoint@ship-it.invalid",
	)
	message := "ship-it checkpoint\n\n" + trailerLabel + label + "\n" + trailerBranch + branch + "\n" + trailerHead + head + "\n"

	// Never signed, whatever commit.gpgSign says: a checkpoint is ship's own
	// record, and signing could ask for a passphrase in the middle of a run.
	out, err := s.Git.RunWithInput(ctx, identity, nil, "commit-tree", "--no-gpg-sign", tree, "-m", message)
	if err != nil {
		return ports.Checkpoint{}, err
	}
	id := strings.TrimSpace(string(out))
	// Each checkpoint has its own ref: concurrent processes never contend
	// (lock-free design, decision M3).
	ref := fmt.Sprintf("%s%019d-%s", RefPrefix, now.UnixNano(), id)
	if _, err := s.Git.RunWithInput(ctx, loc.storeEnv(), nil, "update-ref", ref, id); err != nil {
		return ports.Checkpoint{}, err
	}
	return ports.Checkpoint{ID: id, CreatedAt: now.UTC().Truncate(time.Second), Label: label, Branch: branch, Head: head}, nil
}

// snapshotTree copies every non-ignored file of the working tree, as is, into
// the store and returns the tree object. It never touches the user's index.
// Files unchanged since the last checkpoint reuse their blob (cache.go); when
// a reused blob is gone from the store (pruned), write-tree refuses the tree
// and the snapshot is taken again without the cache.
func (s *Store) snapshotTree(ctx context.Context, loc location) (string, error) {
	cache := readCache(loc.store)
	tree, err := s.writeTree(ctx, loc, cache)
	if err != nil && len(cache.entries) > 0 && ctx.Err() == nil {
		forgetCache(loc.store)
		tree, err = s.writeTree(ctx, loc, blobCache{})
	}
	return tree, err
}

func (s *Store) writeTree(ctx context.Context, loc location, cache blobCache) (string, error) {
	listed, err := s.Git.RunWithInput(ctx, nil, nil, "-C", loc.top, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	var entries []fileEntry
	seen := map[string]bool{}
	for _, path := range strings.Split(string(listed), "\x00") {
		if path == "" || seen[path] || strings.ContainsAny(path, "\n\r") {
			// --stdin-paths reads one path per line: such paths cannot be fed.
			continue
		}
		seen[path] = true
		info, err := os.Lstat(filepath.Join(loc.top, filepath.FromSlash(path)))
		if err != nil || info.IsDir() {
			continue // deleted in the working tree, or a submodule directory
		}
		mode := modeRegular
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			mode = modeSymlink
		case info.Mode()&0o111 != 0:
			mode = modeExecutable
		}
		entries = append(entries, fileEntry{path, mode, stampOf(mode, info)})
	}

	// Blobs of unchanged files come from the cache; the others are hashed.
	hashes := make([]string, len(entries))
	var changed []fileEntry
	var changedIndex []int
	for i, e := range entries {
		if blob, ok := cache.lookup(e.path, e.stamp); ok && e.mode != modeSymlink {
			hashes[i] = blob
			continue
		}
		changed, changedIndex = append(changed, e), append(changedIndex, i)
	}
	if len(changed) > 0 {
		ids, err := s.hashEntries(ctx, loc, changed)
		if err != nil {
			return "", err
		}
		for k, i := range changedIndex {
			hashes[i] = ids[k]
		}
	}
	var indexInfo bytes.Buffer
	for i, e := range entries {
		fmt.Fprintf(&indexInfo, "%s %s\t%s\x00", e.mode, hashes[i], e.path)
	}

	index := filepath.Join(loc.store, "checkpoint-"+strconv.Itoa(os.Getpid())+".index")
	defer os.Remove(index)
	env := loc.storeEnv("GIT_INDEX_FILE=" + index)
	if _, err := s.Git.RunWithInput(ctx, env, indexInfo.Bytes(), "update-index", "-z", "--index-info"); err != nil {
		return "", err
	}
	// write-tree checks that every blob exists in the store.
	out, err := s.Git.RunWithInput(ctx, env, nil, "write-tree")
	if err != nil {
		return "", err
	}
	next := make(map[string]cachedBlob, len(entries))
	for i, e := range entries {
		if e.mode != modeSymlink {
			next[e.path] = cachedBlob{e.stamp, hashes[i]}
		}
	}
	// The real clock: modification times are compared with it.
	writeCache(loc.store, next, time.Now())
	return strings.TrimSpace(string(out)), nil
}

// packThreshold is the number of new blobs from which a snapshot writes them
// with one `git fast-import` run, into a pack, instead of `git hash-object -w`,
// which writes one loose object (a file, compressed, fsynced) per blob: the
// first checkpoint of 2,000 files took 6.5 s on Windows that way. 100 is the
// default of transfer.unpackLimit, the value fastimport.unpackLimit falls back
// to: below it, fast-import itself explodes its pack into loose objects
// (https://git-scm.com/docs/git-fast-import, https://git-scm.com/docs/git-config),
// so it would gain nothing over hash-object, one process either way.
const packThreshold = 100

// hashEntries writes one blob per entry into the store and returns their ids
// in order. Symlinks are stored by their target, as git does.
func (s *Store) hashEntries(ctx context.Context, loc location, entries []fileEntry) ([]string, error) {
	if len(entries) >= packThreshold {
		return s.importEntries(ctx, loc, entries)
	}
	// Hashing runs from the working tree, but objects go to the store.
	objects := []string{"GIT_OBJECT_DIRECTORY=" + filepath.Join(loc.store, "objects")}
	hashes := make([]string, len(entries))
	var regular bytes.Buffer
	var regularIndex []int
	for i, e := range entries {
		if e.mode != modeSymlink {
			regular.WriteString(e.path + "\n")
			regularIndex = append(regularIndex, i)
			continue
		}
		target, err := os.Readlink(filepath.Join(loc.top, filepath.FromSlash(e.path)))
		if err != nil {
			return nil, err
		}
		out, err := s.Git.RunWithInput(ctx, objects, []byte(filepath.ToSlash(target)), "-C", loc.top, "hash-object", "-w", "--stdin")
		if err != nil {
			return nil, err
		}
		hashes[i] = strings.TrimSpace(string(out))
	}
	if len(regularIndex) > 0 {
		out, err := s.Git.RunWithInput(ctx, objects, regular.Bytes(), "-C", loc.top, "hash-object", "-w", "--no-filters", "--stdin-paths")
		if err != nil {
			return nil, err
		}
		ids := strings.Fields(string(out))
		if len(ids) != len(regularIndex) {
			return nil, fmt.Errorf("checkpointstore: hashed %d of %d files", len(ids), len(regularIndex))
		}
		for k, i := range regularIndex {
			hashes[i] = ids[k]
		}
	}
	return hashes, nil
}

// List implements ports.CheckpointStore.
func (s *Store) List(ctx context.Context, limit int) ([]ports.Checkpoint, error) {
	loc, exists, err := s.locate(ctx, false)
	if err != nil || !exists {
		return nil, err
	}
	// Ref names start with the creation time: descending name order is
	// newest first, exact to the nanosecond.
	out, err := s.Git.RunWithInput(ctx, loc.storeEnv(), nil, "for-each-ref",
		"--sort=-refname", "--count="+strconv.Itoa(limit),
		"--format=%(objectname)%00%(creatordate:iso-strict)%00%(contents)%00", RefPrefix)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(out), "\x00")
	var checkpoints []ports.Checkpoint
	for i := 0; i+2 < len(fields); i += 3 {
		created, _ := time.Parse(time.RFC3339, fields[i+1])
		cp := ports.Checkpoint{ID: strings.TrimSpace(fields[i]), CreatedAt: created}
		for _, l := range strings.Split(fields[i+2], "\n") {
			switch {
			case strings.HasPrefix(l, trailerLabel):
				cp.Label = strings.TrimPrefix(l, trailerLabel)
			case strings.HasPrefix(l, trailerBranch):
				cp.Branch = strings.TrimPrefix(l, trailerBranch)
			case strings.HasPrefix(l, trailerHead):
				cp.Head = strings.TrimPrefix(l, trailerHead)
			}
		}
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints, nil
}

// RestoreFiles implements ports.CheckpointStore. Contents are written raw,
// byte for byte, with their executable bit and symlinks.
func (s *Store) RestoreFiles(ctx context.Context, id string) error {
	loc, exists, err := s.locate(ctx, false)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("checkpointstore: no checkpoint has been taken in this repository")
	}
	listing, err := s.Git.RunWithInput(ctx, loc.storeEnv(), nil, "ls-tree", "-r", "-z", id)
	if err != nil {
		return err
	}
	type blob struct{ mode, sha, path string }
	var blobs []blob
	var request bytes.Buffer
	for _, record := range strings.Split(string(listing), "\x00") {
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		blobs = append(blobs, blob{fields[0], fields[2], path})
		request.WriteString(fields[2] + "\n")
	}
	if len(blobs) == 0 {
		return nil
	}
	out, err := s.Git.RunWithInput(ctx, loc.storeEnv(), request.Bytes(), "cat-file", "--batch")
	if err != nil {
		return err
	}
	reader := bufio.NewReader(bytes.NewReader(out))
	for _, b := range blobs {
		content, err := readBatchObject(reader)
		if err != nil {
			return err
		}
		if err := writeRestored(loc.top, filepath.Join(loc.top, filepath.FromSlash(b.path)), b.mode, content); err != nil {
			return err
		}
	}
	return nil
}

// Changes implements ports.CheckpointStore with git diff-tree in the store.
func (s *Store) Changes(ctx context.Context, from, to string) ([]ports.CheckpointChange, error) {
	loc, exists, err := s.locate(ctx, false)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("checkpointstore: no checkpoint has been taken in this repository")
	}
	args := []string{"diff-tree", "-r", "-z", "--no-renames", "--name-status", "--root"}
	if from != "" {
		args = append(args[:len(args)-1], from)
	}
	out, err := s.Git.RunWithInput(ctx, loc.storeEnv(), nil, append(args, to)...)
	if err != nil {
		return nil, err
	}
	statuses := map[string]string{"A": ports.ChangeAdded, "M": ports.ChangeModified, "T": ports.ChangeModified, "D": ports.ChangeDeleted}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	var changes []ports.CheckpointChange
	for i := 0; i+1 < len(fields); i++ {
		status, ok := statuses[fields[i]]
		if !ok {
			continue // with --root and one tree, the first field is the commit id
		}
		changes = append(changes, ports.CheckpointChange{Path: fields[i+1], Status: status})
		i++
	}
	return changes, nil
}

// Remove implements ports.CheckpointStore. Refs are deleted in one
// transaction, each only if it still names the expected checkpoint, then the
// objects nothing references any more are pruned from the private store.
func (s *Store) Remove(ctx context.Context, ids []string) error {
	loc, exists, err := s.locate(ctx, false)
	if err != nil || !exists || len(ids) == 0 {
		return err
	}
	out, err := s.Git.RunWithInput(ctx, loc.storeEnv(), nil, "for-each-ref", "--format=%(objectname) %(refname)", RefPrefix)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var transaction strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		id, ref, ok := strings.Cut(line, " ")
		if ok && wanted[id] {
			fmt.Fprintf(&transaction, "delete %s %s\n", ref, id)
		}
	}
	if transaction.Len() == 0 {
		return nil
	}
	if _, err := s.Git.RunWithInput(ctx, loc.storeEnv(), []byte(transaction.String()), "update-ref", "--stdin"); err != nil {
		return err
	}
	// The pruned blobs may be in the cache (write-tree would notice, at the
	// cost of a second snapshot).
	forgetCache(loc.store)
	_, err = s.Git.RunWithInput(ctx, loc.storeEnv(), nil, "gc", "--quiet", "--prune=now")
	return err
}

// readBatchObject reads one "<sha> <type> <size>\n<content>\n" record.
func readBatchObject(r *bufio.Reader) ([]byte, error) {
	header, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(header)
	if len(fields) != 3 {
		return nil, fmt.Errorf("checkpointstore: unexpected cat-file header %q", header)
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, err
	}
	content := make([]byte, size)
	if _, err := io.ReadFull(r, content); err != nil {
		return nil, err
	}
	if _, err := r.ReadByte(); err != nil { // trailing newline
		return nil, err
	}
	return content, nil
}

// writeRestored writes one checkpointed file back. It never writes through a
// symbolic link: a link that replaced the file since the checkpoint is
// removed (the file comes back as it was), and a parent directory that leads
// outside the working tree stops the restore — undo must not touch files
// elsewhere on the machine.
func writeRestored(top, path, mode string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := insideTree(top, filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	if mode == modeSymlink {
		_ = os.Remove(path)
		return os.Symlink(filepath.FromSlash(string(content)), path)
	}
	perm := os.FileMode(0o644)
	if mode == modeExecutable {
		perm = 0o755
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}

// output runs a read-only git command and returns its trimmed output, or an
// empty string when it fails (missing ref, unborn branch…).
func (s *Store) output(ctx context.Context, env []string, args ...string) string {
	out, err := s.Git.RunWithInput(ctx, env, nil, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// insideTree fails unless dir, links resolved, is inside the working tree top.
func insideTree(top, dir string) error {
	resolvedTop, err := filepath.EvalSymlinks(top)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedTop, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("checkpointstore: %s leads outside the working tree; not restored", dir)
	}
	return nil
}
