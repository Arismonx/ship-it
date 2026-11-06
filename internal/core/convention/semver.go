// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// Version is a Semantic Versioning 2.0.0 version. Tags carry a "v" prefix
// (required by Go modules, docs/design/git-github-standards.md §4) but the version
// itself does not.
type Version struct {
	Major, Minor, Patch int
	Pre                 string // without the leading "-"
	Build               string // without the leading "+"
}

// semverRE is the official numbered-group regular expression from semver.org.
var semverRE = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// ParseVersion accepts "1.2.3" and the tag form "v1.2.3".
func ParseVersion(s string) (Version, error) {
	m := semverRE.FindStringSubmatch(strings.TrimPrefix(s, "v"))
	if m == nil {
		return Version{}, fault.New(fault.InvalidVersion, "convention.version.invalid", "version", s)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return Version{Major: major, Minor: minor, Patch: patch, Pre: m[4], Build: m[5]}, nil
}

// String renders the version without the "v" prefix.
func (v Version) String() string {
	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Tag renders the git tag, "v" included.
func (v Version) Tag() string { return "v" + v.String() }

// Next returns the version that follows v for the given impact. Pre-release
// and build metadata are dropped. Before 1.0.0 a major impact increments the
// minor version: semver.org's FAQ recommends incrementing the minor for each
// release during initial development, and reaching 1.0.0 is a human
// decision (ADR-0013).
func (v Version) Next(impact ReleaseImpact) Version {
	next := Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
	if v.Major == 0 && impact == MajorRelease {
		impact = MinorRelease
	}
	switch impact {
	case MajorRelease:
		next.Major, next.Minor, next.Patch = next.Major+1, 0, 0
	case MinorRelease:
		next.Minor, next.Patch = next.Minor+1, 0
	case PatchRelease:
		next.Patch++
	}
	return next
}

// ReleaseImpactOf returns the impact a single header has on the next
// release, with the built-in table (Rules.ReleaseImpactOf).
func ReleaseImpactOf(h Header) ReleaseImpact { return Rules{}.ReleaseImpactOf(h) }

// ReleaseImpactOf returns the impact a single header has on the next
// release: major for a breaking change, else the release of its type in the
// repository's table (ship.commits.types), none for a type it lacks.
func (r Rules) ReleaseImpactOf(h Header) ReleaseImpact {
	if h.Breaking {
		return MajorRelease
	}
	if spec, ok := r.LookupCommitType(h.Type); ok {
		return spec.ReleaseImpact
	}
	return NoRelease
}

// releaseNames are the names of the release impacts, as settings write them.
var releaseNames = []string{NoRelease: "none", PatchRelease: "patch", MinorRelease: "minor", MajorRelease: "major"}

// ReleaseNames returns the names ParseRelease reads, from no release to major.
func ReleaseNames() []string { return slices.Clone(releaseNames) }

// ParseRelease reads a release impact by name: none, patch, minor or major.
func ParseRelease(name string) (ReleaseImpact, bool) {
	i := slices.Index(releaseNames, name)
	return ReleaseImpact(max(i, 0)), i >= 0
}

// String returns the name of the impact: none, patch, minor or major.
func (i ReleaseImpact) String() string {
	if int(i) < 0 || int(i) >= len(releaseNames) {
		return "none"
	}
	return releaseNames[i]
}

// Compare orders versions by semver precedence: -1, 0 or +1. Build metadata
// is ignored, as the specification requires.
func Compare(a, b Version) int {
	for _, d := range [3]int{a.Major - b.Major, a.Minor - b.Minor, a.Patch - b.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	return comparePre(a.Pre, b.Pre)
}

// comparePre implements semver rule 11: a release outranks its pre-releases;
// identifiers compare numerically when both are numeric, lexically otherwise,
// numeric ones ranking lower; a longer set wins when all others are equal.
func comparePre(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, aNum := atoiStrict(as[i])
		bi, bNum := atoiStrict(bs[i])
		switch {
		case aNum && bNum:
			if ai != bi {
				return sign(ai - bi)
			}
		case aNum:
			return -1
		case bNum:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return sign(len(as) - len(bs))
}

func atoiStrict(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func sign(d int) int {
	switch {
	case d < 0:
		return -1
	case d > 0:
		return 1
	}
	return 0
}
