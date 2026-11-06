// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pins

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// marker finds "<version> # pin: <name>"; the version may follow "@" or ": ".
var marker = regexp.MustCompile(`(v?\d+\.\d+\.\d+[0-9A-Za-z.+-]*)\s+# pin: ([a-z0-9-]+)`)

// configFiles lists the YAML files that may use a pinned version.
func configFiles(t *testing.T, root string) []string {
	t.Helper()
	files := []string{filepath.Join(root, ".goreleaser.yaml")}
	for _, dir := range []string{".github", "actions"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == filepath.Join(root, dir) {
				return filepath.SkipDir // no such directory: nothing to check
			}
			if err != nil {
				return err
			}
			if !d.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// TestMarkersMatchPins keeps the workflows in step with this package: every
// marker names a known tool with its pinned version, every tool is used.
func TestMarkersMatchPins(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	known := map[string]Tool{}
	for _, tool := range Tools {
		known[tool.Name] = tool
	}
	used := map[string]bool{}
	for _, path := range configFiles(t, root) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range marker.FindAllStringSubmatch(string(data), -1) {
			version, name := m[1], m[2]
			tool, ok := known[name]
			switch {
			case !ok:
				t.Errorf("%s: unknown pin %q", path, name)
			case version != tool.Version:
				t.Errorf("%s: %s is %s, tools/internal/pins says %s", path, name, version, tool.Version)
			}
			used[name] = true
		}
		// A pinned tool's version must not appear without its marker.
		for _, tool := range Tools {
			if n := strings.Count(string(data), tool.Name+"@"+tool.Version) +
				strings.Count(string(data), "version: "+tool.Version); n > 0 {
				withMarker := len(regexp.MustCompile(regexp.QuoteMeta(tool.Version)+`\s+# pin: `+tool.Name).FindAllString(string(data), -1))
				if withMarker < n {
					t.Errorf("%s: %s %s used without a \"# pin: %s\" marker", path, tool.Name, tool.Version, tool.Name)
				}
			}
		}
	}
	for _, tool := range Tools {
		if !used[tool.Name] {
			t.Errorf("pin %s is used by no workflow", tool.Name)
		}
	}
}

func TestMarkerSyntax(t *testing.T) {
	for line, want := range map[string][2]string{
		"version: 1.31.1 # pin: zizmor":                              {"1.31.1", "zizmor"},
		"run: go run x/cmd/actionlint@v1.7.12 # pin: actionlint":     {"v1.7.12", "actionlint"},
		"cosign-release: v3.1.3 # pin: cosign (verified by install)": {"v3.1.3", "cosign"},
	} {
		m := marker.FindStringSubmatch(line)
		if m == nil || m[1] != want[0] || m[2] != want[1] {
			t.Errorf("marker(%q) = %q, want %q", line, m, want)
		}
	}
}
