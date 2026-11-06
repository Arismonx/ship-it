// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/fsconfig"
	"github.com/GregoireF/ship-it/internal/platform"
)

// TestLanguageSetting: ui.language beats the locale, SHIP_LANG beats
// ui.language, and a broken configuration leaves the locale's language.
func TestLanguageSetting(t *testing.T) {
	s := newScenario(t, nil)
	home := t.TempDir()
	s.deps.Config = &fsconfig.Files{Git: s.git, Env: platform.Environment{GOOS: "linux", Home: home,
		Getenv: func(string) string { return "" }, PolicyPath: filepath.Join(t.TempDir(), "none.json")}}
	userDir := filepath.Join(home, ".config", "ship-it")
	for _, dir := range []string{filepath.Join(s.dir, ".github"), userDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeUser := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(userDir, "config.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want string) {
		t.Helper()
		if _, _, stderr := s.ship("check", "title", "bad title"); !strings.Contains(stderr, want) {
			t.Fatalf("want %q in\n%s", want, stderr)
		}
	}
	check("The title must look like")

	writeUser(`{"ui": {"language": "fr"}}`)
	check("Le titre doit ressembler")

	s.deps.Environ = []string{"SHIP_LANG=en"}
	check("The title must look like")

	s.deps.Environ = []string{"SHIP_UI_LANGUAGE=en"} // a setting variable beats the file
	check("The title must look like")

	s.deps.Environ = nil
	// A broken file stops the check (its rules cannot be known), and the error
	// is in the locale's language, not the file's.
	s.write(filepath.Join(".github", "ship.json"), `{"ship": {"publish": {"mode": 3}}}`)
	check("ship.publish.mode must be string, not 3.")

	// The interface is personal: the repository file cannot set it, and the
	// error says where it belongs.
	s.write(filepath.Join(".github", "ship.json"), `{"ui": {"language": "fr"}}`)
	check("ui holds personal interface settings, which belong in your user file")
}
