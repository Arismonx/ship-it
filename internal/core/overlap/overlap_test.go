// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package overlap

import (
	"reflect"
	"testing"
)

func TestParsePatch(t *testing.T) {
	patch := "@@ -10,3 +10,4 @@ func Total() {\n-a\n+b\n@@ -40 +41 @@\n-x\n+y\n@@ -60,0 +62,2 @@\n+new\nnot a header @@ -1,1 @@"
	want := []Range{{10, 12}, {40, 40}, {60, 60}}
	if got := ParsePatch(patch); !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePatch = %v, want %v", got, want)
	}
	for _, bad := range []string{"@@ -x,1 +1 @@", "@@ -1,y +1 @@", "@@ -1,1", "@@ --1 +1 @@"} {
		if got := ParsePatch(bad); len(got) != 0 {
			t.Errorf("ParsePatch(%q) = %v, want nothing", bad, got)
		}
	}
}

const branchDiff = `diff --git a/shop.go b/shop.go
index 1..2 100644
--- a/shop.go
+++ b/shop.go
@@ -12,2 +12,3 @@
-old
+new
@@ -30,0 +31 @@
+added
diff --git a/vat.go b/vat.go
new file mode 100644
--- /dev/null
+++ b/vat.go
@@ -0,0 +1,3 @@
+package shop
diff --git a/legacy.go b/legacy.go
deleted file mode 100644
--- a/legacy.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package shop
diff --git a/util.go b/helpers.go
similarity index 100%
rename from util.go
rename to helpers.go
`

func TestParseDiff(t *testing.T) {
	want := []File{
		{Path: "shop.go", Ranges: []Range{{12, 13}, {30, 30}}},
		{Path: "legacy.go", Ranges: []Range{{1, 2}}, Whole: true},
		{Path: "util.go", Whole: true},
	}
	if got := ParseDiff(branchDiff); !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseDiff =\n%+v\nwant\n%+v", got, want)
	}
}

func TestOverlaps(t *testing.T) {
	mine := ParseDiff(branchDiff)
	for _, tt := range []struct {
		name   string
		theirs []File
		want   []string
	}{
		{"same lines", []File{{Path: "shop.go", Ranges: []Range{{13, 15}}}}, []string{"shop.go"}},
		{"adjacent lines", []File{{Path: "shop.go", Ranges: []Range{{14, 20}}}}, []string{"shop.go"}},
		{"one line apart", []File{{Path: "shop.go", Ranges: []Range{{15, 20}}}}, nil},
		{"an insertion next to mine", []File{{Path: "shop.go", Ranges: []Range{{31, 31}}}}, []string{"shop.go"}},
		{"a file I delete", []File{{Path: "legacy.go", Ranges: []Range{{50, 50}}}}, []string{"legacy.go"}},
		{"a file they delete", []File{{Path: "shop.go", Whole: true}}, []string{"shop.go"}},
		{"a file I rename", []File{{Path: "util.go", Ranges: []Range{{3, 3}}}}, []string{"util.go"}},
		{"another file", []File{{Path: "cart.go", Ranges: []Range{{12, 13}}}}, nil},
		{"a file I add", []File{{Path: "vat.go", Ranges: []Range{{1, 1}}}}, nil},
	} {
		if got := Overlaps(mine, tt.theirs); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Overlaps = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func FuzzParsePatch(f *testing.F) {
	f.Add("@@ -10,3 +10,4 @@\n-a\n+b")
	f.Add("@@ -0,0 +1 @@")
	f.Fuzz(func(t *testing.T, patch string) {
		for _, r := range ParsePatch(patch) {
			if r.Start < 0 || r.End < r.Start {
				t.Fatalf("ParsePatch(%q) gave %v", patch, r)
			}
		}
	})
}
