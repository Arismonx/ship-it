// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command changes says which CI jobs a pull request needs, from the files it
// changes (#146). It prints GitHub Actions outputs, one key=true|false per
// line, for the `changes` job of ci.yml:
//
//	go run ./tools/changes -base origin/main >> "$GITHUB_OUTPUT"
//
// Without -base (a push to main, a scheduled run), everything runs. Jobs are
// then skipped with `if:`, which GitHub reports as success, never with path
// filters, which would leave a required check pending.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Needs are the groups of jobs a change calls for.
type Needs struct {
	// Go: Go code, its module or test data changed: Windows and the
	// benchmarks run, besides Linux.
	Go bool
	// System: code that talks to the operating system changed (paths,
	// processes, files, the install scripts): macOS runs too.
	System bool
	// Extension: the VS Code extension, or what it talks to, changed.
	Extension bool
}

// everything is what a run without a base, or a change to the CI itself,
// needs.
var everything = Needs{Go: true, System: true, Extension: true}

// ciItself are files whose change can alter any job: they run them all.
var ciItself = []string{".github/workflows/ci.yml", "tools/changes/", "tools/check/", "go.mod", "go.sum"}

// systemPrefixes hold the code that differs from one operating system to
// another.
var systemPrefixes = []string{"internal/platform/", "internal/adapters/", "internal/obs/", "cmd/ship/", "scripts/"}

// extensionPrefixes are the extension and the MCP surface it calls.
var extensionPrefixes = []string{"extensions/", "internal/surface/mcp/", "schemas/"}

// Classify returns what the changed paths need.
func Classify(paths []string) Needs {
	var n Needs
	for _, p := range paths {
		if hasPrefix(p, ciItself) {
			return everything
		}
		goFile := strings.HasSuffix(p, ".go") || strings.Contains(p, "/testdata/")
		if goFile {
			n.Go = true
		}
		if goFile && hasPrefix(p, systemPrefixes) || strings.HasPrefix(p, "scripts/") {
			n.System = true
		}
		if hasPrefix(p, extensionPrefixes) {
			n.Extension = true
		}
	}
	return n
}

func hasPrefix(p string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if p == prefix || strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// write prints the needs as GitHub Actions outputs.
func write(w io.Writer, n Needs) error {
	_, err := fmt.Fprintf(w, "go=%t\nsystem=%t\nextension=%t\n", n.Go, n.System, n.Extension)
	return err
}

func main() {
	base := flag.String("base", "", "the ref the pull request merges into (origin/main); empty runs everything")
	flag.Parse()
	needs := everything
	if *base != "" {
		out, err := exec.Command("git", "diff", "--name-only", "--no-renames", *base+"...HEAD").Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, "changes: git diff:", err)
			os.Exit(1)
		}
		needs = Classify(strings.Fields(string(out)))
	}
	if err := write(os.Stdout, needs); err != nil {
		os.Exit(1)
	}
}
