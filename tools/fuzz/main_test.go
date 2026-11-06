// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/tools/internal/fuzztargets"
)

func TestFailureReport(t *testing.T) {
	target := fuzztargets.Target{Package: "github.com/x/convention", Name: "FuzzTrailers"}
	title, body := failureReport(target, "internal/core/convention", []string{"0a1b"}, "line\n```\n--- FAIL: FuzzTrailers\n")
	if title != "fuzz: FuzzTrailers fails in internal/core/convention" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{
		"`internal/core/convention/testdata/fuzz/FuzzTrailers/0a1b`",
		"go test -run='^FuzzTrailers$/^0a1b$' ./internal/core/convention",
		"fuzz-FuzzTrailers",
		"--- FAIL: FuzzTrailers",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
	if strings.Count(body, "```") != 4 { // two fences for the command, two for the output
		t.Errorf("a fence of the output leaked into the body:\n%s", body)
	}
	_, none := failureReport(target, "x", nil, "panic")
	if !strings.Contains(none, "No failing input") {
		t.Errorf("without crasher:\n%s", none)
	}
}

func TestFilesInAndCopyCorpus(t *testing.T) {
	dir := t.TempDir()
	if files, err := filesIn(filepath.Join(dir, "missing")); err != nil || files != nil {
		t.Fatalf("missing dir: %q, %v", files, err)
	}
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if files, err := filesIn(dir); err != nil || !reflect.DeepEqual(files, []string{"a", "b"}) {
		t.Fatalf("filesIn = %q, %v", files, err)
	}
	to := filepath.Join(t.TempDir(), "corpus")
	if err := copyCorpus(dir, to); err != nil {
		t.Fatal(err)
	}
	if files, _ := filesIn(to); len(files) != 2 {
		t.Fatalf("copied %q", files)
	}
	if err := copyCorpus(filepath.Join(dir, "missing"), to); err != nil {
		t.Fatalf("missing corpus: %v", err)
	}
}

func TestTail(t *testing.T) {
	if got := tail("1\n2\n3\n", 2); got != "2\n3" {
		t.Errorf("tail = %q", got)
	}
}
