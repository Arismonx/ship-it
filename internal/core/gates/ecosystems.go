// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gates

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path"
	"slices"
	"strings"
)

// The checks of other ecosystems follow one rule: ship-it runs what the
// project declares, never a tool it would impose. A formatter or linter runs
// when its configuration is in the repository; a test runner when the
// ecosystem's standard tool runs tests by itself (go test, cargo test) or
// when the project declares it (pytest configuration, an .rspec file, a
// composer script). Each command was checked against the tool's own documentation.

// RootFiles are the files at the repository root that Detect reads, beyond
// package.json: their content tells what the project declares.
var RootFiles = []string{"pyproject.toml", "composer.json", "Gemfile"}

// MarkerFiles are the root entries whose presence alone tells something.
var MarkerFiles = []string{
	"go.mod", "Cargo.toml", "rustfmt.toml", ".rustfmt.toml",
	"uv.lock", "poetry.lock", "pdm.lock", "ruff.toml", ".ruff.toml", "mypy.ini", "pytest.ini", "conftest.py",
	".rubocop.yml", ".rspec",
	"gradlew", "gradlew.bat", "build.gradle", "build.gradle.kts", "mvnw", "mvnw.cmd", "pom.xml",
}

// goGates: gofmt on the changed Go files (it prints the files it would
// change and exits 0, hence FailOnOutput), then go vet and go test on the
// module.
func goGates(p Project) []Gate {
	if !p.Present["go.mod"] {
		return nil
	}
	var gates []Gate
	if files := changedWithExtension(p.Changed, ".go"); len(files) > 0 {
		gates = append(gates, Gate{Name: "gofmt", Command: append([]string{"gofmt", "-l"}, files...), FailOnOutput: true})
	}
	return append(gates,
		Gate{Name: "go vet", Command: []string{"go", "vet", "./..."}},
		Gate{Name: "go test", Command: []string{"go", "test", "./..."}},
	)
}

// rustGates: rustup's default profile installs rustfmt and clippy with
// cargo, so the three are the standard checks of a Rust project. clippy
// fails on errors only: warnings are the project's to promote.
func rustGates(p Project) []Gate {
	if !p.Present["Cargo.toml"] {
		return nil
	}
	return []Gate{
		{Name: "cargo fmt", Command: []string{"cargo", "fmt", "--all", "--", "--check"}},
		{Name: "clippy", Command: []string{"cargo", "clippy", "--all-targets"}},
		{Name: "cargo test", Command: []string{"cargo", "test"}},
	}
}

// pythonGates run the tools pyproject.toml (or their own files) configure,
// through the project's environment manager when it has a lockfile.
func pythonGates(p Project) []Gate {
	sections := tomlSections(p.Files["pyproject.toml"])
	has := func(prefix string) bool {
		return slices.ContainsFunc(sections, func(s string) bool { return s == prefix || strings.HasPrefix(s, prefix+".") })
	}
	var runner []string
	switch {
	case p.Present["uv.lock"]:
		runner = []string{"uv", "run", "--frozen"} // never rewrites uv.lock
	case p.Present["poetry.lock"]:
		runner = []string{"poetry", "run"}
	case p.Present["pdm.lock"]:
		runner = []string{"pdm", "run"}
	}
	tool := func(name string, args ...string) Gate {
		return Gate{Name: name, Command: append(append(slices.Clone(runner), name), args...)}
	}
	var gates []Gate
	if has("tool.ruff") || p.Present["ruff.toml"] || p.Present[".ruff.toml"] {
		gates = append(gates, Gate{Name: "ruff", Command: append(slices.Clone(runner), "ruff", "check", ".")})
		if has("tool.ruff.format") {
			gates = append(gates, Gate{Name: "ruff format", Command: append(slices.Clone(runner), "ruff", "format", "--check", ".")})
		}
	}
	if has("tool.black") {
		gates = append(gates, tool("black", "--check", "."))
	}
	if has("tool.mypy") || p.Present["mypy.ini"] {
		gates = append(gates, tool("mypy", "."))
	}
	if has("tool.pytest.ini_options") || p.Present["pytest.ini"] || p.Present["conftest.py"] {
		gates = append(gates, tool("pytest"))
	}
	return gates
}

// phpScripts are the composer.json scripts that gate a push, cheapest first.
var phpScripts = []string{"lint", "analyse", "phpstan", "test"}

// phpGates run the project's own composer scripts.
func phpGates(p Project) []Gate {
	data := p.Files["composer.json"]
	if data == nil {
		return nil
	}
	var composer struct {
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return nil // composer itself reports a broken composer.json
	}
	var gates []Gate
	for _, name := range phpScripts {
		if _, ok := composer.Scripts[name]; ok {
			gates = append(gates, Gate{Name: "composer " + name, Command: []string{"composer", "run-script", name}})
		}
	}
	return gates
}

// rubyGates: RuboCop when configured, RSpec when the project has an .rspec
// file, both through Bundler as the Gemfile pins them.
func rubyGates(p Project) []Gate {
	if p.Files["Gemfile"] == nil {
		return nil
	}
	var gates []Gate
	if p.Present[".rubocop.yml"] {
		gates = append(gates, Gate{Name: "rubocop", Command: []string{"bundle", "exec", "rubocop"}})
	}
	if p.Present[".rspec"] {
		gates = append(gates, Gate{Name: "rspec", Command: []string{"bundle", "exec", "rspec"}})
	}
	return gates
}

// jvmGates: Gradle's check task (where plugins attach their verification
// tasks) or Maven's verify phase (every phase up to it, tests included),
// through the project's wrapper when it has one.
func jvmGates(p Project) []Gate {
	wrapper := func(unix, windows string) string {
		if p.Windows && p.Present[windows] {
			return "./" + windows
		}
		if !p.Windows && p.Present[unix] {
			return "./" + unix
		}
		return ""
	}
	switch {
	case p.Present["build.gradle"] || p.Present["build.gradle.kts"] || p.Present["gradlew"]:
		gradle := wrapper("gradlew", "gradlew.bat")
		if gradle == "" {
			gradle = "gradle"
		}
		return []Gate{{Name: "gradle check", Command: []string{gradle, "check"}}}
	case p.Present["pom.xml"]:
		maven := wrapper("mvnw", "mvnw.cmd")
		if maven == "" {
			maven = "mvn"
		}
		return []Gate{{Name: "maven verify", Command: []string{maven, "--batch-mode", "verify"}}}
	}
	return nil
}

// dotnetGates: dotnet format and dotnet test find the project or solution in
// the root themselves, which works when there is exactly one; with several,
// the solution is named, and with several solutions ship-it does not guess.
func dotnetGates(p Project) []Gate {
	var solutions, projects []string
	for name := range p.Present {
		switch path.Ext(name) {
		case ".sln", ".slnx":
			solutions = append(solutions, name)
		case ".csproj", ".fsproj", ".vbproj":
			projects = append(projects, name)
		}
	}
	var target []string
	switch {
	case len(solutions) == 1:
		target = solutions
	case len(solutions) == 0 && len(projects) == 1:
		target = projects
	default:
		return nil // none, or ambiguous
	}
	return []Gate{
		{Name: "dotnet format", Command: append([]string{"dotnet", "format"}, append(target, "--verify-no-changes")...)},
		{Name: "dotnet test", Command: append([]string{"dotnet", "test"}, target...)},
	}
}

// changedWithExtension returns the changed paths with ext, sorted.
func changedWithExtension(changed []string, ext string) []string {
	var files []string
	for _, f := range changed {
		if strings.HasSuffix(f, ext) {
			files = append(files, f)
		}
	}
	slices.Sort(files)
	return files
}

// tomlSections lists the table headers of a TOML document ("tool.ruff",
// "tool.ruff.format"…): enough to know which tools a pyproject.toml
// configures, without a TOML parser (ADR-0012). Array-of-tables headers
// ([[tool.x]]) count too; comments and quoted dotted keys are rare in
// pyproject headers and simply do not match.
func tomlSections(data []byte) []string {
	var sections []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") {
			continue
		}
		line, _, _ = strings.Cut(line, "#")
		header := strings.Trim(strings.TrimSpace(line), "[]")
		header = strings.ReplaceAll(header, " ", "")
		if header != "" {
			sections = append(sections, header)
		}
	}
	return sections
}
