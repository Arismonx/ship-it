// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/forge"
)

func TestRepositoryOfAFork(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/ada/shop":
			fmt.Fprint(w, `{"default_branch":"main","fork":true,"parent":{"name":"shop","owner":{"login":"acme"},
				"default_branch":"trunk","clone_url":"https://github.com/acme/shop.git","ssh_url":"git@github.com:acme/shop.git"}}`)
		case "/repos/acme/shop":
			fmt.Fprint(w, `{"default_branch":"trunk","fork":false}`)
		case "/repos/acme/shop/pulls":
			if got := r.URL.Query().Get("head"); got != "ada:feat/vat" {
				t.Errorf("head filter = %q, want the fork's owner", got)
			}
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	info, err := c.Repository(ctx, forge.Repository{Host: "github.com", Owner: "ada", Name: "shop"})
	if err != nil || info.Parent == nil {
		t.Fatalf("Repository = %+v, %v", info, err)
	}
	p := info.Parent
	if p.Repository != (forge.Repository{Host: "github.com", Owner: "acme", Name: "shop"}) || p.DefaultBranch != "trunk" ||
		p.CloneURL != "https://github.com/acme/shop.git" || p.SSHURL != "git@github.com:acme/shop.git" {
		t.Errorf("parent = %+v", p)
	}
	if info, err := c.Repository(ctx, repo); err != nil || info.Parent != nil {
		t.Errorf("a repository that is no fork has no parent: %+v, %v", info, err)
	}
	if _, found, err := c.FindOpenPullRequest(ctx, repo, "ada:feat/vat"); found || err != nil {
		t.Errorf("FindOpenPullRequest = %v, %v", found, err)
	}
}
