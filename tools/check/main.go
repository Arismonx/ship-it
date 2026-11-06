// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command check runs every verification of the repository, the same way on a
// laptop and in CI (docs/design/workflows.md). Run it from the module root:
//
//	go run ./tools/check            # what a pull request must pass
//	go run ./tools/check -race      # with the race detector (needs cgo)
//	go run ./tools/check -fuzztime=10s
//	go run ./tools/check -workflows # also lint .github (zizmor needs uvx)
//	go run ./tools/check -bench     # also check the performance budgets
//
// Steps stop at the first failure.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/tools/internal/fuzztargets"
	"github.com/GregoireF/ship-it/tools/internal/pins"
)

// licenseIgnore lists what carries no SPDX header: documentation, generated
// files and test data (decision G3 covers the code).
const licenseIgnore = "*.md,*.json,*.svg,*.txt,*.html,LICENSE,testdata,schemas,assets,.editorconfig,.gitattributes,.gitignore," +
	// Build output of the extension, never committed.
	"extensions/vscode/out,extensions/vscode/dist,extensions/vscode/node_modules," +
	// Output of a local GoReleaser run (.goreleaser.yaml), never committed.
	"dist," +
	// Local Claude Code settings and agent worktrees, never committed.
	".claude"

type step struct {
	name string
	run  func() error
}

func main() {
	race := flag.Bool("race", false, "run the tests with the race detector")
	fuzzTime := flag.Duration("fuzztime", 0, "fuzz each target for this long (0: skip)")
	workflows := flag.Bool("workflows", false, "also lint the GitHub workflows with actionlint and zizmor (needs uvx)")
	bench := flag.Bool("bench", false, "also run the benchmarks against tools/bench/budgets.json (a few minutes)")
	flag.Parse()

	testArgs := []string{"test", "-count=1"}
	if *race {
		testArgs = append(testArgs, "-race")
	}
	steps := []step{
		{"gofmt", checkFormat},
		{"vet", command("go", "vet", "./...")},
		{"staticcheck", command("go", "tool", "staticcheck", "./...")},
		{"license headers", command("go", "tool", "addlicense", "--check", "--license", "Apache-2.0",
			"--author", "Grégoire Favreau", "--ignore", licenseIgnore, ".")},
		{"tests", command("go", append(testArgs, "./...")...)},
	}
	if *fuzzTime > 0 {
		steps = append(steps, step{"fuzz", func() error { return fuzz(*fuzzTime) }})
	}
	// Workflow linters (docs/design/workflows.md §5), at the versions of
	// tools/internal/pins. Neither is a module dependency: actionlint is built
	// by `go run pkg@version` (verified against sum.golang.org, go.mod
	// untouched) and zizmor, written in Rust, runs through uv.
	if *workflows {
		steps = append(steps,
			step{"actionlint", command("go", "run", pins.ActionlintPackage())},
			// pedantic: stricter than CI (regular), cheap to keep clean.
			step{"zizmor", command("uvx", pins.ZizmorPackage(), "--persona=pedantic", ".")},
		)
	}
	// Performance budgets (docs/design/performance.md): time ceilings and git
	// processes per command.
	if *bench {
		steps = append(steps, step{"bench", command("go", "run", "./tools/bench")})
	}
	steps = append(steps, step{"govulncheck", command("go", "tool", "govulncheck", "./...")})

	for _, s := range steps {
		start := time.Now()
		fmt.Printf("── %s\n", s.name)
		if err := s.run(); err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", s.name, err)
			os.Exit(1)
		}
		fmt.Printf("✓ %s (%s)\n", s.name, time.Since(start).Round(time.Millisecond))
	}
}

func command(name string, args ...string) func() error {
	return func() error {
		cmd := exec.Command(name, args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
}

func checkFormat() error {
	// The package directories of the module, as ./... sees them: dot
	// directories (.claude worktrees, .git) are left out, as go does.
	dirs, err := exec.Command("go", "list", "-f", "{{.Dir}}", "./...").Output()
	if err != nil {
		return err
	}
	args := []string{"-l"}
	for _, dir := range strings.Split(strings.TrimSpace(string(dirs)), "\n") {
		dir = strings.TrimSpace(dir) // one per line: paths may hold spaces
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		args = append(args, files...)
	}
	out, err := exec.Command("gofmt", args...).Output()
	if err != nil {
		return err
	}
	if files := strings.TrimSpace(string(out)); files != "" {
		return fmt.Errorf("not gofmt-ed:\n%s", files)
	}
	return nil
}

// fuzz finds every Fuzz target with `go test -list` and runs each briefly.
func fuzz(duration time.Duration) error {
	targets, err := fuzztargets.List(".")
	if err != nil {
		return err
	}
	for _, t := range targets {
		fmt.Printf("   fuzz %s %s\n", t.Package, t.Name)
		if err := command("go", "test", "-run=^$", "-fuzz=^"+t.Name+"$", "-fuzztime="+duration.String(), t.Package)(); err != nil {
			return fmt.Errorf("%s in %s: %w", t.Name, t.Package, err)
		}
	}
	return nil
}
