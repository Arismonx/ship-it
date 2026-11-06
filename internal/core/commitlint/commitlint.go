// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package commitlint adapts ship-it's commit titles to a project's own
// commitlint configuration (rule zero, docs/reference/conventions.md: what the project imposes
// wins). It reads the configuration that `commitlint --print-config json`
// resolves and rewrites a header where a rule says so, without inventing
// anything; the project's commitlint then validates the final message, run
// by app through a port. Only rules at level 2 (errors) are followed:
// warnings do not fail commitlint.
//
// Facts checked on commitlint.js.org and @commitlint/cli 19.8.1.
//
// Allowed imports: standard library without I/O, other core packages.
package commitlint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
)

// configFiles are the files @commitlint/cli looks for (commitlint.js.org,
// "Configuration"); a "commitlint" field of package.json counts too.
var configFiles = []string{
	".commitlintrc", ".commitlintrc.json", ".commitlintrc.yaml", ".commitlintrc.yml",
	".commitlintrc.js", ".commitlintrc.cjs", ".commitlintrc.mjs",
	".commitlintrc.ts", ".commitlintrc.cts", ".commitlintrc.mts",
	"commitlint.config.js", "commitlint.config.cjs", "commitlint.config.mjs",
	"commitlint.config.ts", "commitlint.config.cts", "commitlint.config.mts",
}

// ConfigFiles returns the configuration file names commitlint reads.
func ConfigFiles() []string { return slices.Clone(configFiles) }

// Binaries are the local executables an installed @commitlint/cli leaves:
// the script, and the shims npm and pnpm write for Windows.
func Binaries() []string {
	return []string{"node_modules/.bin/commitlint", "node_modules/.bin/commitlint.cmd"}
}

// DeclaredInPackageJSON reports whether package.json has a commitlint field.
func DeclaredInPackageJSON(pkg []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(pkg, &fields) != nil {
		return false
	}
	_, ok := fields["commitlint"]
	return ok
}

// npx runs the local binary only: --no refuses to download a missing package
// (npm 10, "npx": --no-install is its deprecated alias), and "--" ends npm's
// own options, without which npm 10.9 takes --print-config for its own and
// commitlint only prints its help (checked with @commitlint/cli 21.2.1).
var npx = []string{"npx", "--no", "--", "commitlint"}

// PrintConfigCommand prints the resolved configuration as JSON.
func PrintConfigCommand() []string { return append(slices.Clone(npx), "--print-config", "json") }

// LintCommand lints the message given on standard input: commitlint reads
// stdin when --edit, --env, --from and --to are omitted, and exits non-zero
// on an error.
func LintCommand() []string { return slices.Clone(npx) }

// Rule is one rule as commitlint writes it: [level, applicable, value].
type Rule struct {
	// Level is 0 (disabled), 1 (warning) or 2 (error).
	Level int
	// Applicable is "always" or "never".
	Applicable string
	Value      json.RawMessage
}

// Config is the part of the resolved configuration ship-it follows.
type Config struct {
	Rules map[string]Rule
}

// ParseConfig reads the output of `commitlint --print-config json`. A
// parserPreset's headerPattern is a RegExp, which JSON prints as {}: whether
// a leading gitmoji is accepted is found by linting, not read here.
func ParseConfig(out []byte) (Config, error) {
	if i := bytes.IndexByte(out, '{'); i > 0 {
		out = out[i:]
	}
	var raw struct {
		Rules map[string][]json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Config{}, fmt.Errorf("commitlint: unreadable configuration: %w", err)
	}
	cfg := Config{Rules: map[string]Rule{}}
	for name, parts := range raw.Rules {
		var r Rule
		if len(parts) == 0 || json.Unmarshal(parts[0], &r.Level) != nil {
			continue
		}
		if len(parts) > 1 {
			_ = json.Unmarshal(parts[1], &r.Applicable)
		}
		if len(parts) > 2 {
			r.Value = parts[2]
		}
		if r.Applicable == "" {
			r.Applicable = "always"
		}
		cfg.Rules[name] = r
	}
	return cfg, nil
}

// errorRule returns a rule that fails commitlint (level 2).
func (c Config) errorRule(name string) (Rule, bool) {
	r, ok := c.Rules[name]
	return r, ok && r.Level == 2
}

// Length rules of commitlint that ship-it's own limits correspond to
// (ship.commits.headerMaxLength, subjectMaxLength, bodyWidth).
const (
	HeaderMaxLength   = "header-max-length"
	SubjectMaxLength  = "subject-max-length"
	BodyMaxLineLength = "body-max-line-length"
)

// MaxLength returns the limit a length rule sets when it fails commitlint
// (level 2, "always", a number); ok is false when the project has no such
// rule, and ship-it's own limit then decides.
func (c Config) MaxLength(rule string) (limit int, ok bool) {
	r, ok := c.errorRule(rule)
	if !ok || r.Applicable != "always" {
		return 0, false
	}
	return r.number()
}

func (r Rule) strings() []string {
	var list []string
	if json.Unmarshal(r.Value, &list) == nil {
		return list
	}
	var one string
	if json.Unmarshal(r.Value, &one) == nil && one != "" {
		return []string{one}
	}
	return nil
}

func (r Rule) number() (int, bool) {
	var n int
	return n, json.Unmarshal(r.Value, &n) == nil
}

// Reason is one adaptation, as a plan reason key.
type Reason struct {
	Key    string
	Params map[string]string
}

// typeAllowed returns whether the project's type-enum (level 2) allows a
// type, and whether it has one at all.
func (c Config) typeAllowed() (func(convention.CommitType) bool, bool) {
	r, ok := c.errorRule("type-enum")
	if !ok || len(r.strings()) == 0 {
		return nil, false
	}
	list := r.strings()
	return func(t convention.CommitType) bool {
		return slices.Contains(list, string(t)) == (r.Applicable == "always")
	}, true
}

// TypeChoices returns the types a human may pick when the project's
// type-enum refuses t and Adapt cannot pick for them: the close types it
// allows when there are several, otherwise every allowed type of the
// repository's table (rules, ship.commits.types), in its order. nil when t is
// allowed, when exactly one close type is (Adapt takes it), or when the
// project allows none of the table.
func TypeChoices(t convention.CommitType, cfg Config, rules convention.Rules) []convention.CommitType {
	allowed, ok := cfg.typeAllowed()
	if !ok || allowed(t) {
		return nil
	}
	if candidates := typeCandidates(t, allowed, rules); len(candidates) > 1 {
		return candidates
	}
	return nil // one: Adapt takes it; none: Adapt refuses
}

// typeCandidates are the allowed types of the repository's table that could
// replace t: its close types (convention.CloseTypes) the project allows, else
// every allowed type of the table, in its order. Adapt and TypeChoices share it, so that they never
// disagree on whether a human has to pick.
func typeCandidates(t convention.CommitType, allowed func(convention.CommitType) bool, rules convention.Rules) []convention.CommitType {
	var close []convention.CommitType
	for _, c := range convention.CloseTypes(t) {
		if _, known := rules.LookupCommitType(c); known && allowed(c) {
			close = append(close, c)
		}
	}
	if len(close) > 0 {
		return close
	}
	var all []convention.CommitType
	for _, spec := range rules.CommitTypes() {
		if allowed(spec.Type) {
			all = append(all, spec.Type)
		}
	}
	return all
}

// Adapt rewrites h so that the project's level-2 rules accept it, written
// with placement: the type (to the one closest type allowed), the scope
// (dropped when not allowed or too long a header), the subject's case and
// final period. It returns what it changed, or a CommitlintUnsatisfiable
// fault when no rewrite fits (ship-it never truncates a subject).
func Adapt(h convention.Header, placement convention.GitmojiPlacement, cfg Config, rules convention.Rules) (convention.Header, []Reason, *fault.Fault) {
	var reasons []Reason
	if allowed, ok := cfg.typeAllowed(); ok {
		if !allowed(h.Type) {
			fits := typeCandidates(h.Type, allowed, rules)
			if len(fits) != 1 {
				r, _ := cfg.errorRule("type-enum")
				return h, nil, fault.New(fault.CommitlintUnsatisfiable, "deliver.commitlint.type_not_allowed",
					"type", string(h.Type), "allowed", strings.Join(r.strings(), ", "))
			}
			if h.Gitmoji == "" {
				h.Gitmoji = rules.CanonicalGitmoji(h.Type, h.Breaking) // the gitmoji keeps the meaning
			}
			reasons = append(reasons, Reason{Key: "plan.reason.commitlint.type", Params: map[string]string{"from": string(h.Type), "to": string(fits[0])}})
			h.Type = fits[0]
		}
	}
	if h.Scope != "" {
		if r, ok := cfg.errorRule("scope-enum"); ok && len(r.strings()) > 0 && slices.Contains(r.strings(), h.Scope) != (r.Applicable == "always") {
			reasons = append(reasons, Reason{Key: "plan.reason.commitlint.scope", Params: map[string]string{"scope": h.Scope}})
			h.Scope = ""
		}
	}
	if r, ok := cfg.errorRule("scope-empty"); ok && r.Applicable == "never" && h.Scope == "" {
		return h, nil, fault.New(fault.CommitlintUnsatisfiable, "deliver.commitlint.scope_required")
	}
	if r, ok := cfg.errorRule("scope-case"); ok && r.Applicable == "always" && slices.Contains(r.strings(), "lower-case") {
		h.Scope = strings.ToLower(h.Scope)
	}

	subject := strings.TrimSpace(h.Subject)
	if r, ok := cfg.errorRule("subject-full-stop"); ok {
		stop := "."
		if s := r.strings(); len(s) == 1 {
			stop = s[0]
		}
		if r.Applicable == "never" {
			subject = strings.TrimSuffix(subject, stop)
		} else if !strings.HasSuffix(subject, stop) {
			subject += stop
		}
	}
	if r, ok := cfg.errorRule("subject-case"); ok {
		subject = withCase(subject, r)
	}
	if subject != strings.TrimSpace(h.Subject) {
		reasons = append(reasons, Reason{Key: "plan.reason.commitlint.subject"})
	}
	h.Subject = subject

	if limit, ok := cfg.MaxLength(HeaderMaxLength); ok {
		if Length(rules.Format(h, placement)) > limit && h.Scope != "" {
			reasons = append(reasons, Reason{Key: "plan.reason.commitlint.scope", Params: map[string]string{"scope": h.Scope}})
			h.Scope = ""
		}
		if n := Length(rules.Format(h, placement)); n > limit {
			return h, nil, fault.New(fault.CommitlintUnsatisfiable, "deliver.commitlint.too_long",
				"rule", HeaderMaxLength, "length", strconv.Itoa(n), "max", strconv.Itoa(limit))
		}
	}
	if limit, ok := cfg.MaxLength(SubjectMaxLength); ok {
		if n := Length(subjectAsParsed(rules, h, placement)); n > limit {
			return h, nil, fault.New(fault.CommitlintUnsatisfiable, "deliver.commitlint.too_long",
				"rule", SubjectMaxLength, "length", strconv.Itoa(n), "max", strconv.Itoa(limit))
		}
	}
	return h, reasons, nil
}

// subjectAsParsed is the subject commitlint's parser sees: after the colon,
// so it holds the gitmoji written after the type.
func subjectAsParsed(rules convention.Rules, h convention.Header, placement convention.GitmojiPlacement) string {
	_, subject, _ := strings.Cut(rules.Format(h, placement), ": ")
	return subject
}

// withCase applies a subject-case rule to the first letter or the whole
// subject, for the cases ship-it can write without guessing: lower-case,
// upper-case and sentence-case. Other cases are left to the validation.
func withCase(subject string, r Rule) string {
	cases := r.strings()
	matches := func(c string) bool {
		switch c {
		case "lower-case":
			return subject == strings.ToLower(subject)
		case "upper-case":
			return subject == strings.ToUpper(subject)
		case "sentence-case":
			return subject == upperFirst(strings.ToLower(subject))
		case "start-case":
			for _, word := range strings.Fields(subject) {
				if word != upperFirst(strings.ToLower(word)) {
					return false
				}
			}
			return subject != ""
		}
		return false
	}
	if r.Applicable == "always" {
		if slices.ContainsFunc(cases, matches) {
			return subject
		}
		switch {
		case slices.Contains(cases, "lower-case"):
			return strings.ToLower(subject)
		case slices.Contains(cases, "sentence-case"):
			return upperFirst(subject)
		case slices.Contains(cases, "upper-case"):
			return strings.ToUpper(subject)
		}
		return subject
	}
	switch {
	case slices.Contains(cases, "upper-case") && matches("upper-case") && subject != strings.ToLower(subject):
		return strings.ToLower(subject)
	case slices.ContainsFunc([]string{"sentence-case", "start-case"}, func(c string) bool { return slices.Contains(cases, c) && matches(c) }):
		return lowerFirst(subject)
	}
	return subject
}

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func lowerFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[size:]
}

// Length counts like commitlint, a JavaScript program: in UTF-16 code
// units, so "🐛" counts 2 and "✨" 1.
func Length(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}
