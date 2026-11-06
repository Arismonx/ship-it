// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Placeholders written instead of private values.
const (
	RedactedEmail = "[EMAIL]"
	RedactedPath  = "[PATH]"
)

var (
	// urlCredentials matches the user-info part of a URL: scheme://user:pass@.
	urlCredentials = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)[^/\s@]+@`)
	email          = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)*\.[A-Za-z]{2,}`)
	// githubToken matches the documented GitHub token prefixes.
	githubToken = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{16,})`)
	// authorization matches "Bearer <token>", "token <token>", "Basic <b64>".
	authorization = regexp.MustCompile(`(?i)\b(bearer|token|basic)(\s+)[A-Za-z0-9._~+/=\-]{8,}`)
	// windowsPath and unixPath catch absolute paths that no known prefix
	// covered; the first group keeps the delimiter before the path.
	windowsPath = regexp.MustCompile(`(^|[\s"'=(\[,;<])[A-Za-z]:[\\/][^\s"'<>|]*`)
	unixPath    = regexp.MustCompile(`(^|[\s"'=(\[,;<])/(?:home|Users|root|tmp|var|private|mnt|opt|srv|Volumes|run|media)/[^\s"']*`)
)

// Scrubber removes private data from log values: absolute paths (replaced by
// a short alias such as "." for the repository or "~" for the home
// directory), e-mail addresses, URL credentials and tokens. It is applied to
// every record of the journal, the black box and the --debug echo
// (docs/reference/logs.md §4).
type Scrubber struct {
	prefixes []prefix
}

type prefix struct{ path, alias string }

// NewScrubber returns a scrubber replacing each absolute path of aliases (for
// example the repository top level → ".", the home directory → "~") by its
// alias. Longer paths win, so a repository inside the home directory becomes
// "." rather than "~/…". Empty paths are ignored.
func NewScrubber(aliases map[string]string) *Scrubber {
	s := &Scrubber{}
	for path, alias := range aliases {
		if path == "" || path == "/" || path == "." {
			continue
		}
		clean := filepath.Clean(path)
		s.prefixes = append(s.prefixes, prefix{clean, alias})
		if slashed := filepath.ToSlash(clean); slashed != clean {
			s.prefixes = append(s.prefixes, prefix{slashed, alias})
		}
	}
	sort.Slice(s.prefixes, func(i, j int) bool {
		if len(s.prefixes[i].path) != len(s.prefixes[j].path) {
			return len(s.prefixes[i].path) > len(s.prefixes[j].path)
		}
		return s.prefixes[i].path < s.prefixes[j].path
	})
	return s
}

// String returns in without private data.
func (s *Scrubber) String(in string) string {
	if in == "" {
		return in
	}
	out := in
	if s != nil {
		for _, p := range s.prefixes {
			out = strings.ReplaceAll(out, p.path, p.alias)
		}
	}
	out = urlCredentials.ReplaceAllString(out, "${1}"+Redacted+"@")
	out = githubToken.ReplaceAllString(out, Redacted)
	out = authorization.ReplaceAllString(out, "${1}${2}"+Redacted)
	out = email.ReplaceAllString(out, RedactedEmail)
	out = windowsPath.ReplaceAllString(out, "${1}"+RedactedPath)
	out = unixPath.ReplaceAllString(out, "${1}"+RedactedPath)
	return out
}

// Attr returns a with its value scrubbed, resolving slog.LogValuer first so
// that a Secret stays [REDACTED].
func (s *Scrubber) Attr(a slog.Attr) slog.Attr {
	return slog.Attr{Key: a.Key, Value: s.value(a.Value)}
}

func (s *Scrubber) value(v slog.Value) slog.Value {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.StringValue(s.String(v.String()))
	case slog.KindGroup:
		attrs := v.Group()
		out := make([]slog.Attr, len(attrs))
		for i, a := range attrs {
			out[i] = s.Attr(a)
		}
		return slog.GroupValue(out...)
	case slog.KindAny:
		switch x := v.Any().(type) {
		case nil:
			return v
		case error:
			return slog.StringValue(s.String(x.Error()))
		case []string:
			out := make([]string, len(x))
			for i, item := range x {
				out[i] = s.String(item)
			}
			return slog.AnyValue(out)
		default:
			// Anything else could carry private fields once marshalled: it is
			// logged as its scrubbed text.
			return slog.StringValue(s.String(fmt.Sprint(x)))
		}
	}
	return v
}
