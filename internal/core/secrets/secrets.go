// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package secrets

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// RuleID names what was detected; surfaces translate "secrets.rule.<id>".
type RuleID string

// Rules, ported from the nuxt-core draft (ship-secrets.mjs).
const (
	PrivateKey       RuleID = "private_key"
	GitHubToken      RuleID = "github_token"
	AWSKey           RuleID = "aws_key"
	AnthropicKey     RuleID = "anthropic_key"
	OpenAIKey        RuleID = "openai_key"
	GroqKey          RuleID = "groq_key"
	GoogleKey        RuleID = "google_key"
	StripeKey        RuleID = "stripe_key"
	SlackToken       RuleID = "slack_token"
	NPMToken         RuleID = "npm_token"
	URLWithPassword  RuleID = "url_with_password"
	SecretAssignment RuleID = "secret_assignment"
	// SensitiveFile flags a whole file that should never be committed.
	SensitiveFile RuleID = "sensitive_file"
)

type rule struct {
	id      RuleID
	pattern *regexp.Regexp
	// hints are literals every match contains one of, lower case when the
	// pattern ignores case: a line with none is not matched against the
	// pattern, the slow part of the scan.
	hints []string
}

// candidate is a rule a scan tries. A custom one (ship.secrets.rules) is
// reported as is, without the placeholder filter of the built-in rules; its
// value is the first capture group, if any.
type candidate struct {
	rule
	custom bool
}

// Rule is a rule of the repository (ship.secrets.rules), built by NewRule.
type Rule struct{ r rule }

// NewRule compiles a custom rule: a Go regular expression (RE2) and the
// literal keywords a line must contain for the pattern to run, compared
// without case when the pattern ignores case ("(?i" anywhere in it). The
// value of a finding is the first capture group, else the whole match.
func NewRule(id, pattern string, keywords []string) (Rule, error) {
	if strings.TrimSpace(id) == "" {
		return Rule{}, errors.New("a custom rule needs an id")
	}
	if Builtin(RuleID(id)) {
		return Rule{}, errors.New("rule " + id + " is a built-in rule")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Rule{}, err
	}
	if re.MatchString("") {
		return Rule{}, errors.New("the pattern of rule " + id + " matches an empty text")
	}
	if len(keywords) == 0 {
		return Rule{}, errors.New("rule " + id + " needs at least one keyword")
	}
	hints := make([]string, len(keywords))
	for i, k := range keywords {
		if k == "" {
			return Rule{}, errors.New("rule " + id + " has an empty keyword")
		}
		if foldsCase(pattern) {
			k = strings.ToLower(k)
		}
		hints[i] = k
	}
	return Rule{rule{id: RuleID(id), pattern: re, hints: hints}}, nil
}

// BuiltinRules returns the ids of the built-in rules, in the order they are
// tried, the sensitive file check last: the ids ship.secrets.disabledRules
// accepts.
func BuiltinRules() []RuleID {
	ids := make([]RuleID, 0, len(rules)+1)
	for _, r := range rules {
		ids = append(ids, r.id)
	}
	return append(ids, SensitiveFile)
}

// Builtin reports whether id names a built-in rule.
func Builtin(id RuleID) bool { return slices.Contains(BuiltinRules(), id) }

// foldsCase reports whether a pattern ignores case somewhere: its hints are
// then compared with the line in lower case.
func foldsCase(pattern string) bool { return strings.Contains(pattern, "(?i") }

// Fingerprint identifies a finding without its value, for ship.secrets.allow:
// the lower-case hexadecimal PBKDF2-HMAC-SHA256 of the matched value (the
// path, for a whole sensitive file), salted with the rule id. The same value
// found by the same rule has the same fingerprint wherever it is, so the
// fingerprint can be committed in .github/ship.json; the cost of the key
// derivation keeps a weak secret from being found back from it by brute
// force, which a plain hash would allow.
func Fingerprint(id RuleID, value string) string {
	key, err := pbkdf2.Key(sha256.New, value, []byte(fingerprintSalt+string(id)), fingerprintIterations, sha256.Size)
	if err != nil {
		// pbkdf2.Key fails only on parameters these constants never give.
		panic("secrets: fingerprint: " + err.Error())
	}
	return hex.EncodeToString(key)
}

// fingerprintIterations follows OWASP's Password Storage Cheat Sheet for
// PBKDF2-HMAC-SHA256 (600 000). A fingerprint is computed only for a finding,
// once, so the cost stays out of a clean scan.
const fingerprintIterations = 600_000

// fingerprintSalt is the fixed part of the salt: a fingerprint must not
// depend on the machine, or a team could not share it.
const fingerprintSalt = "ship-it/secrets/fingerprint\x00"

// legacyFingerprint is the fingerprint of ship-it 0.1, a plain SHA-256 of the
// rule id, a NUL byte and the value. ship.secrets.allow still accepts it during
// 0.x, so that existing allow lists keep working; it is never printed.
//
// Deprecated: removed in 1.0. ship prints the new fingerprint to replace it.
func legacyFingerprint(id RuleID, value string) string {
	sum := sha256.Sum256([]byte(string(id) + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

// rules are tried in order; the first match wins (one finding per line).
// For SecretAssignment the value is the first capture group.
var rules = []rule{
	{PrivateKey, regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY( BLOCK)?-----`), []string{"PRIVATE KEY"}},
	{GitHubToken, regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{60,})\b`), []string{"gh", "github_pat_"}},
	{AWSKey, regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), []string{"AKIA", "ASIA"}},
	{AnthropicKey, regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), []string{"sk-ant-"}},
	{OpenAIKey, regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{32,}`), []string{"sk-"}},
	{GroqKey, regexp.MustCompile(`\bgsk_[A-Za-z0-9]{20,}`), []string{"gsk_"}},
	{GoogleKey, regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), []string{"AIza"}},
	{StripeKey, regexp.MustCompile(`\b(?:sk|rk)_live_[0-9a-zA-Z]{20,}`), []string{"_live_"}},
	{SlackToken, regexp.MustCompile(`\bxox[abprs]-[0-9A-Za-z-]{10,}`), []string{"xox"}},
	{NPMToken, regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), []string{"npm_"}},
	{URLWithPassword, regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s:/@]+:[^\s:/@]{6,}@[^\s]+`), []string{"://"}},
	// Generic assignment: MISTRAL_API_KEY=…, apiKey: "…", password = '…'.
	{SecretAssignment, regexp.MustCompile(`(?i)\b[\w.-]*(?:api[_-]?key|secret|token|passw(?:or)?d|private[_-]?key|access[_-]?key)[\w.-]*["']?\s*[:=]\s*["']?([^\s"'` + "`" + `,;)]{16,})`),
		[]string{"key", "secret", "token", "passw"}},
}

// mayMatch reports whether line, and lower its ASCII lower case, contain a
// hint of r. A line with other characters always may: Unicode case folding
// matches "K" (Kelvin sign) to "k", which lowering ASCII would miss.
func (r rule) mayMatch(line, lower string, ascii bool) bool {
	if !ascii {
		return true
	}
	text := line
	if foldsCase(r.pattern.String()) {
		text = lower
	}
	for _, h := range r.hints {
		if strings.Contains(text, h) {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// placeholder matches values that are obviously fake or indirect.
var placeholder = regexp.MustCompile(`(?i)\$\{|\$\(|process\.env|import\.meta\.env|secrets\.|useRuntimeConfig|<[^>]+>|^x{6,}|example|placeholder|your[_-]?|changeme|dummy|\*{4,}|^\.{3}`)

var (
	sensitiveFile = regexp.MustCompile(`(?i)(^|/)(\.env(\.[\w-]+)?|id_(rsa|ed25519|ecdsa)|[^/]+\.(pem|p12|pfx|key|keystore)|\.npmrc|credentials(\.json)?)$`)
	safeFile      = regexp.MustCompile(`(^|/)\.env\.example$|(^|/)(yarn\.lock|package-lock\.json|pnpm-lock\.yaml)$`)
	// code-like values of the generic rule: config.public.apiKey, MY_CONSTANT.
	dottedIdentifier = regexp.MustCompile(`^[A-Za-z_$][\w$]*(\.[\w$]+)+$`)
	constantName     = regexp.MustCompile(`^[A-Z0-9_]+$`)
)

// Finding is one detected secret. The value itself is never kept.
type Finding struct {
	Path string `json:"path"`
	// Line is 0 for a whole sensitive file.
	Line int    `json:"line"`
	Rule RuleID `json:"rule"`
	// Preview shows the start and end of the value, never the whole of it.
	Preview string `json:"preview"`
	Length  int    `json:"length"`
	// Fingerprint is what ship.secrets.allow lists to accept the finding.
	Fingerprint string `json:"fingerprint"`
}

// Options tune the scan.
type Options struct {
	// IgnorePaths are glob patterns (path.Match syntax, plus a trailing
	// "/**" for a whole directory) of files never scanned: false positives.
	IgnorePaths []string
	// Rules are tried after the built-in ones.
	Rules []Rule
	// Disabled are built-in rules turned off.
	Disabled []RuleID
	// Allow are the fingerprints of accepted findings: a line whose match is
	// allowed is still tried against the next rules.
	Allow []string
}

// Scan checks the added lines of every changed file, deleted files aside.
func Scan(files []snapshot.FileDiff, opts Options) []Finding {
	active := make([]candidate, 0, len(rules)+len(opts.Rules))
	for _, r := range rules {
		if !slices.Contains(opts.Disabled, r.id) {
			active = append(active, candidate{rule: r})
		}
	}
	for _, r := range opts.Rules {
		active = append(active, candidate{rule: r.r, custom: true})
	}
	// A fingerprint is slow on purpose: each one is computed once per scan.
	fingerprints := map[string]string{}
	fingerprint := func(id RuleID, value string) string {
		key := string(id) + "\x00" + value
		if fp, ok := fingerprints[key]; ok {
			return fp
		}
		fp := Fingerprint(id, value)
		fingerprints[key] = fp
		return fp
	}
	allowed := func(id RuleID, value string) bool {
		return len(opts.Allow) > 0 && (slices.Contains(opts.Allow, fingerprint(id, value)) ||
			slices.Contains(opts.Allow, legacyFingerprint(id, value)))
	}
	var findings []Finding
	for _, f := range files {
		if f.Status == snapshot.Deleted || safeFile.MatchString(f.Path) || ignored(f.Path, opts.IgnorePaths) {
			continue
		}
		if sensitiveFile.MatchString(f.Path) && !slices.Contains(opts.Disabled, SensitiveFile) && !allowed(SensitiveFile, f.Path) {
			findings = append(findings, Finding{Path: f.Path, Rule: SensitiveFile, Fingerprint: fingerprint(SensitiveFile, f.Path)})
		}
		for _, line := range f.Added {
			if finding, ok := scanLine(active, line.Text, allowed, fingerprint); ok {
				finding.Path, finding.Line = f.Path, line.Number
				findings = append(findings, finding)
			}
		}
	}
	return findings
}

func scanLine(active []candidate, text string, allowed func(RuleID, string) bool, fingerprint func(RuleID, string) string) (Finding, bool) {
	ascii := isASCII(text)
	lower := ""
	if ascii {
		lower = strings.ToLower(text)
	}
	for _, r := range active {
		if !r.mayMatch(text, lower, ascii) {
			continue
		}
		m := r.pattern.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		value := m[0]
		if len(m) > 1 && (r.id == SecretAssignment || r.custom && m[1] != "") {
			value = m[1]
		}
		if !r.custom && placeholder.MatchString(value) {
			continue
		}
		if r.id == SecretAssignment && !r.custom && !looksRandom(value) {
			continue
		}
		if allowed(r.id, value) {
			continue
		}
		return Finding{Rule: r.id, Preview: mask(value), Length: len([]rune(value)), Fingerprint: fingerprint(r.id, value)}, true
	}
	return Finding{}, false
}

// looksRandom tells a generated secret from code for the generic rule: it
// mixes letters and digits and is not an identifier, a call or a constant.
func looksRandom(v string) bool {
	return strings.ContainsAny(v, "0123456789") && strings.IndexFunc(v, isLetter) >= 0 &&
		!dottedIdentifier.MatchString(v) && !strings.Contains(v, "(") && !constantName.MatchString(v)
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// mask keeps four characters at the start and two at the end.
func mask(v string) string {
	runes := []rune(v)
	if len(runes) <= 8 {
		return "****"
	}
	return string(runes[:4]) + "…" + string(runes[len(runes)-2:])
}

func ignored(p string, patterns []string) bool {
	for _, pattern := range patterns {
		if dir, ok := strings.CutSuffix(pattern, "/**"); ok {
			if p == dir || strings.HasPrefix(p, dir+"/") {
				return true
			}
			continue
		}
		if matched, _ := path.Match(pattern, p); matched {
			return true
		}
	}
	return false
}
