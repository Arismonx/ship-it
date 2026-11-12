// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseAssemblesTheFragments(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"b-bug.md":  "### Fixed\n\n- `ship bug` reads the right run (#3).\n",
		"a-feat.md": "### Added\n\n- `ship preview` suggests reviewers,\n  from CODEOWNERS (#2).\n\n### Fixed\n\n- a typo (#4).\n",
		"README.md": "How to write a fragment.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fragments, files, err := readFragments(dir)
	if err != nil || len(files) != 2 {
		t.Fatalf("readFragments: %d files, %v", len(files), err)
	}
	changelog := "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- `ship uninstall` (#1).\n\n## [0.1.1] - 2026-11-07\n\nFixes.\n\n" +
		"[Unreleased]: https://github.com/GregoireF/ship-it/compare/v0.1.1...HEAD\n[0.1.1]: https://github.com/GregoireF/ship-it/compare/v0.1.0...v0.1.1\n"
	got, err := release(changelog, fragments, "v0.2.0", "2026-12-01", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## [Unreleased]\n\n" + unreleasedNote + "\n\n## [0.2.0] - 2026-12-01\n\n### Added\n\n- `ship uninstall` (#1).\n- `ship preview` suggests reviewers, from CODEOWNERS (#2).\n\n### Fixed\n\n- a typo (#4).\n- `ship bug` reads the right run (#3).\n\n## [0.1.1]",
		"[Unreleased]: https://github.com/GregoireF/ship-it/compare/v0.2.0...HEAD\n[0.2.0]: https://github.com/GregoireF/ship-it/compare/v0.1.1...v0.2.0\n[0.1.1]:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("CHANGELOG lacks:\n%s\n--- got:\n%s", want, got)
		}
	}
	var notes strings.Builder
	if err := write(&notes, got, "0.2.0"); err != nil || !strings.HasPrefix(notes.String(), "### Added") {
		t.Errorf("notes of the new release: %q, %v", notes.String(), err)
	}
	// Once released, [Unreleased] holds only the note: nothing pending.
	if p, err := pending(got, entries{}); err != nil || p != "" {
		t.Errorf("pending after the release: %q, %v", p, err)
	}
}

func TestFragmentsAreWellFormed(t *testing.T) {
	for _, bad := range []string{"- an entry without a section\n", "### Improved\n\n- x\n", "### Fixed\n\nprose, not an entry\n"} {
		if _, err := parseEntries(bad, "f.md"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	// The real ones.
	if _, _, err := readFragments(filepath.Join("..", "..", FragmentDir)); err != nil {
		t.Error(err)
	}
}
