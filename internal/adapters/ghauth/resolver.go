// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package ghauth implements ports.Credentials: it finds a GitHub token the
// user already has, so that most people never log in to ship-it (ADR-0003).
//
// Sources, first match wins:
//  1. SHIP_TOKEN;
//  2. GH_TOKEN then GITHUB_TOKEN for github.com and *.ghe.com,
//     GH_ENTERPRISE_TOKEN then GITHUB_ENTERPRISE_TOKEN for Enterprise Server
//     (the variables and precedence of the GitHub CLI);
//  3. `gh auth token --hostname <host>`, when gh is installed;
//  4. git's credential helpers (`git credential fill`), never prompting:
//     GIT_TERMINAL_PROMPT=0 and GCM_INTERACTIVE=false.
//
// The OS keychain filled by `ship login` will slot in before step 4.
package ghauth

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/GregoireF/ship-it/internal/ports"
)

// lookupTimeout bounds each external lookup (gh, credential helper).
const lookupTimeout = 10 * time.Second

// GitRunner runs git with extra environment and standard input. It is
// declared here so that adapters never import each other.
type GitRunner interface {
	RunWithInput(ctx context.Context, env []string, stdin []byte, args ...string) ([]byte, error)
}

// Resolver implements ports.Credentials. Tokens are looked up once per host
// and process.
type Resolver struct {
	Git GitRunner
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
	// GitHubCLI runs gh and returns its standard output; it defaults to the gh
	// binary on PATH, and returns an error when gh is missing.
	GitHubCLI func(ctx context.Context, args ...string) ([]byte, error)

	mu    sync.Mutex
	cache map[string]ports.Token
}

var _ ports.Credentials = (*Resolver)(nil)

// New returns a Resolver using git for credential helpers.
func New(git GitRunner) *Resolver { return &Resolver{Git: git} }

// Token implements ports.Credentials.
func (r *Resolver) Token(ctx context.Context, host string) (ports.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token, ok := r.cache[host]; ok {
		return token, nil
	}
	token, ok := r.lookup(ctx, host)
	if !ok {
		return ports.Token{}, ports.ErrNotAuthenticated
	}
	if r.cache == nil {
		r.cache = map[string]ports.Token{}
	}
	r.cache[host] = token
	return token, nil
}

var _ ports.TokenForgetter = (*Resolver)(nil)

// Forget implements ports.TokenForgetter.
func (r *Resolver) Forget(host string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, host)
}

func (r *Resolver) lookup(ctx context.Context, host string) (ports.Token, bool) {
	variables := []string{"SHIP_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}
	if host != "github.com" && !strings.HasSuffix(host, ".ghe.com") {
		variables = []string{"SHIP_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}
	}
	for _, name := range variables {
		if value := strings.TrimSpace(r.getenv(name)); value != "" {
			return ports.NewToken(value, name), true
		}
	}

	lookupCtx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	if out, err := r.gitHubCLI(lookupCtx, "auth", "token", "--hostname", host); err == nil {
		if value := strings.TrimSpace(string(out)); value != "" {
			return ports.NewToken(value, "gh"), true
		}
	}

	if r.Git != nil {
		request := "protocol=https\nhost=" + host + "\n\n"
		env := []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=false"}
		if out, err := r.Git.RunWithInput(lookupCtx, env, []byte(request), "credential", "fill"); err == nil {
			if value := credentialPassword(out); value != "" {
				return ports.NewToken(value, "git credential helper"), true
			}
		}
	}
	return ports.Token{}, false
}

// credentialPassword reads the password attribute of `git credential fill`.
func credentialPassword(out []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "password="); ok {
			return value
		}
	}
	return ""
}

func (r *Resolver) getenv(name string) string {
	if r.Getenv != nil {
		return r.Getenv(name)
	}
	return os.Getenv(name)
}

func (r *Resolver) gitHubCLI(ctx context.Context, args ...string) ([]byte, error) {
	if r.GitHubCLI != nil {
		return r.GitHubCLI(ctx, args...)
	}
	path, err := exec.LookPath("gh")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	return cmd.Output()
}
