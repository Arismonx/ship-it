// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"strings"
	"testing"
)

const changelog = `# Changelog

## [Unreleased]

### Added

- something new

## [0.1.1] - 2026-11-07

Fixes found on real repositories.

### Fixed

- a fix (#17)

## [0.1.0] - 2026-11-07

First release.

[Unreleased]: https://github.com/GregoireF/ship-it/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/GregoireF/ship-it/compare/v0.1.0...v0.1.1
`

func TestWrite(t *testing.T) {
	var out strings.Builder
	if err := write(&out, changelog, "v0.1.1"); err != nil {
		t.Fatal(err)
	}
	if want := "Fixes found on real repositories.\n\n### Fixed\n\n- a fix (#17)\n"; out.String() != want {
		t.Errorf("notes = %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := write(&out, changelog, "0.1.0"); err != nil || out.String() != "First release.\n" {
		t.Errorf("last section: %q, %v (link definitions must go)", out.String(), err)
	}
	for _, version := range []string{"Unreleased", "v9.9.9"} {
		if err := write(&out, changelog, version); err == nil {
			t.Errorf("%s: no error", version)
		}
	}
}

// TestEveryReleaseHasNotes: each dated section of the real CHANGELOG.md
// gives notes.
func TestEveryReleaseHasNotes(t *testing.T) {
	content, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range heading.FindAllStringSubmatch(string(content), -1) {
		if m[1] == "Unreleased" {
			continue
		}
		var out strings.Builder
		if err := write(&out, string(content), m[1]); err != nil || out.Len() == 0 {
			t.Errorf("%s: %v", m[1], err)
		}
	}
}
