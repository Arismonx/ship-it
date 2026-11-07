// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// branchDiff is what `git diff -U0 --find-renames base...HEAD` prints for a
// branch that adds a file, edits one (a function added, one removed, one
// whose signature changed), renames one and removes one.
const branchDiff = `diff --git a/vat.go b/vat.go
new file mode 100644
index 0000000..1111111
--- /dev/null
+++ b/vat.go
@@ -0,0 +1,4 @@
+package shop
+
+// VAT is the French rate.
+const VAT = 20
diff --git a/shop.go b/shop.go
index 2222222..3333333 100644
--- a/shop.go
+++ b/shop.go
@@ -3,0 +4,2 @@
+func WithVAT(p Price) Price { return p + p*VAT/100 }
+type Cart struct{}
@@ -8,2 +10 @@
-func OldRate() int { return 19 }
-func Total(items []Price) Price {
+func Total(items ...Price) Price {
diff --git a/util.go b/helpers.go
similarity index 100%
rename from util.go
rename to helpers.go
diff --git a/legacy.go b/legacy.go
deleted file mode 100644
index 4444444..0000000
--- a/legacy.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package shop
-func Legacy() {}
`

func TestChangesOfABranch(t *testing.T) {
	got := ChangesOf(snapshot.ParseUnifiedDiff(branchDiff))
	want := Changes{
		Added: []string{"vat.go"}, Changed: []string{"shop.go"}, Renamed: []string{"helpers.go"}, Removed: []string{"legacy.go"},
		// Total had its signature changed: neither added nor removed.
		Declared: []string{"Cart", "WithVAT"}, Undeclared: []string{"Legacy", "OldRate"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangesOf =\n%+v\nwant\n%+v", got, want)
	}
	line := got.Line()
	wantLine := "**Changes:** 4 files: 1 added (`vat.go`), 1 changed, 1 renamed (`helpers.go`), 1 removed (`legacy.go`). " +
		"Added `Cart`, `WithVAT`; removed `Legacy`, `OldRate`."
	if line != wantLine {
		t.Fatalf("Line =\n%s\nwant\n%s", line, wantLine)
	}
}

func TestChangesLine(t *testing.T) {
	if got := (Changes{}).Line(); got != "" {
		t.Errorf("no change: %q, want nothing", got)
	}
	if got := (Changes{Changed: []string{"a.go"}}).Line(); got != "**Changes:** 1 file: 1 changed." {
		t.Errorf("one file: %q", got)
	}
	// Past the limits it counts, so a large pull request keeps one line.
	many := Changes{
		Added:    []string{"a", "b", "c", "d"},
		Declared: []string{"A", "B", "C", "D", "E", "F", "G", "H"},
	}
	got := many.Line()
	if !strings.HasPrefix(got, "**Changes:** 4 files: 4 added.") || !strings.HasSuffix(got, "`F` and 2 more.") {
		t.Errorf("many: %q", got)
	}
	if got := (Changes{Removed: []string{"x.go"}, Undeclared: []string{"X"}}).Line(); !strings.HasSuffix(got, ". Removed `X`.") {
		t.Errorf("only removed declarations start with a capital: %q", got)
	}
}

func TestChangesCloseTheSummary(t *testing.T) {
	in := BodyInput{
		Commits: []Commit{{Body: "Prices shown to customers include VAT."}},
		Changes: Changes{Added: []string{"vat.go"}, Declared: []string{"WithVAT"}},
	}
	want := "Prices shown to customers include VAT.\n\n**Changes:** 1 file: 1 added (`vat.go`). Added `WithVAT`."
	if got := Compose(in).Summary; got != want {
		t.Fatalf("Summary =\n%s\nwant\n%s", got, want)
	}
	in.Changes = Changes{}
	if got := Compose(in).Summary; got != "Prices shown to customers include VAT." {
		t.Fatalf("without changes: %q", got)
	}
}
