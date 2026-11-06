// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package snapshot

import (
	"reflect"
	"testing"
)

const sampleDiff = `diff --git a/internal/app/run.go b/internal/app/run.go
index 1111111..2222222 100644
--- a/internal/app/run.go
+++ b/internal/app/run.go
@@ -10,3 +10,4 @@ func Run() {
 	a := 1
-	b := 2
+	b := 3
+	// fix: avoid nil config
 	return
diff --git a/docs/guide.md b/docs/guide.md
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/docs/guide.md
@@ -0,0 +1,2 @@
+# Guide
+Hello
diff --git a/old.txt b/old.txt
deleted file mode 100644
index 4444444..0000000
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/a/name.go b/b/name.go
similarity index 100%
rename from a/name.go
rename to b/name.go
diff --git a/logo.png b/logo.png
new file mode 100644
index 0000000..5555555
Binary files /dev/null and b/logo.png differ
`

func TestParseUnifiedDiff(t *testing.T) {
	files := ParseUnifiedDiff(sampleDiff)
	if len(files) != 5 {
		t.Fatalf("got %d files: %+v", len(files), files)
	}
	run := files[0]
	if run.Path != "internal/app/run.go" || run.Status != Modified {
		t.Errorf("run.go = %+v", run)
	}
	wantAdded := []Line{{Number: 11, Text: "\tb := 3"}, {Number: 12, Text: "\t// fix: avoid nil config"}}
	if !reflect.DeepEqual(run.Added, wantAdded) || !reflect.DeepEqual(run.Removed, []string{"\tb := 2"}) {
		t.Errorf("run.go lines = %+v / %+v", run.Added, run.Removed)
	}
	if files[1].Status != Added || files[1].Path != "docs/guide.md" || len(files[1].Added) != 2 {
		t.Errorf("guide.md = %+v", files[1])
	}
	if files[2].Status != Deleted || files[2].Path != "old.txt" {
		t.Errorf("old.txt = %+v", files[2])
	}
	if files[3].Status != Renamed || files[3].Path != "b/name.go" || files[3].OriginalPath != "a/name.go" {
		t.Errorf("rename = %+v", files[3])
	}
	if !files[4].Binary || files[4].Status != Added {
		t.Errorf("logo.png = %+v", files[4])
	}
}

func TestCategorize(t *testing.T) {
	cases := map[string]Category{
		"internal/app/run.go":      SourceCode,
		"internal/app/run_test.go": Tests,
		"src/button.spec.ts":       Tests,
		"testdata/script/x.txtar":  Tests,
		"README.md":                Documentation,
		"docs/adr/0001.md":         Documentation,
		"LICENSE":                  Documentation,
		"src/app.scss":             Stylesheets,
		".github/workflows/ci.yml": ContinuousIntegration,
		"package.json":             Dependencies,
		"go.sum":                   Dependencies,
		"assets/logo.svg":          Assets,
		".editorconfig":            Configuration,
		"tsconfig.json":            Configuration,
		"components/AppHeader.vue": SourceCode,
	}
	for p, want := range cases {
		if got := Categorize(p); got != want {
			t.Errorf("Categorize(%q) = %d, want %d", p, got, want)
		}
	}
}

func TestNewMarksGenerated(t *testing.T) {
	s := New([]FileDiff{{Path: "pnpm-lock.yaml"}, {Path: "schemas/a.json"}, {Path: "main.go"}},
		Options{GeneratedPaths: map[string]bool{"schemas/a.json": true}})
	got := []bool{s.Files[0].Generated, s.Files[1].Generated, s.Files[2].Generated}
	if !reflect.DeepEqual(got, []bool{true, true, false}) {
		t.Fatalf("generated flags = %v", got)
	}
}

func FuzzParseUnifiedDiff(f *testing.F) {
	f.Add(sampleDiff)
	f.Add("@@ -1 +1 @@\n+x")
	f.Add("diff --git a b\n@@ -0,0 +")
	f.Fuzz(func(t *testing.T, diff string) {
		_ = ParseUnifiedDiff(diff) // must never panic
	})
}
