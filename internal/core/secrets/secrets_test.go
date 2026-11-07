// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package secrets

import (
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// Fake secrets are assembled at run time so that this file never trips the
// scanner when ship-it ships itself.
func fake(parts ...string) string { return strings.Join(parts, "") }

var (
	gitHubToken = fake("gh", "p_", strings.Repeat("a1B2", 9))
	awsKey      = fake("AK", "IA", "IOSFODNN7ABCDEFG")
	privateKey  = fake("-----BEGIN ", "RSA PRIVATE KEY-----")
	stripeKey   = fake("sk", "_live_", "4eC39HqLyjWDarjtT1zdp7dc")
)

func diff(path string, lines ...string) snapshot.FileDiff {
	f := snapshot.FileDiff{Path: path, Status: snapshot.Modified}
	for i, l := range lines {
		f.Added = append(f.Added, snapshot.Line{Number: i + 1, Text: l})
	}
	return f
}

func TestRules(t *testing.T) {
	tests := []struct {
		line string
		want RuleID // "" means no finding
	}{
		{`token := "` + gitHubToken + `"`, GitHubToken},
		{"aws_access_key_id = " + awsKey, AWSKey},
		{privateKey, PrivateKey},
		{"STRIPE=" + stripeKey, StripeKey},
		{"DATABASE_URL=postgres://admin:" + fake("s3cr", "etPass") + "@db.internal:5432/app", URLWithPassword},
		{`MISTRAL_API_KEY="` + fake("Zk8q", "Lw2vN9xT4pR7mY3s") + `"`, SecretAssignment},
		{`apiKey: "` + fake("3fa85f64", "5717c4562b3fc2c963f66afa") + `",`, SecretAssignment},
		// Not secrets: indirections, placeholders, code, constants.
		{`MISTRAL_API_KEY=${MISTRAL_API_KEY}`, ""},
		{`apiKey: process.env.STRIPE_SECRET_KEY_VALUE`, ""},
		{`password = "<your-password-here-123>"`, ""},
		{`token: config.public.apiTokenName1`, ""},
		{`const SECRET_HEADER_NAME_2024 = "X_SHIP_SECRET_HEADER_NAME"`, ""},
		{`secret := useToken(currentUser123)`, ""},
		{`password: "changeme-changeme-123"`, ""},
		{`apiKey: "xxxxxxxxxxxxxxxxxxxx"`, ""},
		{`see https://docs.example.com/token/settings`, ""},
	}
	for _, tt := range tests {
		got := Scan([]snapshot.FileDiff{diff("app/config.ts", tt.line)}, Options{})
		switch {
		case tt.want == "" && len(got) != 0:
			t.Errorf("false positive on %q: %+v", tt.line, got)
		case tt.want != "" && (len(got) != 1 || got[0].Rule != tt.want):
			t.Errorf("%q: findings %+v, want %s", tt.line, got, tt.want)
		}
	}
}

func TestFindingsNeverHoldTheSecret(t *testing.T) {
	got := Scan([]snapshot.FileDiff{diff("main.go", "x", `t := "`+gitHubToken+`"`)}, Options{})
	if len(got) != 1 || got[0].Line != 2 || got[0].Length != len(gitHubToken) {
		t.Fatalf("findings = %+v", got)
	}
	if strings.Contains(got[0].Preview, gitHubToken[6:30]) || got[0].Preview != gitHubToken[:4]+"…"+gitHubToken[len(gitHubToken)-2:] {
		t.Fatalf("preview leaks the secret: %q", got[0].Preview)
	}
}

func TestFilesAndIgnores(t *testing.T) {
	files := []snapshot.FileDiff{
		diff(".env.local", "A=1"),
		diff("deploy/id_ed25519"),
		diff(".env.example", "STRIPE="+stripeKey),
		diff("package-lock.json", `"integrity": "`+gitHubToken+`"`),
		diff("testdata/fixtures/keys.txt", awsKey),
		diff("legacy/old.go", awsKey),
		{Path: "removed.go", Status: snapshot.Deleted, Added: []snapshot.Line{{Number: 1, Text: awsKey}}},
	}
	got := Scan(files, Options{IgnorePaths: []string{"testdata/**", "legacy/*.go"}})
	if len(got) != 2 || got[0].Path != ".env.local" || got[0].Rule != SensitiveFile || got[0].Line != 0 || got[1].Path != "deploy/id_ed25519" {
		t.Fatalf("findings = %+v", got)
	}
}

func TestOneFindingPerLine(t *testing.T) {
	got := Scan([]snapshot.FileDiff{diff("a.go", awsKey+" "+gitHubToken)}, Options{})
	if len(got) != 1 {
		t.Fatalf("findings = %+v", got)
	}
}

func FuzzScan(f *testing.F) {
	f.Add("token = " + gitHubToken)
	f.Add("password: ${X}")
	f.Fuzz(func(t *testing.T, line string) {
		for _, finding := range Scan([]snapshot.FileDiff{diff("a.go", line)}, Options{}) {
			if finding.Length > 8 && strings.Contains(line, finding.Preview) {
				t.Fatalf("the preview must be masked: %q in %q", finding.Preview, line)
			}
		}
	})
}

func TestFingerprint(t *testing.T) {
	// PBKDF2-HMAC-SHA256 of the key, salted with the rule id, in lower-case hexadecimal.
	got := Scan([]snapshot.FileDiff{diff("a.go", "k = "+awsKey)}, Options{})
	if len(got) != 1 || got[0].Fingerprint != Fingerprint(AWSKey, awsKey) || len(got[0].Fingerprint) != 64 ||
		got[0].Fingerprint != strings.ToLower(got[0].Fingerprint) || strings.Contains(got[0].Fingerprint, awsKey) {
		t.Fatalf("findings = %+v", got)
	}
	if Fingerprint(AWSKey, "x") == Fingerprint(GitHubToken, "x") {
		t.Fatal("the fingerprint must depend on the rule and the value")
	}
	// Pinned: the documented definition (docs/reference/configuration.md),
	// checked with Python: hashlib.pbkdf2_hmac("sha256", b"abc",
	// b"ship-it/secrets/fingerprint\x00acme_token", 600000).hex().
	if got := Fingerprint("acme_token", "abc"); got != "c3e81dc0b606a475880ae057ca60e0c25bbd9c65fa06aa7f8a05ac98bdb7f593" {
		t.Fatalf("Fingerprint = %s", got)
	}
	file := Scan([]snapshot.FileDiff{diff(".env")}, Options{})
	if len(file) != 1 || file[0].Fingerprint != Fingerprint(SensitiveFile, ".env") {
		t.Fatalf("sensitive file = %+v", file)
	}
}

func TestAllowAcceptsAFindingByFingerprint(t *testing.T) {
	files := []snapshot.FileDiff{diff("a.go", "k = "+awsKey, "t = "+gitHubToken), diff(".env")}
	got := Scan(files, Options{Allow: []string{Fingerprint(AWSKey, awsKey), Fingerprint(SensitiveFile, ".env")}})
	if len(got) != 1 || got[0].Rule != GitHubToken {
		t.Fatalf("findings = %+v", got)
	}
	// An allowed match leaves the line to the next rules.
	line := awsKey + " " + gitHubToken
	got = Scan([]snapshot.FileDiff{diff("a.go", line)}, Options{Allow: []string{Fingerprint(AWSKey, awsKey)}})
	if len(got) != 1 || got[0].Rule != GitHubToken {
		t.Fatalf("findings = %+v", got)
	}
}

// TestAllowStillAcceptsTheFingerprintsOf01: ship-it 0.1 printed a plain
// SHA-256; an allow list written then keeps working during 0.x.
func TestAllowStillAcceptsTheFingerprintsOf01(t *testing.T) {
	if got := legacyFingerprint("acme_token", "abc"); got != "9ec91464ecaab15247b9c24c0f8aca2198d6f94f1680b8ae0042e9617bbddeb1" {
		t.Fatalf("legacyFingerprint = %s, the value 0.1 printed", got)
	}
	got := Scan([]snapshot.FileDiff{diff("a.go", "k = "+awsKey)}, Options{Allow: []string{legacyFingerprint(AWSKey, awsKey)}})
	if len(got) != 0 {
		t.Fatalf("findings = %+v, want none: a 0.1 fingerprint is still accepted", got)
	}
}

func TestDisabledRules(t *testing.T) {
	files := []snapshot.FileDiff{diff("a.go", "k = "+awsKey, "t = "+gitHubToken), diff(".env")}
	got := Scan(files, Options{Disabled: []RuleID{AWSKey, SensitiveFile}})
	if len(got) != 1 || got[0].Rule != GitHubToken {
		t.Fatalf("findings = %+v", got)
	}
}

func TestCustomRules(t *testing.T) {
	acme, err := NewRule("acme_token", `\bacme_[0-9a-f]{24}\b`, []string{"acme_"})
	if err != nil {
		t.Fatal(err)
	}
	internal, err := NewRule("internal_password", `(?i)internal_pass=(\S{8,})`, []string{"INTERNAL_PASS"})
	if err != nil {
		t.Fatal(err)
	}
	token := fake("acme_", strings.Repeat("0a", 12))
	files := []snapshot.FileDiff{diff("a.go", "x := "+token, "internal_pass="+fake("hunter", "2hunter2"), "acme_ nothing")}
	got := Scan(files, Options{Rules: []Rule{acme, internal}})
	if len(got) != 2 || got[0].Rule != "acme_token" || got[0].Length != len(token) ||
		got[1].Rule != "internal_password" || got[1].Length != 14 || got[1].Fingerprint != Fingerprint("internal_password", fake("hunter", "2hunter2")) {
		t.Fatalf("findings = %+v", got)
	}
	// Without a keyword on the line, the pattern does not run.
	if got := Scan([]snapshot.FileDiff{diff("a.go", "x := "+strings.ToUpper(token))}, Options{Rules: []Rule{acme}}); len(got) != 0 {
		t.Fatalf("findings = %+v", got)
	}
}

func TestNewRuleRefusals(t *testing.T) {
	for _, tt := range []struct {
		id, pattern string
		keywords    []string
	}{
		{"", `x+`, []string{"x"}},
		{"aws_key", `x+`, []string{"x"}},
		{"mine", `(`, []string{"x"}},
		{"mine", `x+`, nil},
		{"mine", `x+`, []string{""}},
		{"mine", `x*`, []string{"x"}},
	} {
		if _, err := NewRule(tt.id, tt.pattern, tt.keywords); err == nil {
			t.Errorf("NewRule(%q, %q, %q) accepted", tt.id, tt.pattern, tt.keywords)
		}
	}
	if ids := BuiltinRules(); len(ids) != 13 || ids[len(ids)-1] != SensitiveFile || !Builtin(AWSKey) || Builtin("mine") {
		t.Fatalf("built-in rules = %v", ids)
	}
}
