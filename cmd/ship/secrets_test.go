// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/fsconfig"
	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/core/secrets"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// TestShipRefusesSecrets: a real-looking key stops ship before the review,
// with exit code 3 and nothing committed; ship.secrets.ignorePaths lets a
// known false positive through.
func TestShipRefusesSecrets(t *testing.T) {
	s := newScenario(t, nil)
	s.deps.Config = &fsconfig.Files{Git: s.git, Env: platform.Environment{GOOS: "linux", Home: t.TempDir(),
		Getenv: func(string) string { return "" }, PolicyPath: filepath.Join(t.TempDir(), "none.json")}}
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	head := s.sh("rev-parse", "HEAD")

	// Assembled at run time so that this file never trips the scanner itself.
	key := strings.Join([]string{"AK", "IA", "IOSFODNN7ABCDEFG"}, "")
	if err := os.MkdirAll(filepath.Join(s.dir, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.write(filepath.Join("fixtures", "aws.txt"), "aws_access_key_id = "+key+"\n")

	code, _, stderr := s.ship("--yes", "--local")
	if code != cli.ExitGate || !strings.Contains(stderr, "SHIP-E0901") || !strings.Contains(stderr, "fixtures/aws.txt:1  AWS access key") {
		t.Fatalf("ship with a secret: exit %d\n%s", code, stderr)
	}
	if strings.Contains(stderr, key) {
		t.Fatal("the secret itself must never be printed")
	}
	if s.sh("rev-parse", "HEAD") != head || s.sh("branch", "--show-current") != "main" {
		t.Fatal("nothing may change when a secret is found")
	}

	// A known test fixture: declared as a false positive, it ships.
	if err := os.MkdirAll(filepath.Join(s.dir, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.write(filepath.Join(".github", "ship.json"), `{"ship": {"secrets": {"ignorePaths": ["fixtures/**"]}}}`)
	if code, _, stderr := s.ship("--yes", "--local"); code != cli.ExitOK {
		t.Fatalf("ship with ignorePaths: exit %d\n%s", code, stderr)
	}
}

// TestShipAcceptsAnAllowedFinding: each finding comes with its fingerprint
// (stderr and --json), never with its value; listed in ship.secrets.allow,
// the finding ships. A custom rule of the repository finds its own secrets.
func TestShipAcceptsAnAllowedFinding(t *testing.T) {
	s := newScenario(t, nil)
	s.deps.Config = &fsconfig.Files{Git: s.git, Env: platform.Environment{GOOS: "linux", Home: t.TempDir(),
		Getenv: func(string) string { return "" }, PolicyPath: filepath.Join(t.TempDir(), "none.json")}}
	if err := os.MkdirAll(filepath.Join(s.dir, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := `"rules": [{"id": "acme_token", "pattern": "\\bacme_[0-9a-f]{24}\\b", "keywords": ["acme_"]}]`
	s.write(filepath.Join(".github", "ship.json"), `{"ship": {"secrets": {`+rules+`}}}`)
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")

	key := strings.Join([]string{"AK", "IA", "IOSFODNN7ABCDEFG"}, "")
	token := strings.Join([]string{"acme", "_", strings.Repeat("0a", 12)}, "")
	s.write("aws.txt", "aws_access_key_id = "+key+"\n")
	s.write("acme.txt", "value: "+token+"\n")
	keyPrint, tokenPrint := secrets.Fingerprint(secrets.AWSKey, key), secrets.Fingerprint("acme_token", token)

	code, stdout, stderr := s.ship("--yes", "--local", "--json")
	if code != cli.ExitGate || !strings.Contains(stderr, "aws.txt:1  AWS access key") || !strings.Contains(stderr, "fingerprint "+keyPrint) ||
		!strings.Contains(stderr, "acme.txt:1  acme_token") || !strings.Contains(stdout, `"fingerprint": "`+tokenPrint+`"`) {
		t.Fatalf("ship with secrets: exit %d\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr+stdout, key) || strings.Contains(stderr+stdout, token) {
		t.Fatal("the secret itself must never be printed")
	}

	s.write(filepath.Join(".github", "ship.json"), `{"ship": {"secrets": {`+rules+`, "allow": ["`+keyPrint+`", "`+tokenPrint+`"]}}}`)
	if code, _, stderr := s.ship("--yes", "--local"); code != cli.ExitOK {
		t.Fatalf("ship with allowed findings: exit %d\n%s", code, stderr)
	}
}

func TestPublishRefusesASecretCommittedOutsideShip(t *testing.T) {
	api := &fakeGitHub{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	credentials := ghauth.New(nil)
	credentials.Getenv = func(name string) string { return map[string]string{"SHIP_TOKEN": "t0ken"}[name] }
	forge := github.New(credentials, "ship-it/test")
	forge.BaseURL = server.URL

	s := newScenario(t, forge)
	bare := filepath.Join(t.TempDir(), "shop.git")
	s.sh("init", "--bare", "--quiet", bare)
	s.sh("remote", "add", "origin", "https://github.com/acme/shop.git")
	s.sh("config", "url."+filepath.ToSlash(bare)+".insteadOf", "https://github.com/acme/shop.git")
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("push", "--quiet", "origin", "main")
	s.sh("fetch", "--quiet", bare, "main:refs/remotes/origin/main")

	// A plain git commit: the delivery's scan never saw it.
	token := strings.Join([]string{"gh", "p_", strings.Repeat("a1B2", 9)}, "")
	s.sh("switch", "--quiet", "-c", "feat/deploy")
	s.write("deploy.sh", "export GH_TOKEN="+token+"\n")
	s.sh("add", "deploy.sh")
	s.sh("commit", "--quiet", "-m", "✨ feat: add deploy script")

	code, _, stderr := s.ship("publish")
	if code != cli.ExitGate || !strings.Contains(stderr, "deploy.sh") || !strings.Contains(stderr, "not pushed yet") || strings.Contains(stderr, token) {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if out, _ := s.git.Run(t.Context(), "--git-dir", bare, "rev-parse", "--verify", "--quiet", "refs/heads/feat/deploy"); len(out) != 0 {
		t.Fatal("nothing may be pushed")
	}
}
