// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package i18n

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Language is a supported user interface language.
type Language string

// Supported languages; English is the default (ADR-0011).
const (
	English Language = "en"
	French  Language = "fr"
)

// DefaultLanguage is used when no preference is configured.
const DefaultLanguage = English

// ParseLanguage maps a locale such as "fr_FR.UTF-8" or "fr-CA" to a
// supported language, falling back to the default.
func ParseLanguage(locale string) Language {
	tag := strings.ToLower(locale)
	if i := strings.IndexAny(tag, "_-.@"); i >= 0 {
		tag = tag[:i]
	}
	switch Language(tag) {
	case French:
		return French
	default:
		return DefaultLanguage
	}
}

// Render returns the message for key in lang, with {name} placeholders
// replaced by params. Unknown keys fall back to English, then to the key
// itself, so a missing translation is visible but never fatal.
//
// A plural selector, {name:one|other}, picks a form by the count in
// params[name]: "{count} {count:file|files}" renders "1 file" and "2 files".
// Without a count, or with one that is not a number, it takes the other form.
func Render(lang Language, key string, params map[string]string) string {
	template, ok := catalogs[lang][key]
	if !ok {
		lang = English
		template, ok = catalogs[English][key]
	}
	if !ok {
		return key
	}
	template = pluralize(lang, template, params)
	if len(params) == 0 {
		return template
	}
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, 2*len(names))
	for _, name := range names {
		pairs = append(pairs, "{"+name+"}", params[name])
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// plural matches a plural selector: {name:one|other}. Neither form may hold
// braces or a bar.
var plural = regexp.MustCompile(`\{([a-z_]+):([^{}|]*)\|([^{}|]*)\}`)

// pluralize replaces the plural selectors of template by the form their
// count asks for in lang.
func pluralize(lang Language, template string, params map[string]string) string {
	if !strings.Contains(template, "|") {
		return template
	}
	return plural.ReplaceAllStringFunc(template, func(selector string) string {
		m := plural.FindStringSubmatch(selector)
		if n, err := strconv.Atoi(params[m[1]]); err == nil && isOne(lang, n) {
			return m[2]
		}
		return m[3]
	})
}

// isOne tells whether n takes the singular form in lang, after the CLDR
// plural rules for integers: only 1 in English, 0 and 1 in French
// (https://www.unicode.org/cldr/charts/latest/supplemental/language_plural_rules.html).
func isOne(lang Language, n int) bool {
	if lang == French {
		return n == 0 || n == 1
	}
	return n == 1
}

// Keys returns every key of the English catalogue, sorted.
func Keys() []string {
	keys := make([]string, 0, len(catalogs[English]))
	for k := range catalogs[English] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var catalogs = map[Language]map[string]string{
	English: english,
	French:  french,
}
