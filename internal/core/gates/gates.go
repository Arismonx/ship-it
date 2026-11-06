// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package gates decides which checks of the project run before a push, from
// what the repository declares. It never runs anything: app does, through a
// port. Node projects run their package.json scripts lint, typecheck and
// test, as in the nuxt-core draft; Go, Rust, Python, PHP, Ruby, JVM and .NET
// projects run what they configure (ecosystems.go, decision of 2026-10-25:
// "as many as possible by default").
//
// Allowed imports: standard library without I/O, other core packages.
package gates

import (
	"encoding/json"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

// Gate is one check: a name and the exact command, never run through a shell.
type Gate struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
	// FailOnOutput: the tool reports problems by printing them and exits 0
	// (gofmt -l), so any output is a failure.
	FailOnOutput bool `json:"fail_on_output,omitempty"`
}

// scriptGates are the package.json scripts that gate a push, in order: the
// cheapest first, so that a lint error is reported before a long test run.
var scriptGates = []string{"lint", "typecheck", "test"}

// PackageManager runs the scripts.
type PackageManager string

// Package managers, from the packageManager field (Corepack: npm, pnpm,
// yarn) or the lockfile (bun too).
const (
	NPM  PackageManager = "npm"
	PNPM PackageManager = "pnpm"
	Yarn PackageManager = "yarn"
	Bun  PackageManager = "bun"
)

// lockfiles map each lockfile to its package manager, most specific first.
var lockfiles = []struct {
	file    string
	manager PackageManager
}{
	{"pnpm-lock.yaml", PNPM},
	{"yarn.lock", Yarn},
	{"bun.lock", Bun},
	{"bun.lockb", Bun},
	{"package-lock.json", NPM},
}

// Lockfiles returns the file names Detect wants to know about.
func Lockfiles() []string {
	names := make([]string, len(lockfiles))
	for i, l := range lockfiles {
		names[i] = l.file
	}
	return names
}

// Project is what the repository root holds.
type Project struct {
	// PackageJSON is the content of package.json, nil when absent.
	PackageJSON []byte
	// Present lists the lockfiles that exist.
	Present map[string]bool
	// Base is the ref the work is compared to ("origin/main"), "" if unknown.
	Base string
	// Files holds the content of the RootFiles that exist.
	Files map[string][]byte
	// Changed are the files of the work that still exist, relative to the
	// root: formatters check those only.
	Changed []string
	// Windows selects the .bat/.cmd wrappers.
	Windows bool
}

// Detection is the outcome: the gates to run and the scripts not found.
type Detection struct {
	Manager PackageManager `json:"manager,omitempty"`
	Gates   []Gate         `json:"gates"`
	// Missing are gate scripts the project does not define: skipped.
	Missing []string `json:"missing,omitempty"`
}

// Detect returns the gates of the project: Node first, then the other
// ecosystems the root declares, in a fixed order. A repository may hold
// several (a Go API with a Node front end).
func Detect(p Project) (Detection, error) {
	d, err := detectNode(p)
	if err != nil {
		return d, err
	}
	for _, ecosystem := range []func(Project) []Gate{goGates, rustGates, pythonGates, phpGates, rubyGates, jvmGates, dotnetGates} {
		d.Gates = append(d.Gates, ecosystem(p)...)
	}
	return d, nil
}

// detectNode returns the gates of package.json, or none without it.
func detectNode(p Project) (Detection, error) {
	if p.PackageJSON == nil {
		return Detection{}, nil
	}
	var pkg struct {
		PackageManager string            `json:"packageManager"`
		Scripts        map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(p.PackageJSON, &pkg); err != nil {
		return Detection{}, fault.New(fault.ConfigInvalidJSON, "config.invalid_json",
			"file", "package.json", "line", "1", "column", "1", "detail", err.Error())
	}
	d := Detection{Manager: manager(pkg.PackageManager, p.Present)}
	for _, name := range scriptGates {
		script, ok := pkg.Scripts[name]
		if !ok || strings.TrimSpace(script) == "" {
			d.Missing = append(d.Missing, name)
			continue
		}
		command := []string{string(d.Manager), "run", name}
		if name == "test" && strings.Contains(script, "vitest") && p.Base != "" {
			// Only the tests affected by the work, as the draft did.
			extra := []string{"--changed", p.Base, "--passWithNoTests"}
			if d.Manager == NPM {
				extra = append([]string{"--"}, extra...) // npm needs -- to pass arguments
			}
			command = append(command, extra...)
		}
		d.Gates = append(d.Gates, Gate{Name: name, Command: command})
	}
	return d, nil
}

// manager reads packageManager ("pnpm@9.1.0+sha512…"), else the lockfile,
// else npm, which ships with Node.
func manager(field string, present map[string]bool) PackageManager {
	if name, _, ok := strings.Cut(field, "@"); ok {
		switch m := PackageManager(strings.TrimSpace(name)); m {
		case NPM, PNPM, Yarn, Bun:
			return m
		}
	}
	for _, l := range lockfiles {
		if present[l.file] {
			return l.manager
		}
	}
	return NPM
}
