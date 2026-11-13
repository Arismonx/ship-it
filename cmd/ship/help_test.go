// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/surface/cli"
)

// everyCommand is every command and subcommand of ship, as typed.
var everyCommand = [][]string{
	{}, {"help"}, {"plan"}, {"doctor"}, {"types"}, {"version"}, {"branch"},
	{"checkpoint"}, {"checkpoint", "create"}, {"checkpoint", "list"}, {"checkpoint", "restore"},
	{"undo"}, {"gc"}, {"trace"}, {"publish"}, {"preview"}, {"status"}, {"tidy"}, {"log"}, {"fix"}, {"label"},
	{"stack"}, {"stack", "status"}, {"stack", "sync"}, {"stack", "merge"}, {"stack", "publish"},
	{"mcp"}, {"mcp", "install"}, {"uninstall"}, {"bug"}, {"skill"}, {"skill", "install"}, {"rewind"},
	{"config"}, {"config", "get"}, {"config", "set"}, {"config", "unset"}, {"config", "explain"},
	{"init"}, {"sync"}, {"roadmap"}, {"roadmap", "sync"},
	{"check"}, {"check", "title"}, {"check", "branch"}, {"check", "signoff"}, {"check", "gates"},
}

// TestHelpChangesNothing: --help (and -h) on every command prints its usage,
// exits 0 and leaves the repository as it was, work in progress included.
func TestHelpChangesNothing(t *testing.T) {
	s := newScenario(t, nil)
	s.write("go.mod", "module example.com/shop\n\ngo 1.21\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.write("vat.go", "package shop\n\nconst VAT = 20\n")
	before := tree(t, s.dir)
	for _, command := range everyCommand {
		for _, help := range []string{"--help", "-h"} {
			args := append(append([]string{}, command...), help)
			code, stdout, stderr := s.ship(args...)
			if code != cli.ExitOK || !strings.Contains(stdout, "Usage: ship") {
				t.Errorf("%q: exit %d\n%s%s", args, code, stdout, stderr)
			}
			if after := tree(t, s.dir); after != before {
				t.Fatalf("%q changed the repository:\n%s\nbefore:\n%s", args, after, before)
			}
		}
	}
}

// tree lists every file under dir with a hash of its content; git's index
// is left out, as git refreshes its stat data on a mere read.
func tree(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if filepath.ToSlash(rel) == ".git/index" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s %x", filepath.ToSlash(rel), sha256.Sum256(data)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
