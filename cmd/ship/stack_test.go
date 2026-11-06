// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// fakePull is a pull request of fakeStackHub.
type fakePull struct {
	Head, Base, State, Body string
	Draft, Merged           bool
}

// fakeStackHub answers the pull request, stacks and merge calls of ship
// stack, following the shapes of GitHub's REST API version 2026-03-10.
type fakeStackHub struct {
	mu       sync.Mutex
	native   bool // false: no stacks API, as on GHES
	pulls    []*fakePull
	stack    []int // the pull requests of stack #1, bottom first
	requests []string
}

func (f *fakeStackHub) pullJSON(n int) string {
	p := f.pulls[n-1]
	merged := "null"
	if p.Merged {
		merged = `"2026-11-05T10:00:00Z"`
	}
	return fmt.Sprintf(`{"number":%d,"html_url":"https://github.com/acme/shop/pull/%d","draft":%v,"state":%q,"merged_at":%s,"head":{"ref":%q,"sha":"x"},"base":{"ref":%q}}`,
		n, n, p.Draft, p.State, merged, p.Head, p.Base)
}

func (f *fakeStackHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/repos/acme/shop")
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	if r.Method != http.MethodGet {
		data, _ := json.Marshal(in)
		f.requests = append(f.requests, r.Method+" "+path+" "+string(data))
	}
	number := func(prefix string) int {
		n, _ := strconv.Atoi(strings.SplitN(strings.TrimPrefix(path, prefix), "/", 2)[0])
		return n
	}
	switch {
	case path == "" && r.Method == http.MethodGet:
		fmt.Fprint(w, `{"default_branch":"main","html_url":"https://github.com/acme/shop"}`)
	case path == "/pulls" && r.Method == http.MethodGet:
		var found []string
		for i := len(f.pulls); i >= 1; i-- { // newest first
			p := f.pulls[i-1]
			if "acme:"+p.Head == r.URL.Query().Get("head") && (r.URL.Query().Get("state") != "open" || p.State == "open") {
				found = append(found, f.pullJSON(i))
			}
		}
		fmt.Fprint(w, "["+strings.Join(found, ",")+"]")
	case path == "/pulls" && r.Method == http.MethodPost:
		f.pulls = append(f.pulls, &fakePull{Head: in["head"].(string), Base: in["base"].(string), State: "open",
			Draft: in["draft"] == true, Body: in["body"].(string)})
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, f.pullJSON(len(f.pulls)))
	case strings.HasSuffix(path, "/merge-async") && r.Method == http.MethodPut:
		n := number("/pulls/")
		for _, m := range f.stack {
			f.pulls[m-1].Merged, f.pulls[m-1].State = true, "closed"
			if m == n {
				break
			}
		}
		fmt.Fprint(w, `{"status":"merged","details":{"message":"Merged","sha":"m"}}`)
	case strings.HasPrefix(path, "/pulls/") && r.Method == http.MethodPatch:
		n := number("/pulls/")
		if s, ok := in["state"].(string); ok {
			f.pulls[n-1].State = s
		}
		if b, ok := in["base"].(string); ok {
			f.pulls[n-1].Base = b
		}
		fmt.Fprint(w, f.pullJSON(n))
	case strings.HasPrefix(path, "/issues/") && strings.HasSuffix(path, "/comments"):
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	case strings.HasPrefix(path, "/stacks") && !f.native:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	case path == "/stacks" && r.Method == http.MethodGet:
		if len(f.stack) == 0 {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprint(w, "["+f.stackJSON()+"]")
	case path == "/stacks" && r.Method == http.MethodPost:
		for _, n := range in["pull_requests"].([]any) {
			f.stack = append(f.stack, int(n.(float64)))
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, f.stackJSON())
	case path == "/stacks/1/unstack":
		f.stack = nil
		w.WriteHeader(http.StatusNoContent)
	case strings.Contains(path, "/check-runs"):
		fmt.Fprint(w, `{"check_runs":[]}`)
	case strings.HasSuffix(path, "/status"):
		fmt.Fprint(w, `{"statuses":[]}`)
	case path == "/labels":
		fmt.Fprint(w, `[]`)
	default:
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func (f *fakeStackHub) stackJSON() string {
	prs := make([]string, len(f.stack))
	for i, n := range f.stack {
		prs[i] = f.pullJSON(n)
	}
	return `{"number":1,"open":true,"base":{"ref":"main"},"pull_requests":[` + strings.Join(prs, ",") + `]}`
}

// stackScenario is a repository whose origin is a local bare repository
// behind a github.com URL, and whose API is hub.
func stackScenario(t *testing.T, hub *fakeStackHub) (*scenario, string) {
	server := httptest.NewServer(hub)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL
	s := newScenario(t, forge)
	s.deps.Stacks = forge
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", bare)
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("fetch", "--quiet", "origin")
	s.sh("remote", "set-head", "origin", "main")
	for name, content := range map[string]string{
		"api/cart.go":  "package api\n\n// Cart holds items.\ntype Cart struct{ Items []string }\n\nfunc (c Cart) Len() int { return len(c.Items) }\n",
		"web/cart.ts":  "export const cart: string[] = []\nexport const size = () => cart.length\n",
		"docs/cart.md": "# Cart\n",
	} {
		if err := os.MkdirAll(filepath.Join(s.dir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		s.write(name, content)
	}
	return s, bare
}

// TestStackEndToEnd ships a stack of three layers, merges it, syncs and
// undoes, against real git and a fake GitHub with native stacks.
func TestStackEndToEnd(t *testing.T) {
	hub := &fakeStackHub{native: true}
	s, bare := stackScenario(t, hub)
	// The local rebase, which this test follows; github is the default.
	s.deps.SettingFlags = []string{"ship.stack.rebase=local"}

	code, stdout, stderr := s.ship("stack", "--yes")
	if code != cli.ExitOK {
		t.Fatalf("ship stack --yes: exit %d\n%s\n%s", code, stdout, stderr)
	}
	branches := strings.Fields(s.sh("for-each-ref", "--format=%(refname:short)", "--sort=committerdate", "refs/heads/"))
	if len(hub.pulls) != 3 || len(hub.stack) != 3 || !strings.Contains(stderr, "Stack #1 of 3 pull requests") {
		t.Fatalf("pulls %+v, stack %v\n%s", hub.pulls, hub.stack, stderr)
	}
	for i, p := range hub.pulls {
		want := "main"
		if i > 0 {
			want = hub.pulls[i-1].Head
		}
		if p.Base != want || !p.Draft {
			t.Errorf("pull request %d: base %s, want %s (draft %v: --yes means unattended)", i+1, p.Base, want, p.Draft)
		}
		if parent := s.sh("rev-parse", p.Head+"~1"); parent != s.sh("rev-parse", want) {
			t.Errorf("%s is not based on %s", p.Head, want)
		}
		if pushed := s.sh("--git-dir", bare, "rev-parse", "refs/heads/"+p.Head); pushed != s.sh("rev-parse", p.Head) {
			t.Errorf("%s not pushed", p.Head)
		}
	}
	top := hub.pulls[2].Head
	if current := s.sh("branch", "--show-current"); current != top || len(branches) != 4 {
		t.Errorf("on %s, branches %v", current, branches)
	}

	if code, stdout, _ := s.ship("stack", "status"); code != cli.ExitOK || !strings.Contains(stdout, "Stack #1 on main") || !strings.Contains(stdout, "#3 draft") {
		t.Errorf("ship stack status: exit %d\n%s", code, stdout)
	}

	// The merge needs a human: without a terminal nor --yes it is refused.
	if code, _, stderr := s.ship("stack", "merge"); code != cli.ExitNeedsHuman || !strings.Contains(stderr, "SHIP-E1006") {
		t.Fatalf("ship stack merge without consent: exit %d\n%s", code, stderr)
	}
	bottom := mergeBottom(s, hub)

	code, _, stderr = s.ship("stack", "sync")
	if code != cli.ExitOK {
		t.Fatalf("ship stack sync: exit %d\n%s", code, stderr)
	}
	if s.sh("branch", "--list", bottom) != "" || s.sh("--git-dir", bare, "branch", "--list", bottom) != "" {
		t.Errorf("the merged layer %s must be gone here and on GitHub", bottom)
	}
	middle := hub.pulls[1].Head
	if parent := s.sh("rev-parse", middle+"~1"); parent != s.sh("rev-parse", "origin/main") {
		t.Errorf("%s not rebased on main", middle)
	}
	if parent := s.sh("rev-parse", top+"~1"); parent != s.sh("rev-parse", middle) {
		t.Errorf("%s not rebased on %s", top, middle)
	}
	if pushed := s.sh("--git-dir", bare, "rev-parse", "refs/heads/"+top); pushed != s.sh("rev-parse", top) {
		t.Error("the rebased top layer was not pushed")
	}

	// The whole stack merges at once, from its top.
	if code, _, stderr := s.ship("stack", "merge", "--yes"); code != cli.ExitOK || !strings.Contains(stderr, "Merged into main") {
		t.Fatalf("ship stack merge --yes: exit %d\n%s", code, stderr)
	}
	if last := hub.requests[len(hub.requests)-1]; !strings.HasPrefix(last, "PUT /pulls/3/merge-async") || !strings.Contains(last, `"bypass_rules":false`) {
		t.Errorf("merge request = %s", last)
	}
}

// TestStackUndoEndToEnd undoes a published stack: the pull requests are
// closed, the stack dissolved, the remote branches deleted, then the layers
// go back to uncommitted work.
func TestStackUndoEndToEnd(t *testing.T) {
	hub := &fakeStackHub{native: true}
	s, bare := stackScenario(t, hub)
	if code, _, stderr := s.ship("stack", "--yes"); code != cli.ExitOK {
		t.Fatalf("ship stack: %s", stderr)
	}
	if code, _, stderr := s.ship("undo"); code != cli.ExitNeedsHuman || !strings.Contains(stderr, "closes the 3 pull requests") {
		t.Fatalf("undo without consent: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := s.ship("undo", "--yes"); code != cli.ExitOK || !strings.Contains(stderr, "3 pull requests closed") {
		t.Fatalf("undo --yes: exit %d\n%s", code, stderr)
	}
	for _, p := range hub.pulls {
		if p.State != "closed" {
			t.Errorf("%s still open", p.Head)
		}
		if s.sh("--git-dir", bare, "branch", "--list", p.Head) != "" {
			t.Errorf("remote %s not deleted", p.Head)
		}
	}
	if hub.stack != nil {
		t.Error("the stack was not dissolved")
	}
	if code, _, stderr := s.ship("undo"); code != cli.ExitOK {
		t.Fatalf("undo of the layers: exit %d\n%s", code, stderr)
	}
	if current := s.sh("branch", "--show-current"); current != "main" || s.sh("status", "--porcelain") == "" {
		t.Errorf("on %s, work: %q", current, s.sh("status", "--porcelain"))
	}
	if branches := s.sh("branch", "--list"); strings.Count(branches, "\n") != 0 {
		t.Errorf("the layers' branches must be gone:\n%s", branches)
	}
}

// TestStackChainOnGHES: without a stacks API the same command opens a
// classic chain whose bodies say where each pull request sits.
func TestStackChainOnGHES(t *testing.T) {
	hub := &fakeStackHub{}
	s, _ := stackScenario(t, hub)
	code, _, stderr := s.ship("stack", "--yes")
	if code != cli.ExitOK || !strings.Contains(stderr, "Chain of 3 pull requests") {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if !strings.HasSuffix(hub.pulls[2].Body, "Part 3 of 3, after #2.\n") || hub.pulls[2].Base != hub.pulls[1].Head {
		t.Errorf("top body:\n%s", hub.pulls[2].Body)
	}
}

// mergeBottom does what GitHub does when the bottom layer merges, squashed:
// one commit on main, the pull request merged, the others left open.
func mergeBottom(s *scenario, hub *fakeStackHub) string {
	bottom, top := hub.pulls[0].Head, hub.pulls[len(hub.pulls)-1].Head
	s.sh("switch", "--quiet", "main")
	s.sh("merge", "--squash", "--quiet", bottom)
	s.sh("commit", "--quiet", "-m", "✨ feat(api): add the cart (#1)")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("switch", "--quiet", top)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.pulls[0].Merged, hub.pulls[0].State = true, "closed"
	hub.stack = hub.stack[1:]
	return bottom
}

// TestStackSyncThenUndoEndToEnd: after a sync pushed rebased layers, undoing
// the sync puts the local and remote branches back, and undoing the
// publication then deletes the remote branches.
func TestStackSyncThenUndoEndToEnd(t *testing.T) {
	hub := &fakeStackHub{native: true}
	s, bare := stackScenario(t, hub)
	// The local rebase, which this test follows; github is the default.
	s.deps.SettingFlags = []string{"ship.stack.rebase=local"}
	if code, _, stderr := s.ship("stack", "--yes"); code != cli.ExitOK {
		t.Fatalf("ship stack: %s", stderr)
	}
	top := hub.pulls[2].Head
	before := s.sh("rev-parse", top)
	bottom := mergeBottom(s, hub)
	if code, _, stderr := s.ship("stack", "sync"); code != cli.ExitOK {
		t.Fatalf("sync: %s", stderr)
	}
	if s.sh("--git-dir", bare, "rev-parse", top) == before {
		t.Fatal("the sync must have pushed the rebased top layer")
	}
	// GitHub changes: consent first, then the remote is put back too.
	if code, _, stderr := s.ship("undo"); code != cli.ExitNeedsHuman || !strings.Contains(stderr, "SHIP-E0407") {
		t.Fatalf("undo of the sync without consent: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := s.ship("undo", "--yes"); code != cli.ExitOK || !strings.Contains(stderr, "Back on GitHub, under a lease") {
		t.Fatalf("undo of the sync: exit %d\n%s", code, stderr)
	}
	if s.sh("rev-parse", top) != before || s.sh("branch", "--list", bottom) == "" {
		t.Error("undoing the sync must put the local branches back")
	}
	if s.sh("--git-dir", bare, "rev-parse", top) != before || s.sh("--git-dir", bare, "branch", "--list", bottom) == "" {
		t.Error("undoing the sync must put the remote branches back")
	}
	if code, _, stderr := s.ship("undo", "--yes"); code != cli.ExitOK {
		t.Fatalf("undo of the publication: exit %d\n%s", code, stderr)
	}
	for _, p := range hub.pulls[1:] {
		if p.State != "closed" || s.sh("--git-dir", bare, "branch", "--list", p.Head) != "" {
			t.Errorf("%s: %s, remote branch kept", p.Head, p.State)
		}
	}
	if !hub.pulls[0].Merged {
		t.Error("the merged layer stays merged")
	}
}
