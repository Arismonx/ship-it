// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/fsconfig"
	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// labelServer keeps the labels of acme/shop, starting from GitHub's defaults.
type labelServer struct {
	mu     sync.Mutex
	labels []map[string]string
	writes int
}

func newLabelServer() *labelServer {
	s := &labelServer{}
	for _, name := range []string{"bug", "documentation", "duplicate", "enhancement", "good first issue", "help wanted", "invalid", "question", "wontfix"} {
		s.labels = append(s.labels, map[string]string{"name": name, "color": "ededed", "description": ""})
	}
	return s
}

func (s *labelServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var in map[string]string
	_ = json.NewDecoder(r.Body).Decode(&in)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/shop/labels":
		_ = json.NewEncoder(w).Encode(s.labels)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/shop/labels":
		s.writes++
		s.labels = append(s.labels, in)
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/repos/acme/shop/labels/"):
		s.writes++
		name, _ := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), "/repos/acme/shop/labels/"))
		for _, l := range s.labels {
			if l["name"] == name {
				l["name"], l["color"], l["description"] = in["new_name"], in["color"], in["description"]
			}
		}
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func (s *labelServer) names() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := map[string]bool{}
	for _, l := range s.labels {
		names[l["name"]] = true
	}
	return names
}

func TestInitConfigAndSync(t *testing.T) {
	api := newLabelServer()
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	home := t.TempDir()
	policyPath := filepath.Join(home, "system", "policy.json")
	s.deps.Config = &fsconfig.Files{Git: s.git, Env: platform.Environment{GOOS: runtime.GOOS, Home: home, PolicyPath: policyPath,
		Getenv: func(k string) string {
			if k == "AppData" || k == "XDG_CONFIG_HOME" {
				return filepath.Join(home, "config")
			}
			return ""
		}}}
	s.sh("remote", "add", "origin", "git@github.com:acme/shop.git")
	for _, subject := range []string{"feat(api): a", "fix(api): b", "feat(api): c", "docs: d"} {
		s.sh("commit", "--quiet", "--allow-empty", "-m", subject)
	}

	if code, _, stderr := s.ship("init"); code != cli.ExitOK || !strings.Contains(stderr, "area/ labels from your commit scopes: api") {
		t.Fatalf("ship init: exit %d\n%s", code, stderr)
	}
	written, err := os.ReadFile(filepath.Join(s.dir, ".github", "ship.json"))
	if err != nil || !strings.Contains(string(written), `"areas": ["api"]`) {
		t.Fatalf(".github/ship.json:\n%s (%v)", written, err)
	}
	if code, _, stderr := s.ship("init"); code != cli.ExitError || !strings.Contains(stderr, "SHIP-E0706") {
		t.Fatalf("second ship init must refuse: exit %d\n%s", code, stderr)
	}

	code, stdout, _ := s.ship("config")
	if code != cli.ExitOK || !strings.Contains(stdout, `github.labels.areas`) || !strings.Contains(stdout, `["api"]  ← repository`) {
		t.Fatalf("ship config:\n%s", stdout)
	}

	// Without a terminal and without --yes, nothing is applied.
	code, stdout, _ = s.ship("sync")
	if code != cli.ExitNeedsHuman || api.writes != 0 || !strings.Contains(stdout, "~ rename  bug → kind/bug") {
		t.Fatalf("ship sync without consent: exit %d, writes %d\n%s", code, api.writes, stdout)
	}
	if code, _, stderr := s.ship("sync", "--yes"); code != cli.ExitOK || !strings.Contains(stderr, "changes applied") {
		t.Fatalf("ship sync --yes: exit %d\n%s", code, stderr)
	}
	names := api.names()
	for _, want := range []string{"kind/bug", "kind/feature", "triage/wontfix", "size/XXL", "area/api", "good first issue", "help wanted"} {
		if !names[want] {
			t.Errorf("label %q missing after sync", want)
		}
	}
	if names["bug"] || names["enhancement"] {
		t.Error("GitHub default labels must be renamed, not duplicated")
	}
	if code, stdout, _ := s.ship("sync"); code != cli.ExitOK || !strings.Contains(stdout, "already matches") {
		t.Fatalf("second ship sync: exit %d\n%s", code, stdout)
	}

	// A broken file stops ship before anything changes, naming file and line.
	if err := os.WriteFile(filepath.Join(s.dir, ".github", "ship.json"), []byte("{\n  \"ship\": {\"publish\": {\"mode\": \"maybe\"}}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := s.ship("--yes"); code != cli.ExitError || !strings.Contains(stderr, "SHIP-E0704") || !strings.Contains(stderr, "ship.publish.mode cannot be \"maybe\"") {
		t.Fatalf("broken config: exit %d\n%s", code, stderr)
	}

	// The organisation imposes ship.publish.mode; the repository says otherwise:
	// refused with exit code 5, naming who to ask.
	if err := os.WriteFile(filepath.Join(s.dir, ".github", "ship.json"), []byte(`{"ship": {"publish": {"mode": "auto"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	policy := `{"organization": "Acme", "contact": "dsi@acme.example", "enforce": {"ship": {"publish": {"mode": "never"}}}}`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := s.ship("--yes")
	if code != cli.ExitPolicy || !strings.Contains(stderr, "Acme imposes ship.publish.mode") || !strings.Contains(stderr, "dsi@acme.example") {
		t.Fatalf("policy conflict: exit %d\n%s", code, stderr)
	}
	code, stdout, _ = s.ship("doctor")
	if code != cli.ExitError || !strings.Contains(stdout, "SHIP-E0801") && !strings.Contains(stdout, "Acme imposes") {
		t.Fatalf("doctor with a policy conflict: exit %d\n%s", code, stdout)
	}
}
