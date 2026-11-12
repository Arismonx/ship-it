// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// helpArgs asks each flag set for its flags; the key is the flag set name
// given to r.flags.
var helpArgs = map[string][]string{
	"ship":               {"--yes", "--help"},
	"doctor":             {"doctor", "--help"},
	"checkpoint create":  {"checkpoint", "--help"},
	"checkpoint list":    {"checkpoint", "list", "--help"},
	"checkpoint restore": {"checkpoint", "restore", "--help"},
	"status":             {"status", "--help"},
	"publish":            {"publish", "--help"},
	"preview":            {"preview", "--help"},
	"uninstall":          {"uninstall", "--help"},
	"undo":               {"undo", "--help"},
	"gc":                 {"gc", "--help"},
	"plan":               {"plan", "--help"},
	"types":              {"types", "--help"},
	"version":            {"version", "--help"},
	"check title":        {"check", "title", "--help"},
	"check branch":       {"check", "branch", "--help"},
	"check signoff":      {"check", "signoff", "--help"},
	"check gates":        {"check", "gates", "--help"},
	"label":              {"label", "--help"},
	"branch":             {"branch", "--help"},
	"trace":              {"trace", "--help"},
	"tidy":               {"tidy", "--help"},
	"fix":                {"fix", "--help"},
	"log":                {"log", "--help"},
	"config":             {"config", "--help"},
	"config get":         {"config", "get", "--help"},
	"config ":            {"config", "set", "--help"}, // "config " + action: set and unset
	"config explain":     {"config", "explain", "--help"},
	"init":               {"init", "--help"},
	"sync":               {"sync", "--help"},
	"roadmap sync":       {"roadmap", "sync", "--help"},
	"mcp install":        {"mcp", "install", "--help"},
	"stack":              {"stack", "--help"},
	"stack publish":      {"stack", "publish", "--help"},
	"stack status":       {"stack", "status", "--help"},
	"stack sync":         {"stack", "sync", "--help"},
	"stack merge":        {"stack", "merge", "--help"},
}

// reservedShortFlags are the global short flags: no command may define them
// (docs/reference/cli.md), or `ship <command> -d` would change meaning.
var reservedShortFlags = []string{"d", "q"}

// reservedLongFlags are the global long flags, removed from the arguments
// before any command sees them: a command flag of the same name could never
// be set.
var reservedLongFlags = []string{"quiet", "debug", "trace", "set", "plain"}

func TestNoCommandDefinesAGlobalShortFlag(t *testing.T) {
	for name, args := range helpArgs {
		code, stdout, stderr := run(args...)
		// Help is an answer: on stdout, exit 0, flags written as typed.
		if code != ExitOK || stderr != "" || !regexp.MustCompile(`(?m)^  --json\b`).MatchString(stdout) {
			t.Errorf("%s: %q did not print the flags of %q (exit %d):\n%s%s", name, args, name, code, stdout, stderr)
			continue
		}
		if regexp.MustCompile(`(?m)^\s+-[a-z][a-z-]+\b`).MatchString(stdout) {
			t.Errorf("%s: a long flag is written with one dash:\n%s", name, stdout)
		}
		for _, short := range reservedShortFlags {
			if regexp.MustCompile(`(?m)^\s+-` + short + `\b`).MatchString(stdout) {
				t.Errorf("%s defines -%s, reserved for a global flag", name, short)
			}
		}
		for _, long := range reservedLongFlags {
			if regexp.MustCompile(`(?m)^\s+--` + long + `\b`).MatchString(stdout) {
				t.Errorf("%s defines --%s, reserved for a global flag", name, long)
			}
		}
	}
}

// TestEveryFlagSetIsChecked fails when a command gets a flag set that the
// test above does not ask for its flags.
func TestEveryFlagSetIsChecked(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`r\.flags\("([^"]*)"`)
	newFlagSet := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		newFlagSet += strings.Count(string(source), "flag.NewFlagSet(")
		for _, m := range literal.FindAllStringSubmatch(string(source), -1) {
			found := false
			for name := range helpArgs {
				found = found || strings.HasPrefix(name, m[1])
			}
			if !found {
				t.Errorf("%s: flag set %q is missing from helpArgs", file, m[1])
			}
		}
	}
	if newFlagSet != 1 {
		t.Errorf("flag sets must all come from r.flags; found %d flag.NewFlagSet calls", newFlagSet)
	}
}
