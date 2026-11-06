// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/GregoireF/ship-it/"

// layer describes what a group of packages may import (ARCHITECTURE.md).
type layer struct {
	name string
	// owns reports whether a package (path relative to the module) belongs here.
	owns func(rel string) bool
	// allowModule reports whether an import inside the module is allowed.
	allowModule func(rel, imported string) bool
	// allowExternal reports whether a non-module import is allowed.
	allowExternal func(rel, imported string) bool
}

// ioStdlib lists the standard packages that perform I/O, forbidden in core.
var ioStdlib = []string{"os", "os/exec", "os/signal", "net", "syscall", "log", "plugin"}

// pureUnderIO are packages below an I/O path that only parse values.
var pureUnderIO = map[string]bool{"net/url": true, "net/netip": true}

func isStdlib(path string) bool { return !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") }

func hasPrefixPath(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func stdlibWithoutIO(_, imported string) bool {
	if !isStdlib(imported) {
		return false
	}
	if pureUnderIO[imported] {
		return true
	}
	for _, forbidden := range ioStdlib {
		if hasPrefixPath(imported, forbidden) {
			return false
		}
	}
	return true
}

func onlyStdlib(_, imported string) bool { return isStdlib(imported) }

func stdlibOrGolangX(_, imported string) bool {
	return isStdlib(imported) || strings.HasPrefix(imported, "golang.org/x/")
}

func within(prefixes ...string) func(string, string) bool {
	return func(_, imported string) bool {
		for _, p := range prefixes {
			if hasPrefixPath(imported, p) {
				return true
			}
		}
		return false
	}
}

func under(prefix string) func(string) bool {
	return func(rel string) bool { return hasPrefixPath(rel, prefix) }
}

var layers = []layer{
	{"core", under("internal/core"), within("internal/core"), stdlibWithoutIO},
	{"ports", under("internal/ports"), within("internal/core"), onlyStdlib},
	{"app", under("internal/app"), within("internal/core", "internal/ports", "internal/i18n"),
		func(rel, imported string) bool {
			return isStdlib(imported) && !hasPrefixPath(imported, "os/exec") && !hasPrefixPath(imported, "net")
		}},
	{"adapters", under("internal/adapters"),
		func(rel, imported string) bool {
			// An adapter may import its own subpackages but never another adapter.
			own := strings.Join(strings.SplitN(rel, "/", 4)[:3], "/")
			return hasPrefixPath(imported, own) ||
				within("internal/ports", "internal/core", "internal/platform", "internal/obs")(rel, imported)
		},
		stdlibOrGolangX},
	{"surface", under("internal/surface"),
		within("internal/app", "internal/ports", "internal/schema", "internal/i18n", "internal/platform", "internal/obs"),
		func(rel, imported string) bool {
			if strings.HasPrefix(imported, "charm.land/") {
				return hasPrefixPath(rel, "internal/surface/cli") // ADR-0012
			}
			return stdlibOrGolangX(rel, imported)
		}},
	{"schema", under("internal/schema"), within(), onlyStdlib},
	{"i18n", under("internal/i18n"), within(), onlyStdlib},
	{"platform", under("internal/platform"), within(), stdlibOrGolangX},
	{"obs", under("internal/obs"), within("internal/platform"), onlyStdlib},
	{"testkit", under("internal/testkit"), func(string, string) bool { return true }, func(string, string) bool { return true }},
	{"archtest", under("internal/archtest"), func(string, string) bool { return true }, func(string, string) bool { return true }},
	{"cmd", under("cmd"), func(string, string) bool { return true }, func(string, string) bool { return true }},
	// Developer tasks (go run ./tools/<task>), never part of the binary.
	{"tools", under("tools"), func(string, string) bool { return true }, func(string, string) bool { return true }},
}

// isCharm reports whether a package belongs to Charm (Bubble Tea, Lip Gloss
// and their own modules), allowed in the CLI surface only (ADR-0012).
func isCharm(path string) bool {
	return strings.HasPrefix(path, "charm.land/") || strings.HasPrefix(path, "github.com/charmbracelet/")
}

type goPackage struct {
	ImportPath string
	Imports    []string
	// Deps are every package it depends on, transitively.
	Deps []string
}

func listPackages(t *testing.T) []goPackage {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, stderr.String())
	}
	var pkgs []goPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p goPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

func TestImportRules(t *testing.T) {
	for _, pkg := range listPackages(t) {
		rel := strings.TrimPrefix(pkg.ImportPath, module)
		var owner *layer
		for i := range layers {
			if layers[i].owns(rel) {
				owner = &layers[i]
				break
			}
		}
		if owner == nil {
			t.Errorf("%s belongs to no layer of ARCHITECTURE.md", rel)
			continue
		}
		for _, imported := range pkg.Imports {
			if strings.HasPrefix(imported, module) {
				target := strings.TrimPrefix(imported, module)
				// tools/internal holds what the developer tasks share (pinned
				// versions); Go itself keeps it out of reach of other packages.
				sharedTools := hasPrefixPath(target, "tools/internal") && hasPrefixPath(rel, "tools")
				if hasPrefixPath(target, "cmd") || (hasPrefixPath(target, "tools") && !sharedTools) {
					t.Errorf("%s imports %s: nothing may import cmd/ or tools/ (except tools/internal from tools/)", rel, target)
				}
				if hasPrefixPath(target, "internal/testkit") {
					t.Errorf("%s imports %s outside a _test.go file", rel, target)
				}
				if !owner.allowModule(rel, target) {
					t.Errorf("%s (%s layer) must not import %s", rel, owner.name, target)
				}
				continue
			}
			if !owner.allowExternal(rel, imported) {
				t.Errorf("%s (%s layer) must not import %s", rel, owner.name, imported)
			}
		}
	}
}

// TestRulesCatchViolations guards the guard: known-bad imports are rejected.
func TestRulesCatchViolations(t *testing.T) {
	core, adapters, surface := layers[0], layers[3], layers[4]
	if core.allowExternal("internal/core/plan", "os/exec") {
		t.Error("core may not import os/exec")
	}
	if core.allowModule("internal/core/plan", "internal/adapters/github") {
		t.Error("core may not import adapters")
	}
	if adapters.allowModule("internal/adapters/github", "internal/adapters/gitexec") {
		t.Error("an adapter may not import another adapter")
	}
	if surface.allowExternal("internal/surface/mcp", "charm.land/bubbletea/v2") {
		t.Error("only surface/cli may import Charm")
	}
	if !surface.allowExternal("internal/surface/cli", "charm.land/bubbletea/v2") {
		t.Error("surface/cli may import Charm")
	}
	if surface.allowExternal("internal/surface/mcp", "github.com/charmbracelet/x/ansi") {
		t.Error("only surface/cli may import Charm's x modules")
	}
}

// TestCharmStaysInTheCLI checks the transitive closure: no package reaches
// Charm except the CLI surface and the composition root that starts it, so
// ship mcp, the app and the core never pull a TUI (ADR-0012).
func TestCharmStaysInTheCLI(t *testing.T) {
	reached := false
	for _, pkg := range listPackages(t) {
		rel := strings.TrimPrefix(pkg.ImportPath, module)
		allowed := hasPrefixPath(rel, "internal/surface/cli") || rel == "cmd/ship"
		for _, dep := range pkg.Deps {
			if !isCharm(dep) {
				continue
			}
			if !allowed {
				t.Errorf("%s depends on %s: Charm is for internal/surface/cli only (ADR-0012)", rel, dep)
				break
			}
			reached = true
		}
	}
	if !reached {
		t.Error("no package depends on Charm: update ADR-0012 and this test")
	}
}
