// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gates

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/fault"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name    string
		project Project
		want    Detection
	}{
		{"not a Node project", Project{}, Detection{}},
		{
			"npm with vitest", Project{PackageJSON: []byte(`{"scripts": {"lint": "eslint .", "test": "vitest"}}`), Base: "origin/main"},
			Detection{Manager: NPM, Missing: []string{"typecheck"}, Gates: []Gate{
				{Name: "lint", Command: []string{"npm", "run", "lint"}},
				{Name: "test", Command: []string{"npm", "run", "test", "--", "--changed", "origin/main", "--passWithNoTests"}},
			}},
		},
		{
			"pnpm from the lockfile", Project{
				PackageJSON: []byte(`{"scripts": {"lint": "eslint .", "typecheck": "vue-tsc --noEmit", "test": "vitest run"}}`),
				Present:     map[string]bool{"pnpm-lock.yaml": true}, Base: "origin/develop",
			},
			Detection{Manager: PNPM, Gates: []Gate{
				{Name: "lint", Command: []string{"pnpm", "run", "lint"}},
				{Name: "typecheck", Command: []string{"pnpm", "run", "typecheck"}},
				{Name: "test", Command: []string{"pnpm", "run", "test", "--changed", "origin/develop", "--passWithNoTests"}},
			}},
		},
		{
			"packageManager wins over the lockfile, jest runs all tests", Project{
				PackageJSON: []byte(`{"packageManager": "yarn@4.5.0+sha512.abc", "scripts": {"test": "jest"}}`),
				Present:     map[string]bool{"package-lock.json": true}, Base: "origin/main",
			},
			Detection{Manager: Yarn, Missing: []string{"lint", "typecheck"}, Gates: []Gate{{Name: "test", Command: []string{"yarn", "run", "test"}}}},
		},
		{
			"unknown base: all tests", Project{PackageJSON: []byte(`{"scripts": {"test": "vitest"}}`), Present: map[string]bool{"bun.lockb": true}},
			Detection{Manager: Bun, Missing: []string{"lint", "typecheck"}, Gates: []Gate{{Name: "test", Command: []string{"bun", "run", "test"}}}},
		},
		{"no scripts", Project{PackageJSON: []byte(`{"name": "x"}`)}, Detection{Manager: NPM, Missing: []string{"lint", "typecheck", "test"}}},
	}
	for _, tt := range tests {
		got, err := Detect(tt.project)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Detect = %+v, %v\nwant %+v", tt.name, got, err, tt.want)
		}
	}
}

func TestDetectBrokenPackageJSON(t *testing.T) {
	_, err := Detect(Project{PackageJSON: []byte(`{"scripts": `)})
	if !errors.Is(err, &fault.Fault{Code: fault.ConfigInvalidJSON}) {
		t.Fatalf("error = %v", err)
	}
}
