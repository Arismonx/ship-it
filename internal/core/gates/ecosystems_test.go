// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gates

import (
	"reflect"
	"strings"
	"testing"
)

func present(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// commands renders the gates as "name: command" lines for compact tests.
func commands(t *testing.T, p Project) []string {
	t.Helper()
	d, err := Detect(p)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, g := range d.Gates {
		line := g.Name + ": " + strings.Join(g.Command, " ")
		if g.FailOnOutput {
			line += " (output fails)"
		}
		out = append(out, line)
	}
	return out
}

func TestEcosystemGates(t *testing.T) {
	for _, tt := range []struct {
		name string
		p    Project
		want []string
	}{
		{"nothing declared", Project{}, nil},
		{"go, gofmt on changed files only", Project{Present: present("go.mod"), Changed: []string{"b.go", "README.md", "a/a.go"}},
			[]string{"gofmt: gofmt -l a/a.go b.go (output fails)", "go vet: go vet ./...", "go test: go test ./..."}},
		{"go, no Go file changed", Project{Present: present("go.mod"), Changed: []string{"README.md"}},
			[]string{"go vet: go vet ./...", "go test: go test ./..."}},
		{"rust", Project{Present: present("Cargo.toml")},
			[]string{"cargo fmt: cargo fmt --all -- --check", "clippy: cargo clippy --all-targets", "cargo test: cargo test"}},
		{"python with uv", Project{Present: present("uv.lock"), Files: map[string][]byte{"pyproject.toml": []byte(
			"[project]\nname = \"x\"\n\n[tool.ruff]\nline-length = 100\n[tool.ruff.format] # quotes\n[tool.mypy]\n[tool.pytest.ini_options]\n")}},
			[]string{"ruff: uv run --frozen ruff check .", "ruff format: uv run --frozen ruff format --check .",
				"mypy: uv run --frozen mypy .", "pytest: uv run --frozen pytest"}},
		{"python with poetry and black, pytest by conftest", Project{Present: present("poetry.lock", "conftest.py"),
			Files: map[string][]byte{"pyproject.toml": []byte("[tool.black]\n")}},
			[]string{"black: poetry run black --check .", "pytest: poetry run pytest"}},
		{"ruff.toml without pyproject", Project{Present: present("ruff.toml")}, []string{"ruff: ruff check ."}},
		{"php scripts", Project{Files: map[string][]byte{"composer.json": []byte(`{"scripts": {"test": "pest", "analyse": "phpstan", "post-install-cmd": "x"}}`)}},
			[]string{"composer analyse: composer run-script analyse", "composer test: composer run-script test"}},
		{"ruby", Project{Present: present(".rubocop.yml", ".rspec"), Files: map[string][]byte{"Gemfile": []byte("source 'https://rubygems.org'\n")}},
			[]string{"rubocop: bundle exec rubocop", "rspec: bundle exec rspec"}},
		{"gradle wrapper", Project{Present: present("build.gradle.kts", "gradlew", "gradlew.bat")}, []string{"gradle check: ./gradlew check"}},
		{"gradle wrapper on windows", Project{Windows: true, Present: present("build.gradle.kts", "gradlew", "gradlew.bat")}, []string{"gradle check: ./gradlew.bat check"}},
		{"maven without wrapper", Project{Present: present("pom.xml")}, []string{"maven verify: mvn --batch-mode verify"}},
		{"dotnet solution", Project{Present: present("Shop.sln", "Shop.csproj")},
			[]string{"dotnet format: dotnet format Shop.sln --verify-no-changes", "dotnet test: dotnet test Shop.sln"}},
		{"dotnet ambiguous", Project{Present: present("A.sln", "B.sln")}, nil},
		{"several ecosystems", Project{Present: present("go.mod"), PackageJSON: []byte(`{"scripts": {"lint": "eslint ."}}`)},
			[]string{"lint: npm run lint", "go vet: go vet ./...", "go test: go test ./..."}},
	} {
		if got := commands(t, tt.p); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, got, tt.want)
		}
	}
}

func TestTOMLSections(t *testing.T) {
	got := tomlSections([]byte("# [not.a.header]\n[project]\n  [ tool.ruff ]  # comment\n[[tool.mypy.overrides]]\nkey = \"[x]\"\n"))
	want := []string{"project", "tool.ruff", "tool.mypy.overrides"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tomlSections = %q, want %q", got, want)
	}
}

// FuzzTOMLSections: any input is scanned without panicking.
func FuzzTOMLSections(f *testing.F) {
	f.Add("[tool.ruff]\n[[a.b]]\n")
	f.Fuzz(func(t *testing.T, s string) { tomlSections([]byte(s)) })
}
