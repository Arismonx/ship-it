// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package secrets

import (
	"regexp"
	"strings"
	"testing"
)

// hintSeeds are lines each rule matches, in several cases, and lines that
// fold to a hint only through Unicode (Kelvin sign, long s).
var hintSeeds = []string{
	"token = " + gitHubToken, "AWS=" + awsKey, privateKey, "STRIPE=" + stripeKey,
	"sk-ant-" + strings.Repeat("a", 24), "sk-proj-" + strings.Repeat("b", 40), "gsk_" + strings.Repeat("c", 24),
	"AIza" + strings.Repeat("d", 35), "xoxb-1234567890-abc", "npm_" + strings.Repeat("e", 36),
	"HTTPS://admin:" + fake("s3cr", "etPass") + "@db.internal", `API-KEY: "` + fake("Zk8q", "Lw2vN9xT4pR7mY3s") + `"`,
	`PassWord = "` + fake("Zk8q", "Lw2vN9xT4pR7mY3s") + `"`, `my_kEy="` + fake("Zk8q", "Lw2vN9xT4pR7mY3s") + `"`,
	"api_Key=" + fake("Zk8q", "Lw2vN9xT4pR7mY3s"), "ſecret=" + fake("Zk8q", "Lw2vN9xT4pR7mY3s"),
}

// sampleRules are custom rules as a repository writes them (ship.secrets.rules),
// one ignoring case and one with mixed-case keywords.
var sampleRules = func() []Rule {
	var out []Rule
	for _, def := range []struct {
		id, pattern string
		keywords    []string
	}{
		{"acme_token", `\bACME-[0-9a-f]{24}\b`, []string{"ACME-"}},
		{"internal_password", `(?i)\binternal[_-]?pass(?:word)?\s*[:=]\s*(\S{8,})`, []string{"Internal"}},
	} {
		r, err := NewRule(def.id, def.pattern, def.keywords)
		if err != nil {
			panic(err)
		}
		out = append(out, r)
	}
	return out
}()

// checkHints fails when a rule's pattern matches line but its hints would
// have skipped it: the prefilter may only save time, never a finding. A
// built-in rule's hints are part of every match; a custom rule only runs on a
// line holding one of its keywords (compared with Unicode case folding when
// the pattern ignores case), and then its prefilter must not skip the line.
func checkHints(t *testing.T, line string) {
	t.Helper()
	ascii := isASCII(line)
	lower := strings.ToLower(line)
	for _, r := range rules {
		if r.pattern.MatchString(line) && !r.mayMatch(line, lower, ascii) {
			t.Errorf("rule %s matches %q but its hints skip it", r.id, line)
		}
	}
	for _, custom := range sampleRules {
		r := custom.r
		if r.pattern.MatchString(line) && holdsKeyword(r, line) && !r.mayMatch(line, lower, ascii) {
			t.Errorf("custom rule %s matches %q, which holds a keyword, but its hints skip it", r.id, line)
		}
	}
}

// holdsKeyword is the definition the prefilter of a custom rule implements.
func holdsKeyword(r rule, line string) bool {
	for _, k := range r.hints {
		flags := ""
		if foldsCase(r.pattern.String()) {
			flags = "(?i)"
		}
		if regexp.MustCompile(flags + regexp.QuoteMeta(k)).MatchString(line) {
			return true
		}
	}
	return false
}

func TestHintsNeverHideAMatch(t *testing.T) {
	for _, line := range hintSeeds {
		checkHints(t, line)
	}
	// A Kelvin sign folds to "k" in the pattern, not in ASCII lowering: the
	// line is still matched against the pattern.
	if _, ok := scanLine(builtinCandidates(), "api_Key="+fake("Zk8q", "Lw2vN9xT4pR7mY3s"), noneAllowed, noFingerprint); !ok {
		t.Error("a Kelvin sign must not hide a secret")
	}
}

func FuzzHints(f *testing.F) {
	for _, line := range hintSeeds {
		f.Add(line)
	}
	f.Fuzz(checkHints)
}

func builtinCandidates() []candidate {
	out := make([]candidate, len(rules))
	for i, r := range rules {
		out[i] = candidate{rule: r}
	}
	return out
}

func noneAllowed(RuleID, string) bool { return false }

// noFingerprint skips the slow fingerprint where a test only looks for a match.
func noFingerprint(RuleID, string) string { return "" }
