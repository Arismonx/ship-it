// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package i18n

import (
	"regexp"
	"strings"
	"testing"
)

func TestCataloguesHaveSameKeys(t *testing.T) {
	for key := range english {
		if _, ok := french[key]; !ok {
			t.Errorf("French catalogue misses %q", key)
		}
	}
	for key := range french {
		if _, ok := english[key]; !ok {
			t.Errorf("French catalogue has extra key %q", key)
		}
	}
}

func TestPlaceholdersMatch(t *testing.T) {
	placeholder := regexp.MustCompile(`\{[a-z_]+\}`)
	// Sets, not counts: a placeholder may appear twice in one sentence.
	set := func(s string) map[string]bool {
		found := map[string]bool{}
		for _, p := range placeholder.FindAllString(s, -1) {
			found[p] = true
		}
		return found
	}
	for key, en := range english {
		want, got := set(en), set(french[key])
		for p := range got {
			if !want[p] {
				t.Errorf("%q: French uses %s, English does not", key, p)
			}
		}
		for p := range want {
			if !got[p] {
				t.Errorf("%q: French misses %s", key, p)
			}
		}
	}
}

func TestRender(t *testing.T) {
	got := Render(French, "convention.header.subject_too_long", map[string]string{"length": "61", "max": "50"})
	if got != "Le sujet fait 61 caractères ; reste sous 50." {
		t.Fatalf("Render() = %q", got)
	}
	if got := Render(French, "no.such.key", nil); got != "no.such.key" {
		t.Fatalf("Render(unknown) = %q, want the key", got)
	}
}

func TestPluralSelectors(t *testing.T) {
	cases := []struct {
		lang  Language
		count string
		want  string
	}{
		{English, "0", "0 files not shipped"},
		{English, "1", "1 file not shipped"},
		{English, "2", "2 files not shipped"},
		{French, "0", "0 fichier non livré"},
		{French, "1", "1 fichier non livré"},
		{French, "2", "2 fichiers non livrés"},
	}
	for _, c := range cases {
		if got := Render(c.lang, "cli.status.uncommitted", map[string]string{"count": c.count}); got != c.want {
			t.Errorf("%s, %s: Render() = %q, want %q", c.lang, c.count, got, c.want)
		}
	}
	// Without a count (the error reference renders the templates), the
	// other form stands for any number.
	if got := Render(English, "secrets.found", nil); !strings.HasPrefix(got, "{count} secrets in your changes") {
		t.Errorf("Render(no count) = %q", got)
	}
}

// TestNoMessageGuessesItsPlural keeps "1 file(s)" out of the catalogues:
// a count takes a plural selector instead.
func TestNoMessageGuessesItsPlural(t *testing.T) {
	guess := regexp.MustCompile(`\w\((s|es|x|e)\)`)
	for lang, catalogue := range catalogs {
		for key, message := range catalogue {
			if guess.MatchString(message) {
				t.Errorf("%s %q guesses its plural: %q", lang, key, message)
			}
		}
	}
}

// TestPluralSelectorsAreWellFormed checks every selector names a placeholder
// of its own message, so that a count always reaches it.
func TestPluralSelectorsAreWellFormed(t *testing.T) {
	for lang, catalogue := range catalogs {
		for key, message := range catalogue {
			for _, m := range plural.FindAllStringSubmatch(message, -1) {
				if !strings.Contains(message, "{"+m[1]+"}") {
					t.Errorf("%s %q: the selector of {%s} has no {%s} in its message", lang, key, m[1], m[1])
				}
			}
			if rest := plural.ReplaceAllString(message, ""); regexp.MustCompile(`\{[a-z_]+:`).MatchString(rest) {
				t.Errorf("%s %q: a malformed plural selector: %q", lang, key, message)
			}
		}
	}
}

func TestParseLanguage(t *testing.T) {
	cases := map[string]Language{"fr_FR.UTF-8": French, "fr-CA": French, "en_US": English, "": English, "de_DE": English}
	for in, want := range cases {
		if got := ParseLanguage(in); got != want {
			t.Errorf("ParseLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}
