// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package benchdata generates the realistic, deterministic inputs of the
// benchmarks (docs/design/performance.md): paths of a monorepo, a unified diff,
// lines of code, and a git repository with history and work in progress.
//
// Test files only (internal/testkit): nothing of it reaches the binary.
package benchdata

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// seed makes every generated input the same from one run to the next, so
// that numbers compare.
const seed = 2026

func newRand() *rand.Rand { return rand.New(rand.NewPCG(seed, seed)) }

// roots, dirs and leaves build the paths of a typical monorepo: Go and
// TypeScript sources, their tests, docs, CI, configuration and lockfiles.
var (
	roots = []string{
		"apps/web/src", "apps/web/tests", "apps/api/internal", "packages/ui/src", "packages/ui/src/components",
		"internal/core", "internal/adapters", "cmd/server", "docs", "docs/guides", "scripts", ".github/workflows", "test/e2e",
	}
	dirs   = []string{"", "auth", "billing", "search", "users", "shared", "orders", "reports", "util", "config"}
	leaves = []string{
		"handler.go", "handler_test.go", "service.go", "store.go", "index.ts", "index.spec.ts", "Button.vue",
		"README.md", "guide.md", "styles.css", "settings.json", "ci.yml", "release.yml", "run.sh", "logo.svg",
	}
)

// Paths returns n distinct slash-separated paths.
func Paths(n int) []string {
	r := newRand()
	seen := make(map[string]bool, n)
	paths := make([]string, 0, n)
	for i := 0; len(paths) < n; i++ {
		root, dir, leaf := roots[r.IntN(len(roots))], dirs[r.IntN(len(dirs))], leaves[r.IntN(len(leaves))]
		p := root
		if dir != "" {
			p += "/" + dir
		}
		if i >= len(roots)*len(dirs)*len(leaves)/4 {
			// Past the first combinations, number the files so they stay distinct.
			ext := leaf[strings.LastIndexByte(leaf, '.'):]
			leaf = fmt.Sprintf("%s_%d%s", strings.TrimSuffix(leaf, ext), i, ext)
		}
		p += "/" + leaf
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// codeLines are what source files look like to the secret scanner: mostly
// code, a few assignments that look like secrets but are not (references,
// placeholders, identifiers), and URLs.
var codeLines = []string{
	"\treturn fmt.Errorf(\"load %s: %w\", name, err)",
	"\tif err != nil {",
	"\t}",
	"\tctx, cancel := context.WithTimeout(ctx, 5*time.Second)",
	"\tdefer cancel()",
	"export function formatPrice(amount: number, currency = 'EUR'): string {",
	"  const apiKey = config.public.apiKey",
	"  token: process.env.GITHUB_TOKEN,",
	"\tpassword := os.Getenv(\"DB_PASSWORD\")",
	"const SECRET_HEADER = 'X-Request-Signature'",
	"  <Button variant=\"primary\" @click=\"submit\">{{ t('form.submit') }}</Button>",
	"See https://docs.example.com/guides/getting-started for the details.",
	"  - run: go test -race ./...",
	"\tusers, err := s.store.ListUsers(ctx, ListOptions{Limit: 50, Cursor: cursor})",
	"  .card { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; }",
	"// accessKey is read from the vault at start-up, never from the repository.",
	"\tfor i, item := range items {",
	"",
}

// Line returns a line of code-like text; r picks it.
func Line(r *rand.Rand) string { return codeLines[r.IntN(len(codeLines))] }

// FileContent returns lines lines of code-like text, newline-terminated.
func FileContent(r *rand.Rand, lines int) string {
	var b strings.Builder
	for range lines {
		b.WriteString(Line(r))
		b.WriteByte('\n')
	}
	return b.String()
}

// Diff returns what `git diff HEAD` prints for files changed files with
// changed lines added and as many removed each, in one hunk with three lines
// of context around: Diff(500, 6) is about 10,000 lines. One file in 50 is
// new, one in 97 deleted, one in 61 renamed, one in 89 binary.
func Diff(files, changed int) string {
	r := newRand()
	var b strings.Builder
	for i, p := range Paths(files) {
		switch {
		case i%50 == 49:
			fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode 100644\nindex 0000000..1a2b3c4\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", p, p, p, changed)
			for range changed {
				b.WriteString("+" + Line(r) + "\n")
			}
		case i%97 == 96:
			fmt.Fprintf(&b, "diff --git a/%s b/%s\ndeleted file mode 100644\nindex 1a2b3c4..0000000\n--- a/%s\n+++ /dev/null\n@@ -1,%d +0,0 @@\n", p, p, p, changed)
			for range changed {
				b.WriteString("-" + Line(r) + "\n")
			}
		case i%89 == 88:
			fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1a2b3c4..5d6e7f8 100644\nBinary files a/%s and b/%s differ\n", p, p, p, p)
		default:
			from := p
			if i%61 == 60 {
				from = "old/" + p
				fmt.Fprintf(&b, "diff --git a/%s b/%s\nsimilarity index 88%%\nrename from %s\nrename to %s\nindex 1a2b3c4..5d6e7f8 100644\n", from, p, from, p)
			} else {
				fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1a2b3c4..5d6e7f8 100644\n", p, p)
			}
			start := 10 + r.IntN(200)
			fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n@@ -%d,%d +%d,%d @@ func example() {\n", from, p, start, changed+6, start, changed+6)
			for range 3 {
				b.WriteString(" " + Line(r) + "\n")
			}
			for range changed {
				b.WriteString("-" + Line(r) + "\n")
			}
			for range changed {
				b.WriteString("+" + Line(r) + "\n")
			}
			for range 3 {
				b.WriteString(" " + Line(r) + "\n")
			}
		}
	}
	return b.String()
}
