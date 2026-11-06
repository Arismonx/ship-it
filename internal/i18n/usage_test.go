// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// keyLiteral matches a string literal that looks like a message key. Literals
// ending with "." or "_" are prefixes completed at run time (remedy per OS,
// plan category…) and are checked by the tests of their packages.
var keyLiteral = regexp.MustCompile(`"((?:app|checkpoint|cli|config|convention|deliver|doctor|plan|publish|secrets|stack|status|sync|undo)\.[a-z0-9_.\-]*[a-z0-9])"`)

var fileName = regexp.MustCompile(`\.(json|go|md|ya?ml|txt)$`)

// TestEveryKeyUsedInTheCodeExists catches a key typed in Go but missing from
// the catalogue, which would show the raw key to the user.
func TestEveryKeyUsedInTheCodeExists(t *testing.T) {
	known := Keys()
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && path != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasPrefix(filepath.Base(path), "catalog") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range keyLiteral.FindAllStringSubmatch(string(source), -1) {
			if fileName.MatchString(match[1]) {
				continue // "config.json" is a file, not a key
			}
			if !slices.Contains(known, match[1]) {
				t.Errorf("%s uses %q, missing from the catalogue", path, match[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
