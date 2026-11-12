// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GregoireF/ship-it/internal/ports"
)

// Client implements ports.Git by executing the git binary. It never goes
// through a shell: arguments are passed as a slice.
type Client struct {
	// Dir is the directory git runs in.
	Dir string
	// Binary is the git executable; "git" (looked up in PATH) when empty.
	Binary string
	// Log receives one DEBUG record per git invocation; nil logs nothing.
	Log *slog.Logger

	// cache holds what does not change during a run and costs a process
	// each time (about 20 ms on Windows): the repository paths, and the
	// remote URLs until ship-it itself changes the git configuration.
	cache struct {
		sync.Mutex
		topLevel, gitDir string
		remotes          map[string]*string // nil: no such remote
	}
}

var _ ports.Git = (*Client)(nil)

// New returns a client running git in dir.
func New(dir string) *Client { return &Client{Dir: dir} }

// Error reports a failed git invocation with its stderr.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", strings.Join(e.Args, " "), e.ExitCode, strings.TrimSpace(e.Stderr))
}

// Is lets errors.Is(err, ports.ErrNotARepository) recognise git run outside a
// repository: exit 128 and git's "fatal: not a git repository" (LC_ALL=C
// keeps it untranslated).
func (e *Error) Is(target error) bool {
	return target == ports.ErrNotARepository && e.ExitCode == 128 && strings.Contains(e.Stderr, "not a git repository")
}

// Run executes git with args and returns stdout.
func (c *Client) Run(ctx context.Context, args ...string) ([]byte, error) {
	return c.RunWithEnv(ctx, nil, args...)
}

// RunWithEnv executes git with extra environment variables, such as
// GIT_INDEX_FILE to work on a private index.
func (c *Client) RunWithEnv(ctx context.Context, env []string, args ...string) ([]byte, error) {
	return c.RunWithInput(ctx, env, nil, args...)
}

// RunWithInput executes git with extra environment variables and stdin.
func (c *Client) RunWithInput(ctx context.Context, env []string, stdin []byte, args ...string) ([]byte, error) {
	if stdin == nil {
		return c.RunWithReader(ctx, env, nil, args...)
	}
	return c.RunWithReader(ctx, env, bytes.NewReader(stdin), args...)
}

// RunWithReader executes git with extra environment variables, streaming
// stdin from a reader: input too large to hold in memory, such as a
// fast-import stream of file contents. A nil reader is no input.
func (c *Client) RunWithReader(ctx context.Context, env []string, stdin io.Reader, args ...string) ([]byte, error) {
	binary := c.Binary
	if binary == "" {
		binary = "git"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // never block on a credential prompt
		"LC_ALL=C",              // stable, untranslated messages for parsing
		"GIT_OPTIONAL_LOCKS=0",  // read commands must not take the index lock
	)
	cmd.Env = append(cmd.Env, env...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	c.log(ctx, args, exitCode, time.Since(start), stderr.String())
	if err != nil {
		return nil, &Error{Args: args, ExitCode: exitCode, Stderr: stderr.String()}
	}
	return stdout.Bytes(), nil
}

// maxDetail bounds the stderr line kept in the journal.
const maxDetail = 200

// log records one git invocation in the black box (DEBUG): the arguments
// with configuration values redacted, the exit code and the duration. Never
// stdout (file contents, diffs) nor stdin (commit messages).
func (c *Client) log(ctx context.Context, args []string, exitCode int, elapsed time.Duration, stderr string) {
	if c.Log == nil {
		return
	}
	attrs := []any{
		"step", "git", "cmd", LogArgs(args), "exit_code", exitCode, "duration_ms", elapsed.Milliseconds(),
	}
	if exitCode != 0 {
		detail := strings.TrimSpace(stderr)
		if i := strings.LastIndexByte(detail, '\n'); i >= 0 {
			detail = detail[i+1:]
		}
		if len(detail) > maxDetail {
			detail = detail[:maxDetail]
		}
		if detail != "" {
			attrs = append(attrs, "detail", detail)
		}
	}
	c.Log.DebugContext(ctx, "git.exec", attrs...)
}

// LogArgs returns git arguments fit for the journal: the value of
// `git config <key> <value>` and of `-c key=value` options is redacted, as
// it may hold a credential (http.extraheader…) or an e-mail.
func LogArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out); i++ {
		if out[i] == "-c" && i+1 < len(out) {
			if key, _, ok := strings.Cut(out[i+1], "="); ok && key != "core.quotePath" {
				out[i+1] = key + "=" + redacted
			}
			i++
			continue
		}
		// The first other argument is the subcommand.
		if out[i] == "config" {
			// Positional arguments after the options: key, then value.
			var positional []int
			for k := i + 1; k < len(out); k++ {
				if !strings.HasPrefix(out[k], "-") {
					positional = append(positional, k)
				}
			}
			for _, k := range positional[min(1, len(positional)):] {
				out[k] = redacted
			}
		}
		break
	}
	return out
}

// redacted matches obs.Redacted; adapters log through *slog.Logger only.
const redacted = "[REDACTED]"

// Version implements ports.Git.
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := c.Run(ctx, "--version")
	if err != nil {
		return "", err
	}
	// "git version 2.52.0.windows.1" → "2.52.0.windows.1"
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "git version "), nil
}

// TopLevel implements ports.Git.
func (c *Client) TopLevel(ctx context.Context) (string, error) {
	top, _, err := c.paths(ctx)
	return top, err
}

// GitDir implements ports.Git.
func (c *Client) GitDir(ctx context.Context) (string, error) {
	_, dir, err := c.paths(ctx)
	return dir, err
}

// paths reads the root of the working tree and the git directory with one
// rev-parse, once per client: neither moves during a run. A failure (not a
// repository yet) is not cached.
func (c *Client) paths(ctx context.Context) (topLevel, gitDir string, err error) {
	c.cache.Lock()
	defer c.cache.Unlock()
	if c.cache.topLevel != "" {
		return c.cache.topLevel, c.cache.gitDir, nil
	}
	out, err := c.Run(ctx, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return "", "", fmt.Errorf("gitexec: unexpected rev-parse output %q", out)
	}
	c.cache.topLevel = filepath.Clean(strings.TrimSpace(lines[0]))
	c.cache.gitDir = filepath.Clean(strings.TrimSpace(lines[1]))
	return c.cache.topLevel, c.cache.gitDir, nil
}

// Status implements ports.Git.
func (c *Client) Status(ctx context.Context) (ports.WorkingTreeStatus, error) {
	out, err := c.Run(ctx, StatusArgs...)
	if err != nil {
		return ports.WorkingTreeStatus{}, err
	}
	return ParseStatus(out)
}

// DiffFromHead implements ports.Git.
func (c *Client) DiffFromHead(ctx context.Context) (string, error) {
	out, err := c.Run(ctx, "-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "--find-renames", "HEAD", "--")
	return string(out), err
}

// ReadWorkingFile implements ports.Git.
func (c *Client) ReadWorkingFile(ctx context.Context, path string, limit int64) ([]byte, error) {
	top, err := c.TopLevel(ctx)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(top, filepath.FromSlash(path))
	if rel, err := filepath.Rel(top, full); err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("gitexec: %q is outside the working tree", path)
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, ports.ErrFileTooLarge
	}
	return os.ReadFile(full)
}

// ConfigValue implements ports.Git. git exits with 1 when the key is unset.
func (c *Client) ConfigValue(ctx context.Context, key string) (string, bool, error) {
	out, err := c.Run(ctx, "config", "--get", key)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

// ConfigMatching implements ports.Git with `git config -z --get-regexp`,
// whose records are "key\nvalue" separated by NUL bytes.
func (c *Client) ConfigMatching(ctx context.Context, keyPattern string) (map[string]string, error) {
	out, err := c.Run(ctx, "config", "-z", "--get-regexp", keyPattern)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, record := range strings.Split(string(out), "\x00") {
		if record == "" {
			continue
		}
		key, value, _ := strings.Cut(record, "\n")
		values[key] = value
	}
	return values, nil
}

// Remotes implements ports.Git.
func (c *Client) Remotes(ctx context.Context) ([]string, error) {
	out, err := c.Run(ctx, "remote")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// operationMarkers are the files git keeps in its directory while an
// operation waits for the user.
var operationMarkers = []struct{ file, operation string }{
	{"MERGE_HEAD", "merge"},
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
}

// OperationInProgress implements ports.Git.
func (c *Client) OperationInProgress(ctx context.Context) (string, error) {
	gitDir, err := c.GitDir(ctx)
	if err != nil {
		return "", err
	}
	for _, m := range operationMarkers {
		if _, err := os.Stat(filepath.Join(gitDir, m.file)); err == nil {
			return m.operation, nil
		}
	}
	return "", nil
}

// LocalBranchExists implements ports.Git.
func (c *Client) LocalBranchExists(ctx context.Context, name string) (bool, error) {
	_, err := c.Run(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return false, nil
	}
	return err == nil, err
}

// SwitchToNewBranch implements ports.Git.
func (c *Client) SwitchToNewBranch(ctx context.Context, name string) error {
	_, err := c.Run(ctx, "switch", "--quiet", "--create", name)
	return err
}

// StageAll implements ports.Git.
func (c *Client) StageAll(ctx context.Context) error {
	_, err := c.Run(ctx, "add", "--all", "--", ":/")
	return err
}

// Commit implements ports.Git. The message goes through stdin so that no
// shell or argument length limit ever applies.
func (c *Client) Commit(ctx context.Context, message string, opts ports.CommitOptions) (string, error) {
	args := append([]string{"commit", "--quiet", "--file=-"}, commitArgs(opts)...)
	if _, err := c.RunWithInput(ctx, nil, []byte(message), args...); err != nil {
		return "", err
	}
	out, err := c.Run(ctx, "rev-parse", "HEAD")
	return strings.TrimSpace(string(out)), err
}

// commitArgs are the options of a commit (or merge) beyond the user's
// configuration.
func commitArgs(opts ports.CommitOptions) []string {
	var args []string
	if opts.Signoff {
		args = append(args, "--signoff")
	}
	return append(args, signArgs(opts.Sign)...)
}

// signArgs override commit.gpgSign: -S signs with the configured key,
// --no-gpg-sign never signs.
func signArgs(mode ports.SignMode) []string {
	switch mode {
	case ports.SignAlways:
		return []string{"--gpg-sign"}
	case ports.SignNever:
		return []string{"--no-gpg-sign"}
	}
	return nil
}

// RecentMessages implements ports.Git; %B is the raw message.
func (c *Client) RecentMessages(ctx context.Context, limit int) ([]string, error) {
	out, err := c.Run(ctx, "log", "-z", "--format=%B", "--max-count="+strconv.Itoa(limit), "HEAD")
	if err == nil {
		return splitNul(out), nil
	}
	if _, headErr := c.Run(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); headErr != nil {
		return nil, nil // no commit yet
	}
	return nil, err
}

// Fetch implements ports.Git.
func (c *Client) Fetch(ctx context.Context, remote string) error {
	// A fetch only reads: retrying a dropped connection is always safe.
	return c.retry(ctx, transientFetch, "fetch", "--quiet", "--prune", remote)
}

// FetchBranch implements ports.Git: the refspec writes ref only, and
// --no-tags keeps the parent's tags out of the repository.
func (c *Client) FetchBranch(ctx context.Context, url, branch, ref string) error {
	if strings.HasPrefix(url, "-") {
		return fmt.Errorf("gitexec: not a repository URL: %q", url)
	}
	return c.retry(ctx, transientFetch, "fetch", "--quiet", "--no-tags", url, "+refs/heads/"+branch+":"+ref)
}

// Connection failures worth one more try, as git words them. A fetch may be
// retried on any of them; a push only when the connection never opened, as
// nothing can have been transferred then.
var (
	transientPush  = []string{"Could not read from remote repository", "Could not resolve host"}
	transientFetch = append([]string{"The remote end hung up unexpectedly", "early EOF", "Connection reset", "Connection timed out", "Operation timed out"}, transientPush...)
)

// retryDelays are the waits before the second and third attempts.
var retryDelays = []time.Duration{500 * time.Millisecond, 2 * time.Second}

// retry runs git, and again after a short wait when it fails with one of
// transient: the network, a VPN or the system dropped the connection.
func (c *Client) retry(ctx context.Context, transient []string, args ...string) error {
	_, err := c.Run(ctx, args...)
	for _, delay := range retryDelays {
		var gitErr *Error
		if !errors.As(err, &gitErr) || !containsAny(gitErr.Stderr, transient) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
		_, err = c.Run(ctx, args...)
	}
	return err
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// CommitOf implements ports.Git.
func (c *Client) CommitOf(ctx context.Context, ref string) (string, error) {
	out, err := c.Run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return "", nil
	}
	return strings.TrimSpace(string(out)), err
}

// IsAncestor implements ports.Git.
func (c *Client) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	_, err := c.Run(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return false, nil
	}
	return err == nil, err
}

// LocalBranches implements ports.Git.
func (c *Client) LocalBranches(ctx context.Context) (map[string]string, error) {
	out, err := c.Run(ctx, "for-each-ref", "--format=%(refname:short)%00%(objectname)", "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name, commit, ok := strings.Cut(line, "\x00"); ok {
			branches[name] = commit
		}
	}
	return branches, nil
}

// SwitchBranch implements ports.Git.
func (c *Client) SwitchBranch(ctx context.Context, branch string) error {
	_, err := c.Run(ctx, "switch", "--quiet", branch)
	return err
}

// FastForward implements ports.Git.
func (c *Client) FastForward(ctx context.Context, ref string) error {
	_, err := c.Run(ctx, "merge", "--ff-only", "--quiet", ref)
	return err
}

// Stash implements ports.Git.
func (c *Client) Stash(ctx context.Context, message string) error {
	_, err := c.Run(ctx, "stash", "push", "--include-untracked", "--quiet", "--message", message)
	return err
}

// StashAppliesCleanlyOn implements ports.Git with `merge-tree --merge-base`
// (git ≥ 2.40), which reads neither the working tree nor the index.
func (c *Client) StashAppliesCleanlyOn(ctx context.Context, ref string) (bool, error) {
	_, err := c.Run(ctx, "merge-tree", "--write-tree", "--quiet", "--merge-base=stash@{0}^1", ref, "stash@{0}")
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Untracked files live in the third parent of the stash, outside the merge.
	untracked, err := c.CommitOf(ctx, "stash@{0}^3")
	if err != nil || untracked == "" {
		return err == nil, err
	}
	out, err := c.Run(ctx, "ls-tree", "-r", "-z", "--name-only", untracked)
	if err != nil {
		return false, err
	}
	paths := splitNul(out)
	if len(paths) == 0 {
		return true, nil
	}
	colliding, err := c.Run(ctx, append([]string{"ls-tree", "-r", "-z", "--name-only", ref, "--"}, paths...)...)
	return len(splitNul(colliding)) == 0, err
}

// StashPop implements ports.Git.
func (c *Client) StashPop(ctx context.Context) error {
	_, err := c.Run(ctx, "stash", "pop", "--quiet")
	return err
}

// RecreateBranch implements ports.Git: an empty old value makes update-ref
// refuse when the branch exists.
func (c *Client) RecreateBranch(ctx context.Context, branch, commit string) error {
	_, err := c.Run(ctx, "update-ref", "-m", reflogMessage, "refs/heads/"+branch, commit, "")
	return err
}

// RemoteURL implements ports.Git. It returns the URL as configured, not
// rewritten by url.<base>.insteadOf: the rewrite is a transport detail
// (corporate mirror, ssh/https swap), while the configured URL names the
// GitHub repository.
func (c *Client) RemoteURL(ctx context.Context, remote string) (string, error) {
	c.cache.Lock()
	url, cached := c.cache.remotes[remote]
	c.cache.Unlock()
	if cached && url == nil {
		return "", fmt.Errorf("gitexec: no remote named %s", remote)
	}
	if cached {
		return *url, nil
	}
	value, found, err := c.ConfigValue(ctx, "remote."+remote+".url")
	if err != nil {
		return "", err // not cached: may be transient
	}
	c.cache.Lock()
	if c.cache.remotes == nil {
		c.cache.remotes = map[string]*string{}
	}
	if found {
		c.cache.remotes[remote] = &value
	} else {
		c.cache.remotes[remote] = nil
	}
	c.cache.Unlock()
	if !found {
		return "", fmt.Errorf("gitexec: no remote named %s", remote)
	}
	return value, nil
}

// forgetRemotes drops the cached remote URLs: the configuration changed.
func (c *Client) forgetRemotes() {
	c.cache.Lock()
	c.cache.remotes = nil
	c.cache.Unlock()
}

// RemoteBranchExists implements ports.Git.
func (c *Client) RemoteBranchExists(ctx context.Context, remote, branch string) (bool, error) {
	_, err := c.Run(ctx, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote+"/"+branch)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return false, nil
	}
	return err == nil, err
}

// RecentSubjects implements ports.Git.
// One process when there are commits: HEAD is checked only when log fails.
func (c *Client) RecentSubjects(ctx context.Context, limit int) ([]string, error) {
	out, err := c.Run(ctx, "log", "-z", "--format=%s", "--max-count="+strconv.Itoa(limit), "HEAD")
	if err == nil {
		return splitNul(out), nil
	}
	if _, headErr := c.Run(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); headErr != nil {
		return nil, nil // no commit yet
	}
	return nil, err
}

// FilesAt implements ports.Git with `git ls-tree -z --name-only`.
func (c *Client) FilesAt(ctx context.Context, rev, dir string) ([]string, error) {
	args := []string{"ls-tree", "-z", "--name-only", rev}
	if dir != "" {
		args = append(args, "--", strings.TrimSuffix(dir, "/")+"/")
	}
	out, err := c.Run(ctx, args...)
	return splitNul(out), err
}

// ReadFileAt implements ports.Git: `git cat-file -s` checks the size before
// `git cat-file blob` reads it.
func (c *Client) ReadFileAt(ctx context.Context, rev, path string, limit int64) ([]byte, error) {
	object := rev + ":" + path
	size, err := c.Run(ctx, "cat-file", "-s", object)
	if err != nil {
		return nil, err
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(string(size)), 10, 64); err != nil || n > limit {
		return nil, ports.ErrFileTooLarge
	}
	return c.Run(ctx, "cat-file", "blob", object)
}

// CommitMessagesSince implements ports.Git; %B is the raw message.
func (c *Client) CommitMessagesSince(ctx context.Context, base string) ([]string, error) {
	args := []string{"log", "-z", "--reverse", "--format=%B"}
	if base == "" {
		args = append(args, "-1", "HEAD")
	} else {
		args = append(args, base+"..HEAD")
	}
	out, err := c.Run(ctx, args...)
	return splitNul(out), err
}

// commitFields is the number of NUL-separated fields of commitFormat.
const commitFields = 7

// commitFormat prints raw identities (%an, not the .mailmap'ed %aN) and the
// raw message; with -z, commits are NUL-separated too, so every commit is
// exactly commitFields fields (a message cannot contain NUL).
const commitFormat = "--format=%H%x00%P%x00%an%x00%ae%x00%cn%x00%ce%x00%B"

// CommitsBetween implements ports.Git.
func (c *Client) CommitsBetween(ctx context.Context, from, to string) ([]ports.CommitRecord, error) {
	out, err := c.Run(ctx, "log", "-z", "--reverse", commitFormat, from+".."+to, "--")
	if err != nil {
		return nil, err
	}
	return ParseCommits(out)
}

// ParseCommits reads the output of `git log -z` with commitFormat.
func ParseCommits(out []byte) ([]ports.CommitRecord, error) {
	text := strings.TrimSuffix(string(out), "\x00")
	if text == "" {
		return nil, nil
	}
	fields := strings.Split(text, "\x00")
	if len(fields)%commitFields != 0 {
		return nil, fmt.Errorf("gitexec: unexpected log output (%d fields)", len(fields))
	}
	commits := make([]ports.CommitRecord, 0, len(fields)/commitFields)
	for i := 0; i < len(fields); i += commitFields {
		f := fields[i : i+commitFields]
		commits = append(commits, ports.CommitRecord{
			ID:             strings.TrimLeft(f[0], "\n"),
			Parents:        strings.Fields(f[1]),
			AuthorName:     f[2],
			AuthorEmail:    f[3],
			CommitterName:  f[4],
			CommitterEmail: f[5],
			Message:        strings.TrimRight(f[6], "\n"),
		})
	}
	return commits, nil
}

// SignedCommits implements ports.Git: one `git cat-file --batch` reads the
// commit objects, and a signature header (gpgsig, or gpgsig-sha256 in
// SHA-256 repositories) before the message marks a signed commit. %G? is not
// used: git reports "N" for an SSH signature it cannot verify without
// gpg.ssh.allowedSignersFile (git 2.52, TestSignedCommits).
func (c *Client) SignedCommits(ctx context.Context, ids []string) (map[string]bool, error) {
	signed := map[string]bool{}
	if len(ids) == 0 {
		return signed, nil
	}
	out, err := c.RunWithInput(ctx, nil, []byte(strings.Join(ids, "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	rest := out
	for len(rest) > 0 {
		header, after, ok := bytes.Cut(rest, []byte("\n"))
		if !ok {
			break
		}
		fields := strings.Fields(string(header))
		if len(fields) != 3 {
			return nil, fmt.Errorf("gitexec: unexpected cat-file header %q", header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size > len(after) {
			return nil, fmt.Errorf("gitexec: unexpected cat-file header %q", header)
		}
		object := string(after[:size])
		headers, _, _ := strings.Cut(object, "\n\n")
		for _, line := range strings.Split(headers, "\n") {
			if strings.HasPrefix(line, "gpgsig ") || strings.HasPrefix(line, "gpgsig-sha256 ") {
				signed[fields[0]] = true
			}
		}
		rest = bytes.TrimPrefix(after[size:], []byte("\n"))
	}
	return signed, nil
}

// CommitPatches implements ports.Git with `git log -p` over HEAD --not
// exclude, in the unified diff form DiffFromHead uses. A merge shows what its
// author changed against git's own re-merge (--diff-merges=remerge): a
// conflict resolution is scanned, the merged-in branch's commits are not
// shown twice. (git documents that remerge output may evolve; a test pins
// what ship-it relies on.)
func (c *Client) CommitPatches(ctx context.Context, exclude []string) (string, error) {
	args := []string{"-c", "core.quotePath=false", "log", "-p", "--reverse", "--diff-merges=remerge", "--format=",
		"--no-color", "--no-ext-diff", "--find-renames", "HEAD"}
	if len(exclude) > 0 {
		args = append(append(args, "--not"), exclude...)
	}
	out, err := c.Run(ctx, append(args, "--")...)
	return string(out), err
}

// DiffStatSince implements ports.Git.
func (c *Client) DiffStatSince(ctx context.Context, base string) ([]ports.FileStat, error) {
	args := []string{"diff", "--numstat", "-z", "--no-renames", base + "...HEAD", "--"}
	if base == "" {
		args = []string{"diff-tree", "--root", "--no-commit-id", "--numstat", "-z", "--no-renames", "-r", "HEAD"}
	}
	out, err := c.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParseNumstat(out)
}

// recentAuthorsPaths bounds the paths given to git log: the command line of
// Windows holds 32,767 characters.
const recentAuthorsPaths = 100

// RecentAuthors implements ports.Git. Paths are literal, never patterns.
func (c *Client) RecentAuthors(ctx context.Context, rev, since string, paths []string) ([]ports.CommitAuthor, error) {
	if len(paths) > recentAuthorsPaths {
		paths = paths[:recentAuthorsPaths]
	}
	args := append([]string{"--literal-pathspecs", "log", "--no-merges", "--since=" + since, "--format=%H%x1f%an%x1f%ae", rev, "--"}, paths...)
	out, err := c.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var authors []ports.CommitAuthor
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Split(strings.TrimSpace(line), "\x1f"); len(f) == 3 {
			authors = append(authors, ports.CommitAuthor{SHA: f[0], Name: f[1], Email: f[2]})
		}
	}
	return authors, nil
}

// DiffSince implements ports.Git.
func (c *Client) DiffSince(ctx context.Context, base string) (string, error) {
	args := []string{"-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "--find-renames", "-U0", base + "...HEAD", "--"}
	if base == "" {
		args = []string{"-c", "core.quotePath=false", "diff-tree", "-p", "--root", "--no-commit-id", "--no-color",
			"--no-ext-diff", "--find-renames", "-U0", "-r", "HEAD"}
	}
	out, err := c.Run(ctx, args...)
	return string(out), err
}

// DiffStatBetween implements ports.Git.
func (c *Client) DiffStatBetween(ctx context.Context, base, head string) ([]ports.FileStat, error) {
	out, err := c.Run(ctx, "diff", "--numstat", "-z", "--no-renames", base+"..."+head, "--")
	if err != nil {
		return nil, err
	}
	return ParseNumstat(out)
}

// ParseNumstat reads `--numstat -z --no-renames` output: one
// "<added>\t<deleted>\t<path>" record per NUL, "-" counts for binary files.
func ParseNumstat(out []byte) ([]ports.FileStat, error) {
	var stats []ports.FileStat
	for _, record := range strings.Split(string(out), "\x00") {
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 || fields[2] == "" {
			return nil, fmt.Errorf("gitexec: unexpected numstat record %q", record)
		}
		stat := ports.FileStat{Path: fields[2]}
		if fields[0] == "-" && fields[1] == "-" {
			stat.Binary = true
		} else {
			var errAdded, errDeleted error
			stat.Added, errAdded = strconv.Atoi(fields[0])
			stat.Deleted, errDeleted = strconv.Atoi(fields[1])
			if errAdded != nil || errDeleted != nil {
				return nil, fmt.Errorf("gitexec: unexpected numstat record %q", record)
			}
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// GeneratedPaths implements ports.Git with `git check-attr -z --stdin`.
func (c *Client) GeneratedPaths(ctx context.Context, paths []string) (map[string]bool, error) {
	generated := map[string]bool{}
	if len(paths) == 0 {
		return generated, nil
	}
	out, err := c.RunWithInput(ctx, nil, []byte(strings.Join(paths, "\x00")+"\x00"), "check-attr", "-z", "--stdin", "linguist-generated")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i+2 < len(fields); i += 3 {
		// "set" for a bare attribute, "true" for linguist-generated=true.
		if value := fields[i+2]; value == "set" || value == "true" {
			generated[fields[i]] = true
		}
	}
	return generated, nil
}

// CompareWith implements ports.Git.
func (c *Client) CompareWith(ctx context.Context, ref string) (ahead, behind int, err error) {
	out, err := c.Run(ctx, "rev-list", "--left-right", "--count", ref+"...HEAD")
	if err != nil {
		return 0, 0, err
	}
	left, right, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok {
		return 0, 0, fmt.Errorf("gitexec: unexpected rev-list output %q", out)
	}
	if behind, err = strconv.Atoi(left); err != nil {
		return 0, 0, err
	}
	ahead, err = strconv.Atoi(right)
	return ahead, behind, err
}

// RemoteDefaultBranch implements ports.Git.
func (c *Client) RemoteDefaultBranch(ctx context.Context, remote string) (string, error) {
	out, err := c.Run(ctx, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD")
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), remote+"/"), nil
}

// Push implements ports.Git. The explicit refspec never pushes anything but
// the branch, whatever push.default says.
func (c *Client) Push(ctx context.Context, remote, branch, lease string) error {
	ref := "refs/heads/" + branch
	args := []string{"push", "--quiet", "--set-upstream"}
	if lease != "" {
		args = append(args, "--force-with-lease="+ref+":"+lease)
	}
	return c.retry(ctx, transientPush, append(args, remote, ref+":"+ref)...)
}

// PushCommit implements ports.Git. An empty expected value in the lease
// means the branch must not exist on the remote (git-push(1)).
func (c *Client) PushCommit(ctx context.Context, remote, branch, commit, expected string) error {
	ref := "refs/heads/" + branch
	return c.retry(ctx, transientPush, "push", "--quiet", "--force-with-lease="+ref+":"+expected, remote, commit+":"+ref)
}

// Rebase implements ports.Git.
func (c *Client) Rebase(ctx context.Context, onto string) (bool, error) {
	return c.RebaseWith(ctx, onto, ports.CommitOptions{})
}

// RebaseWith implements ports.Git.
func (c *Client) RebaseWith(ctx context.Context, onto string, opts ports.CommitOptions) (bool, error) {
	args := append([]string{"rebase", "--autostash", "--quiet"}, signArgs(opts.Sign)...)
	_, err := c.Run(ctx, append(args, onto)...)
	if err == nil {
		return false, nil
	}
	if inProgress, _ := c.OperationInProgress(ctx); inProgress == "rebase" {
		if _, abortErr := c.Run(ctx, "rebase", "--abort"); abortErr != nil {
			return true, abortErr
		}
		return true, nil
	}
	return false, err
}

// RebaseOnto implements ports.Git.
func (c *Client) RebaseOnto(ctx context.Context, branch, onto, upstream string, opts ports.CommitOptions) (bool, error) {
	args := append([]string{"rebase", "--quiet"}, signArgs(opts.Sign)...)
	_, err := c.Run(ctx, append(args, "--onto", onto, upstream, branch)...)
	if err == nil {
		return false, nil
	}
	if inProgress, _ := c.OperationInProgress(ctx); inProgress == "rebase" {
		if _, abortErr := c.Run(ctx, "rebase", "--abort"); abortErr != nil {
			return true, abortErr
		}
		return true, nil
	}
	return false, err
}

// Merge implements ports.Git. --no-edit keeps git's own message ("Merge
// remote-tracking branch ..."), which commit conventions exempt.
func (c *Client) Merge(ctx context.Context, ref string, opts ports.CommitOptions) (bool, error) {
	args := append([]string{"merge", "--autostash", "--no-edit", "--quiet"}, commitArgs(opts)...)
	_, err := c.Run(ctx, append(args, ref)...)
	if err == nil {
		return false, nil
	}
	if inProgress, _ := c.OperationInProgress(ctx); inProgress == "merge" {
		if _, abortErr := c.Run(ctx, "merge", "--abort"); abortErr != nil {
			return true, abortErr
		}
		return true, nil
	}
	return false, err
}

// MergeBase implements ports.Git; exit status 1 means no common ancestor.
func (c *Client) MergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := c.Run(ctx, "merge-base", a, b)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return "", nil
	}
	return strings.TrimSpace(string(out)), err
}

// DeleteRemoteBranch implements ports.Git. The lease guards the deletion
// too: a stale expectation is rejected with "stale info" (checked on git
// 2.52, see TestDeleteRemoteBranchIsLeased).
func (c *Client) DeleteRemoteBranch(ctx context.Context, remote, branch, expected string) error {
	ref := "refs/heads/" + branch
	// Retried like Push: only when the connection never opened.
	return c.retry(ctx, transientPush, "push", "--quiet", "--force-with-lease="+ref+":"+expected, remote, "--delete", ref)
}

func splitNul(out []byte) []string {
	var items []string
	for _, item := range strings.Split(string(out), "\x00") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// reflogMessage marks every ref change made by ship-it in the user's reflog.
const reflogMessage = "ship: undo"

// AttachHead implements ports.Git.
func (c *Client) AttachHead(ctx context.Context, branch string) error {
	_, err := c.Run(ctx, "symbolic-ref", "-m", reflogMessage, "HEAD", "refs/heads/"+branch)
	return err
}

// DetachHead implements ports.Git.
func (c *Client) DetachHead(ctx context.Context, commit string) error {
	_, err := c.Run(ctx, "update-ref", "--no-deref", "-m", reflogMessage, "HEAD", commit)
	return err
}

// MoveBranch implements ports.Git.
func (c *Client) MoveBranch(ctx context.Context, branch, commit, expected string) error {
	_, err := c.Run(ctx, "update-ref", "-m", reflogMessage, "refs/heads/"+branch, commit, expected)
	return err
}

// DeleteBranch implements ports.Git.
func (c *Client) DeleteBranch(ctx context.Context, branch, expected string) error {
	if _, err := c.Run(ctx, "update-ref", "-m", reflogMessage, "-d", "refs/heads/"+branch, expected); err != nil {
		return err
	}
	// Like `git branch -D`, drop the branch's settings (upstream…); a branch
	// without any has no section, which is fine.
	_, _ = c.Run(ctx, "config", "--remove-section", "branch."+branch)
	return nil
}

// RenameBranch implements ports.Git with `git branch -m`, which moves the
// reflog and the branch.<name>.* settings along and refuses an existing
// target. The expected commit is checked first: a branch someone moved is
// not renamed.
func (c *Client) RenameBranch(ctx context.Context, from, to, expected string) error {
	current, err := c.CommitOf(ctx, "refs/heads/"+from)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("gitexec: %s is at %s, not %s", from, current, expected)
	}
	_, err = c.Run(ctx, "branch", "-m", from, to)
	return err
}

// ResetIndex implements ports.Git. read-tree leaves the working tree alone;
// the refresh only updates cached stat data, and its exit status 1 (files
// that differ) is expected.
func (c *Client) ResetIndex(ctx context.Context, commit string) error {
	args := []string{"read-tree", commit}
	if commit == "" {
		args = []string{"read-tree", "--empty"}
	}
	if _, err := c.Run(ctx, args...); err != nil {
		return err
	}
	if _, err := c.Run(ctx, "update-index", "-q", "--refresh"); err != nil {
		if gitErr, ok := err.(*Error); !ok || gitErr.ExitCode != 1 {
			return err
		}
	}
	return nil
}

// CommitPaths implements ports.Git. Paths are relative to the top level: the
// ":/" magic makes them so wherever ship runs from. `git commit -- <paths>`
// commits those paths only (--only is the default with paths), new files
// included once added.
func (c *Client) CommitPaths(ctx context.Context, message string, paths []string, opts ports.CommitOptions) (string, error) {
	specs := topSpecs(paths)
	if _, err := c.Run(ctx, append([]string{"add", "--all", "--"}, specs...)...); err != nil {
		return "", err
	}
	args := append(append([]string{"commit", "--quiet", "--file=-"}, commitArgs(opts)...), "--")
	args = append(args, specs...)
	if _, err := c.RunWithInput(ctx, nil, []byte(message), args...); err != nil {
		return "", err
	}
	out, err := c.Run(ctx, "rev-parse", "HEAD")
	return strings.TrimSpace(string(out)), err
}

// ScopedConfigValue implements ports.Git.
func (c *Client) ScopedConfigValue(ctx context.Context, key string, global bool) (string, bool, error) {
	out, err := c.Run(ctx, "config", scope(global), "--get", key)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimRight(string(out), "\r\n"), true, nil
}

// SetConfig implements ports.Git.
func (c *Client) SetConfig(ctx context.Context, key, value string, global bool) error {
	defer c.forgetRemotes()
	_, err := c.Run(ctx, "config", scope(global), key, value)
	return err
}

// UnsetConfig implements ports.Git; exit status 5 means "not set", fine.
func (c *Client) UnsetConfig(ctx context.Context, key string, global bool) error {
	defer c.forgetRemotes()
	_, err := c.Run(ctx, "config", scope(global), "--unset-all", key)
	if gitErr, ok := err.(*Error); ok && gitErr.ExitCode == 5 {
		return nil
	}
	return err
}

func scope(global bool) string {
	if global {
		return "--global"
	}
	return "--local"
}

// TrackedFiles implements ports.Git.
func (c *Client) TrackedFiles(ctx context.Context) ([]string, error) {
	out, err := c.Run(ctx, "-c", "core.quotePath=false", "ls-files", "-z", "--full-name", "--", ":/")
	return splitNul(out), err
}

// Untrack implements ports.Git.
func (c *Client) Untrack(ctx context.Context, paths []string) error {
	_, err := c.Run(ctx, append([]string{"rm", "-r", "--cached", "--quiet", "--"}, topSpecs(paths)...)...)
	return err
}

// Track implements ports.Git.
func (c *Client) Track(ctx context.Context, paths []string) error {
	_, err := c.Run(ctx, append([]string{"add", "--force", "--"}, topSpecs(paths)...)...)
	return err
}

// topSpecs turns paths relative to the top level into pathspecs that mean
// the same wherever ship runs from.
func topSpecs(paths []string) []string {
	specs := make([]string, len(paths))
	for i, p := range paths {
		specs[i] = ":/" + strings.TrimPrefix(filepath.ToSlash(p), "/")
	}
	return specs
}

// WriteWorkingFile implements ports.Git. Missing parent directories (such as
// .vscode) are created.
func (c *Client) WriteWorkingFile(ctx context.Context, path string, content []byte) error {
	full, err := c.workingPath(ctx, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, content, 0o644)
}

// RemoveWorkingFile implements ports.Git. Parent directories left empty are
// removed too, up to the top level, so that undoing the creation of
// .vscode/mcp.json leaves no empty .vscode behind.
func (c *Client) RemoveWorkingFile(ctx context.Context, path string) error {
	full, err := c.workingPath(ctx, path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	top, err := c.TopLevel(ctx)
	if err != nil {
		return err
	}
	for dir := filepath.Dir(full); dir != filepath.Clean(top) && len(dir) > len(filepath.Clean(top)); dir = filepath.Dir(dir) {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) > 0 || os.Remove(dir) != nil {
			break
		}
	}
	return nil
}

// workingPath resolves a path of the working tree, refusing to leave it.
func (c *Client) workingPath(ctx context.Context, path string) (string, error) {
	top, err := c.TopLevel(ctx)
	if err != nil {
		return "", err
	}
	full := filepath.Join(top, filepath.FromSlash(path))
	if rel, err := filepath.Rel(top, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("gitexec: %s is outside the working tree", path)
	}
	return full, nil
}
