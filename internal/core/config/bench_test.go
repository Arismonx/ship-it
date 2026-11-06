// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config_test

import (
	"testing"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/testkit/benchdata"
)

// The configuration stack of a consultant: a user file with contexts, the
// recommended preset, a repository file with overrides, SHIP_* variables
// and a --set flag, for a delivery of 500 files.
const (
	userFile = `{
  // ~/.config/ship-it/config.json
  "ship": {"issues": {"keyword": "Closes"}, "pullRequest": {"maxLines": 400}},
  "contexts": [
    {"match": {"owner": "acme-corp"}, "ship": {"issues": {"keyword": "Fixes"}}},
    {"match": {"path": "~/clients/gouv/**"}, "ship": {"publish": {"mode": "never"}}},
    {"match": {"host": "github.example.gouv.fr", "owner": "dinum"}, "ship": {"pullRequest": {"maxLines": 200}}},
    {"match": {"owner": "acme"}, "ship": {"gates": {"enabled": true}}}, // trailing comma below
  ]
}`
	repositoryFile = `{
  "$schema": "https://raw.githubusercontent.com/GregoireF/ship-it/main/schemas/ship.schema.json",
  "extends": ["ship-it:recommended"],
  /* Team defaults. */
  "ship": {"pullRequest": {"maxLines": 600}, "gates": {"enabled": true}},
  "overrides": [
    {"paths": ["docs/**", "*.md"], "ship": {"gates": {"enabled": false}}},
    {"paths": ["apps/web/**"], "ship": {"pullRequest": {"maxLines": 800}}},
    {"paths": ["**/*_test.go"], "ship": {"pullRequest": {"maxLines": 1000}}}
  ]
}`
)

func layers(b *testing.B) []config.Layer {
	b.Helper()
	preset, ok := config.BuiltinPresetLayer("recommended")
	if !ok {
		b.Fatal("no recommended preset")
	}
	env, err := config.EnvironmentLayers([]string{"SHIP_PULLREQUEST_MAXLINES=700", "PATH=/usr/bin", "HOME=/home/ada"})
	if err != nil {
		b.Fatal(err)
	}
	flags, err := config.FlagLayers([]string{"ship.issues.keyword=Resolves"})
	if err != nil {
		b.Fatal(err)
	}
	stack := []config.Layer{
		preset,
		{Name: "user", Kind: config.KindUser, Path: "~/.config/ship-it/config.json", Data: []byte(userFile)},
		{Name: "repository", Kind: config.KindRepository, Path: ".github/ship.json", Data: []byte(repositoryFile)},
	}
	return append(append(stack, env...), flags...)
}

// BenchmarkMergeFor resolves the whole stack for one repository and the
// paths of a delivery.
func BenchmarkMergeFor(b *testing.B) {
	stack := layers(b)
	target := config.Target{Owner: "acme", Host: "github.com", Dir: "/home/ada/src/widget", Home: "/home/ada",
		Paths: benchdata.Paths(500)}
	for b.Loop() {
		if _, err := config.MergeFor(stack, target); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStripJSONC strips comments and trailing commas from a
// configuration file.
func BenchmarkStripJSONC(b *testing.B) {
	data := []byte(userFile + "\n" + repositoryFile)
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		config.StripJSONC(data)
	}
}
