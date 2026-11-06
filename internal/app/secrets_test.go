// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/secrets"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

// Assembled at run time so that this file never trips the scanner itself.
var testAWSKey = strings.Join([]string{"AK", "IA", "IOSFODNN7ABCDEFG"}, "")

func added(path string, lines ...string) snapshot.FileDiff {
	d := snapshot.FileDiff{Path: path, Status: snapshot.Modified}
	for i, l := range lines {
		d.Added = append(d.Added, snapshot.Line{Number: i + 1, Text: l})
	}
	return d
}

func secretsWorkspace(repository string) Workspace {
	return Workspace{Git: testkit.HealthyRepository("main"), Config: &testkit.FakeConfigFiles{Repository: repository}}
}

func ruleIDs(views []SecretView) []string {
	var out []string
	for _, v := range views {
		out = append(out, v.Rule)
	}
	return out
}

func TestScanHonoursTheSecretsSettings(t *testing.T) {
	ctx := context.Background()
	acme := "acme_" + strings.Repeat("0a", 12)
	diffs := []snapshot.FileDiff{added("a.go", "k = "+testAWSKey, "t = "+acme), added("deploy/id_ed25519")}

	views, err := scanSecrets(ctx, secretsWorkspace(""), diffs)
	if err != nil || strings.Join(ruleIDs(views), " ") != "aws_key sensitive_file" {
		t.Fatalf("defaults: %v, %v", ruleIDs(views), err)
	}
	if views[0].Fingerprint != secrets.Fingerprint(secrets.AWSKey, testAWSKey) || strings.Contains(views[0].Fingerprint, testAWSKey) {
		t.Fatalf("fingerprint = %q", views[0].Fingerprint)
	}

	ws := secretsWorkspace(`{"ship": {"secrets": {
		"rules": [{"id": "acme_token", "pattern": "acme_[0-9a-f]{24}", "keywords": ["acme_"]}],
		"disabledRules": ["sensitive_file"],
		"allow": ["` + secrets.Fingerprint(secrets.AWSKey, testAWSKey) + `"]}}}`)
	views, err = scanSecrets(ctx, ws, diffs)
	if err != nil || strings.Join(ruleIDs(views), " ") != "acme_token" {
		t.Fatalf("configured: %v, %v", ruleIDs(views), err)
	}
}

// An agent's exemptions (ignorePaths, disabledRules, allow) are those of
// .github/ship.json as committed in HEAD only: the working tree, the user
// file and SHIP_* cannot weaken its scan. Extra rules apply from any layer.
func TestAnAgentCannotWeakenItsOwnScan(t *testing.T) {
	ctx := context.Background()
	exemptions := `{"secrets": {"ignorePaths": ["legacy/**"], "disabledRules": ["sensitive_file"],
		"allow": ["` + secrets.Fingerprint(secrets.AWSKey, testAWSKey) + `"]}}`
	settings := `{"ship": ` + exemptions + `}`
	diffs := []snapshot.FileDiff{added("a.go", "k = "+testAWSKey), added("legacy/b.go", "k = "+testAWSKey), added(".env")}
	const strict = "aws_key aws_key sensitive_file"

	agentWith := func(working, committed string) Workspace {
		ws := secretsWorkspace(working)
		git := testkit.HealthyRepository("main")
		if committed != "" {
			git.Files = map[string]string{ports.RepositoryConfigPath: committed}
		}
		ws.Git, ws.Agent = git, true
		return ws
	}
	scan := func(name string, ws Workspace, want string) {
		t.Helper()
		if views, err := scanSecrets(ctx, ws, diffs); err != nil || strings.Join(ruleIDs(views), " ") != want {
			t.Errorf("%s: %v, %v", name, ruleIDs(views), err)
		}
	}
	scan("committed and in the working tree", agentWith(settings, settings), "")
	scan("in the working tree only (changed by the agent)", agentWith(settings, ""), strict)
	scan("committed file unreadable", agentWith(settings, `{"ship": `), strict)

	user := agentWith("", "")
	user.Config = &testkit.FakeConfigFiles{User: settings}
	scan("user file", user, strict)
	env := agentWith("", "")
	env.Environ = []string{`SHIP_SECRETS_DISABLEDRULES=sensitive_file`, `SHIP_SECRETS_IGNOREPATHS=legacy/**`}
	scan("SHIP_*", env, strict)

	// The committed file's overrides that match the delivery count too.
	override := `{"overrides": [{"paths": ["**"], "ship": ` + exemptions + `}]}`
	scan("committed override", agentWith(override, override), "")

	// Extra rules apply from any layer: they only add checks.
	acme := "acme_" + strings.Repeat("0a", 12)
	extra := agentWith("", settings)
	extra.Config = &testkit.FakeConfigFiles{User: `{"ship": {"secrets": {"rules": [{"id": "acme_token", "pattern": "acme_[0-9a-f]{24}", "keywords": ["acme_"]}]}}}`}
	if views, err := scanSecrets(ctx, extra, []snapshot.FileDiff{added("c.go", "t = "+acme)}); err != nil || strings.Join(ruleIDs(views), " ") != "acme_token" {
		t.Errorf("extra rule from the user file: %v, %v", ruleIDs(views), err)
	}

	human := secretsWorkspace(settings)
	if views, err := scanSecrets(ctx, human, diffs); err != nil || len(views) != 0 {
		t.Fatalf("a human gets every layer's exemptions: %v, %v", ruleIDs(views), err)
	}
}

func TestDeliveryWithAnAllowedFinding(t *testing.T) {
	git := deliverableGit("main", nil)
	git.DiffText = "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1,2 @@\n package a\n+aws_access_key_id = " + testAWSKey + "\n"
	ws, _, _ := workspaceOf(git)
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"}); err == nil {
		t.Fatal("the key must be refused by default")
	} else if found, _ := SecretsIn(err); len(found) != 1 || found[0].Fingerprint == "" {
		t.Fatalf("findings = %+v", found)
	}
	ws.Config = &testkit.FakeConfigFiles{Repository: `{"ship": {"secrets": {"allow": ["` + secrets.Fingerprint(secrets.AWSKey, testAWSKey) + `"]}}}`}
	if _, err := DeliverLocally(context.Background(), ws, DeliveryRequest{Title: "feat: add login", Branch: "feat/add-login"}); err != nil {
		t.Fatalf("an allowed finding ships: %v", err)
	}
}
