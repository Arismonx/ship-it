// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmap

import (
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/convention"
)

const sample = "# Roadmap\n\n> Intro, not a milestone.\n\n" +
	"## Phase 1 — Foundations\n" +
	"**Due:** 2026-12-15\n" +
	"- [ ] feat: CLI skeleton #12\n" +
	"  Binaries for three systems.\n" +
	"  - [ ] git exec layer\n" +
	"    with tests on real repositories\n" +
	"  - [x] status parsing #14\n" +
	"- [x] ADRs\n" +
	"```\n- [ ] not an item, inside a fence\n```\n" +
	"**Exit:** a junior ships a PR.\n\n" +
	"## Notes\nNo checklist here.\n"

func TestParse(t *testing.T) {
	r := Parse([]byte(sample))
	if len(r.Phases) != 1 {
		t.Fatalf("phases = %+v", r.Phases)
	}
	p := r.Phases[0]
	if p.Title != "Phase 1 — Foundations" || p.Due != "2026-12-15" || p.Exit != "a junior ships a PR." || len(p.Items) != 2 {
		t.Fatalf("phase = %+v", p)
	}
	cli := p.Items[0]
	if cli.Kind != "feat" || cli.Title() != "CLI skeleton" || cli.Number != 12 || cli.Done || cli.Line != 7 {
		t.Errorf("item = %+v (title %q)", cli, cli.Title())
	}
	if len(cli.Description) != 1 || cli.Description[0] != "Binaries for three systems." {
		t.Errorf("description = %q", cli.Description)
	}
	if len(cli.Children) != 2 || cli.Children[0].Text != "git exec layer" || cli.Children[0].Number != 0 ||
		cli.Children[0].Description[0] != "with tests on real repositories" || !cli.Children[1].Done || cli.Children[1].Number != 14 {
		t.Errorf("children = %+v", cli.Children)
	}
	if adr := p.Items[1]; !adr.Done || adr.Kind != "" || adr.Title() != "ADRs" {
		t.Errorf("item = %+v", adr)
	}
}

func TestUnknownPrefixIsPartOfTheTitle(t *testing.T) {
	r := Parse([]byte("## P\n- [ ] `obs`: JSONL journal\n- [ ] note: not a type\n"))
	if got := r.Phases[0].Items[0]; got.Kind != "" || got.Title() != "`obs`: JSONL journal" {
		t.Errorf("item = %+v", got)
	}
	if got := r.Phases[0].Items[1]; got.Kind != "" || got.Title() != "note: not a type" {
		t.Errorf("item = %+v", got)
	}
}

// Kinds follow the repository's commit types: an added type is a kind, a
// removed one is part of the title.
func TestKindsOfTheRepositoryTypes(t *testing.T) {
	deps := "⬆️"
	prefix := "chore"
	rules := convention.Rules{Types: convention.WithTypes([]convention.TypeChange{
		{Type: "deps", Gitmoji: &deps, BranchPrefix: &prefix}, {Type: "docs", Remove: true},
	})}
	r := ParseWith([]byte("## P\n- [ ] deps: bump yaml\n- [ ] docs: guide\n"), rules)
	if got := r.Phases[0].Items[0]; got.Kind != "deps" || got.Title() != "bump yaml" {
		t.Errorf("added type: %+v", got)
	}
	if got := r.Phases[0].Items[1]; got.Kind != "" || got.Title() != "docs: guide" {
		t.Errorf("removed type: %+v", got)
	}
}

func TestLongTitlesAreShortenedAtAWord(t *testing.T) {
	it := Item{Text: strings.Repeat("word ", 80)}
	title := it.Title()
	if len([]rune(title)) > MaxTitleLength || !strings.HasSuffix(title, "word…") {
		t.Fatalf("title (%d) = %q", len([]rune(title)), title)
	}
}

func TestAnnotateTouchesOnlyTheNamedLines(t *testing.T) {
	in := strings.ReplaceAll(sample, "\n", "\r\n")
	out := string(Annotate([]byte(in), []Edit{
		{Line: 9, Number: 13},               // "git exec layer": gets its number
		{Line: 7, Number: 99},               // already #12: kept
		{Line: 12, Check: true, Number: 15}, // "ADRs": already ticked, gets its number
		{Line: 5, Number: 1},                // a heading: ignored
	}))
	want := strings.NewReplacer(
		"  - [ ] git exec layer\r\n", "  - [ ] git exec layer #13\r\n",
		"- [x] ADRs\r\n", "- [x] ADRs #15\r\n",
	).Replace(in)
	if out != want {
		t.Fatalf("Annotate =\n%q\nwant\n%q", out, want)
	}
	if got := string(Annotate([]byte("## P\n- [ ] a #3\n"), []Edit{{Line: 2, Check: true}})); got != "## P\n- [x] a #3\n" {
		t.Errorf("check = %q", got)
	}
}

// FuzzParse: no input panics, and Annotate keeps the line count.
func FuzzParse(f *testing.F) {
	f.Add(sample)
	f.Add("## a\n\t- [ ] b\n  - [x] c #0\n")
	f.Fuzz(func(t *testing.T, md string) {
		r := Parse([]byte(md))
		var edits []Edit
		for _, p := range r.Phases {
			for _, it := range p.Items {
				edits = append(edits, Edit{Line: it.Line, Number: 7, Check: true})
			}
		}
		out := Annotate([]byte(md), edits)
		if strings.Count(string(out), "\n") != strings.Count(md, "\n") {
			t.Fatalf("line count changed: %q → %q", md, out)
		}
	})
}
