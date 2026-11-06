// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ghauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/ports"
)

type fakeGit struct {
	out   string
	input *string
}

func (g fakeGit) RunWithInput(_ context.Context, env []string, stdin []byte, args ...string) ([]byte, error) {
	if strings.Join(args, " ") != "credential fill" || !strings.Contains(strings.Join(env, " "), "GCM_INTERACTIVE=false") {
		return nil, fmt.Errorf("unexpected git %v %v", env, args)
	}
	if g.input != nil {
		*g.input = string(stdin)
	}
	if g.out == "" {
		return nil, errors.New("exit status 128")
	}
	return []byte(g.out), nil
}

func noGitHubCLI(context.Context, ...string) ([]byte, error) { return nil, errors.New("gh not found") }

func resolver(env map[string]string, gh func(context.Context, ...string) ([]byte, error), git GitRunner) *Resolver {
	return &Resolver{Git: git, Getenv: func(k string) string { return env[k] }, GitHubCLI: gh}
}

func TestSourcesInOrder(t *testing.T) {
	ctx := context.Background()
	gh := func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Join(args, " ") != "auth token --hostname github.com" {
			return nil, fmt.Errorf("unexpected gh %v", args)
		}
		return []byte("gho_fromgh\n"), nil
	}
	helper := fakeGit{out: "protocol=https\nhost=github.com\nusername=x\npassword=gho_fromhelper\n"}
	tests := []struct {
		name   string
		env    map[string]string
		gh     func(context.Context, ...string) ([]byte, error)
		source string
		value  string
	}{
		{"ship token wins", map[string]string{"SHIP_TOKEN": "s", "GH_TOKEN": "g"}, gh, "SHIP_TOKEN", "s"},
		{"GH_TOKEN before GITHUB_TOKEN", map[string]string{"GH_TOKEN": "g", "GITHUB_TOKEN": "h"}, gh, "GH_TOKEN", "g"},
		{"gh CLI", nil, gh, "gh", "gho_fromgh"},
		{"credential helper", nil, noGitHubCLI, "git credential helper", "gho_fromhelper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := resolver(tt.env, tt.gh, helper).Token(ctx, "github.com")
			if err != nil || token.Source != tt.source || token.Reveal() != tt.value {
				t.Fatalf("Token = %s/%q, %v", token.Source, token.Reveal(), err)
			}
		})
	}
}

func TestEnterpriseServerUsesEnterpriseVariables(t *testing.T) {
	var input string
	r := resolver(map[string]string{"GH_TOKEN": "dotcom", "GH_ENTERPRISE_TOKEN": "ghes"}, noGitHubCLI, fakeGit{input: &input})
	token, err := r.Token(context.Background(), "git.example.org")
	if err != nil || token.Reveal() != "ghes" {
		t.Fatalf("Token = %q, %v", token.Reveal(), err)
	}
	if _, err := resolver(map[string]string{"GH_TOKEN": "dotcom"}, noGitHubCLI, fakeGit{input: &input}).Token(context.Background(), "git.example.org"); !errors.Is(err, ports.ErrNotAuthenticated) {
		t.Fatalf("a github.com token must not be sent to another host: %v", err)
	}
	if input != "protocol=https\nhost=git.example.org\n\n" {
		t.Fatalf("credential request = %q", input)
	}
}

func TestNothingFound(t *testing.T) {
	_, err := resolver(nil, noGitHubCLI, fakeGit{}).Token(context.Background(), "github.com")
	if !errors.Is(err, ports.ErrNotAuthenticated) {
		t.Fatalf("error = %v", err)
	}
}

func TestTokenNeverPrints(t *testing.T) {
	token := ports.NewToken("ghp_secret", "GH_TOKEN")
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("auth", "token", token)
	for _, s := range []string{fmt.Sprint(token), fmt.Sprintf("%v %+v %#v %s", token, token, token, token), logged.String()} {
		if strings.Contains(s, "ghp_secret") {
			t.Fatalf("token leaked: %s", s)
		}
	}
}
