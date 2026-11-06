// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package rules

import (
	"regexp"
	"strings"
)

// prefixAliases are the long prefixes Conventional Branch 1.1.0 accepts for
// ship-it's short ones: a ruleset asking for "feature/" keeps the branch
// conventional.
var prefixAliases = map[string]string{"feat": "feature", "fix": "bugfix"}

// ticketKey is an external ticket key (ABC-123) as ship-it writes it in a
// branch description: lowercase, since Conventional Branch only allows that.
var ticketKey = regexp.MustCompile(`^[a-z][a-z0-9]*-[0-9]+`)

// CompliantBranch returns a branch name every pattern accepts, derived from
// name (ship-it's own name), or false when none of the derivations fits.
// It never invents text: the candidates are name itself, name with the long
// prefix alias, without its prefix, with an upper-case ticket key, and these
// with the prefix or suffix a literal rule requires. The first candidate
// that is a valid git branch name and breaks no rule wins.
func CompliantBranch(name string, patterns []Pattern) (string, bool) {
	for _, candidate := range branchCandidates(name, patterns) {
		if !ValidRefName(candidate) {
			continue
		}
		if _, broken := FirstViolated(patterns, candidate); !broken {
			return candidate, true
		}
	}
	return "", false
}

func branchCandidates(name string, patterns []Pattern) []string {
	prefix, description, hasPrefix := strings.Cut(name, "/")
	bases := []string{name}
	if hasPrefix {
		if alias, ok := prefixAliases[prefix]; ok {
			bases = append(bases, alias+"/"+description)
		}
		bases = append(bases, description)
	}
	for _, b := range bases[:len(bases):len(bases)] {
		if upper := upperTicket(b); upper != b {
			bases = append(bases, upper)
		}
	}

	var candidates []string
	seen := map[string]bool{}
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			candidates = append(candidates, c)
		}
	}
	for _, b := range bases {
		add(b)
		for _, variant := range withRequiredAffixes(b, patterns) {
			add(variant)
		}
	}
	return candidates
}

// withRequiredAffixes applies the literal prefixes and suffixes the patterns
// require and base lacks. A required prefix ending with "/" replaces base's
// own prefix first ("feat/x" → "feature/x" for "feature/"), then is simply
// prepended ("feature/feat/x").
func withRequiredAffixes(base string, patterns []Pattern) []string {
	variants := []string{base}
	for _, p := range patterns {
		if p.Negate || p.Pattern == "" || p.Check(base) != Violated {
			continue
		}
		var next []string
		for _, v := range variants {
			switch p.Operator {
			case StartsWith:
				if strings.HasPrefix(v, p.Pattern) {
					next = append(next, v)
					continue
				}
				if strings.HasSuffix(p.Pattern, "/") {
					if _, description, ok := strings.Cut(v, "/"); ok {
						next = append(next, p.Pattern+description)
					}
				}
				next = append(next, p.Pattern+v)
			case EndsWith:
				if strings.HasSuffix(v, p.Pattern) {
					next = append(next, v)
					continue
				}
				next = append(next, v+p.Pattern)
			default:
				next = append(next, v)
			}
		}
		variants = next
	}
	return variants
}

// upperTicket writes a ticket key at the start of the description in upper
// case: "feat/abc-123-login" → "feat/ABC-123-login".
func upperTicket(name string) string {
	head, description := "", name
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		head, description = name[:i+1], name[i+1:]
	}
	key := ticketKey.FindString(description)
	if key == "" {
		return name
	}
	return head + strings.ToUpper(key) + description[len(key):]
}

// ValidRefName reports whether name is a valid branch name for
// git check-ref-format --branch: no space, control character or any of
// ~^:?*[\, no "..", "@{" or "//", no component starting with "." or ending
// with ".lock", not starting or ending with "/", not ending with ".", not "@".
func ValidRefName(name string) bool {
	if name == "" || name == "@" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.HasPrefix(name, "-") ||
		strings.Contains(name, "..") || strings.Contains(name, "@{") || strings.Contains(name, "//") {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(" ~^:?*[\\", r) {
			return false
		}
	}
	for _, component := range strings.Split(name, "/") {
		if strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	return true
}
