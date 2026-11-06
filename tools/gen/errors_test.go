// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"path/filepath"
	"testing"

	"github.com/GregoireF/ship-it/internal/i18n"
)

// TestEveryFaultCodeHasAMessage keeps docs/reference/errors.md useful: a
// code that the source never pairs with a catalogue key would be listed with
// nothing to read.
func TestEveryFaultCodeHasAMessage(t *testing.T) {
	root := filepath.Join("..", "..")
	codes, err := faultCodes(root)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := faultKeys(root, i18n.Keys())
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) < 50 {
		t.Fatalf("read only %d codes from fault.go", len(codes))
	}
	for _, code := range codes {
		if len(keys[code.Name]) == 0 {
			t.Errorf("%s (%s) is never used with a catalogue key", code.Value, code.Name)
		}
	}
}

func TestGroupTitle(t *testing.T) {
	for doc, want := range map[string]string{
		"Codes of the organisation policy (08xx range): they exit with code 5.": "Organisation policy",
		"Codes of delivery (05xx range).":                                       "Delivery",
		"Unexpected wraps an error that has no dedicated code yet.":             "General",
	} {
		if got := groupTitle(doc); got != want {
			t.Errorf("groupTitle(%q) = %q, want %q", doc, got, want)
		}
	}
}
