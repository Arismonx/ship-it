// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package risk

import (
	"reflect"
	"testing"
)

func keys(a Assessment) []string {
	var k []string
	for _, f := range a.Factors {
		k = append(k, f.Key)
	}
	return k
}

func TestAssess(t *testing.T) {
	for _, tt := range []struct {
		name   string
		in     Input
		level  string
		points int
		keys   []string
	}{
		{"a small change with its test", Input{ChangedLines: 20, Files: []File{{Path: "vat.go"}, {Path: "vat_test.go"}}}, Low, 0, nil},
		{"docs only", Input{ChangedLines: 300, Files: []File{{Path: "docs/vat.md"}}}, Low, 1, []string{"risk.size"}},
		{"code without tests", Input{ChangedLines: 40, Files: []File{{Path: "vat.go"}}}, Medium, 2, []string{"risk.untested"}},
		{"a big change without tests", Input{ChangedLines: 800, Files: []File{{Path: "vat.go"}}}, High, 4, []string{"risk.size", "risk.untested"}},
		{"a workflow and a dependency", Input{ChangedLines: 5, Files: []File{{Path: ".github/workflows/ci.yml"}, {Path: "go.mod"}}}, Medium, 2, []string{"risk.sensitive"}},
		{"a migration, by the default patterns", Input{ChangedLines: 5, Files: []File{{Path: "db/migrations/001_users.sql"}, {Path: "db/migrations/001_test.go"}}, SensitivePaths: DefaultSensitivePaths}, Medium, 2, []string{"risk.sensitive"}},
		{"removals", Input{ChangedLines: 10, Files: []File{{Path: "legacy.go", Removed: true}}, RemovedDeclarations: 2}, Low, 1, []string{"risk.removals"}},
	} {
		got := Assess(tt.in)
		if got.Level != tt.level || got.Points != tt.points || !reflect.DeepEqual(keys(got), tt.keys) {
			t.Errorf("%s: %+v, want %s with %d points from %v", tt.name, got, tt.level, tt.points, tt.keys)
		}
	}
}

func TestSensitiveFilesAreListed(t *testing.T) {
	files := []File{{Path: "a.sql"}, {Path: "b.sql"}, {Path: "c.sql"}, {Path: "d.sql"}, {Path: "e.sql"}}
	got := Assess(Input{Files: files, SensitivePaths: []string{"**/*.sql"}})
	var listed string
	for _, f := range got.Factors {
		if f.Key == "risk.sensitive" {
			listed = f.Params["files"]
		}
	}
	if listed != "a.sql, b.sql, c.sql (+2)" {
		t.Fatalf("factors = %+v", got.Factors)
	}
}
