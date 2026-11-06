// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package settings computes the changes that bring a GitHub repository to the
// state .github/ship.json describes (ADR-0014): `ship sync` shows them, then
// applies them after confirmation. Nothing is ever deleted.
//
// Allowed imports: standard library without I/O, other core packages.
package settings

import (
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
)

// Label is a repository label. Colors are six hexadecimal digits, without "#".
type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	// Replaces lists GitHub default labels renamed into this one, so that the
	// issues already labelled keep their label (docs/design/git-github-standards.md §5 bis).
	Replaces []string `json:"replaces,omitempty"`
}

// Colors carry meaning, as in kubernetes/kubernetes (its labels, read from
// the GitHub API on 2026-10-23): red for what blocks or breaks,
// orange to yellow for urgency, a green-to-red ramp for size, neutral blue
// for kinds of work and areas. A label Kubernetes does not have takes the
// color of its closest Kubernetes counterpart.
const (
	alarmColor     = "e11d21" // kind/bug, kind/regression, kind/api-change, priority/critical-urgent
	workColor      = "c7def8" // kind/feature, kind/cleanup, kind/documentation
	discussColor   = "d455d0" // kind/support, triage/needs-information, triage/duplicate
	soonColor      = "eb6420" // priority/important-soon, priority/important-longterm
	backlogColor   = "fbca04" // priority/backlog
	untriagedColor = "ededed" // needs-triage
	goColor        = "8fc951" // triage/accepted, lifecycle/active
	staleColor     = "795548" // lifecycle/stale
	frozenColor    = "d3e2f0" // lifecycle/frozen
	areaColor      = "0052cc" // area/*
)

// sizeColors ramp from green to red, size/XS to size/XXL (Kubernetes).
var sizeColors = [...]string{"009900", "77bb00", "eebb00", "ee9900", "ee5500", "ee0000"}

// ShipItTaxonomy returns the labels of docs/design/git-github-standards.md §5 bis, plus area/<name> for
// each area.
func ShipItTaxonomy(areas []string) []Label {
	labels := []Label{
		{"kind/bug", alarmColor, "Something does not work as intended", []string{"bug"}},
		{"kind/feature", workColor, "New feature or request", []string{"enhancement"}},
		{"kind/docs", workColor, "Documentation only", []string{"documentation"}},
		{"kind/refactor", workColor, "Code change that neither fixes a bug nor adds a feature", nil},
		{"kind/perf", workColor, "Performance improvement", nil},
		{"kind/security", alarmColor, "Security fix or hardening", nil},
		{"kind/test", workColor, "Tests only", nil},
		{"kind/ci", workColor, "Continuous integration", nil},
		{"kind/build", workColor, "Build system or dependencies", nil},
		{"kind/chore", workColor, "Maintenance", nil},
		{"kind/regression", alarmColor, "Something that used to work is broken", nil},
		{"kind/breaking", alarmColor, "Breaking change for users", nil},
		{"kind/support", discussColor, "Question or support request", []string{"question"}},
		{"kind/accessibility", workColor, "Accessibility", []string{"accessibility"}},
		{"priority/p0", alarmColor, "Must be worked on now; needs an assignee", nil},
		{"priority/p1", soonColor, "Next release", nil},
		{"priority/p2", soonColor, "Long term", nil},
		{"priority/p3", backlogColor, "Backlog", nil},
		{"triage/needs-triage", untriagedColor, "Not reviewed by a maintainer yet", nil},
		{"triage/accepted", goColor, "Reviewed and accepted", nil},
		{"triage/needs-info", discussColor, "Waiting for more information", nil},
		{"triage/duplicate", discussColor, "Already reported elsewhere", []string{"duplicate"}},
		{"triage/not-reproducible", discussColor, "Could not be reproduced", nil},
		{"triage/invalid", discussColor, "Not a valid report", []string{"invalid"}},
		{"triage/wontfix", discussColor, "Will not be worked on", []string{"wontfix"}},
		{"lifecycle/active", goColor, "Being worked on", nil},
		{"lifecycle/stale", staleColor, "No activity for a long time", nil},
		{"lifecycle/frozen", frozenColor, "Kept open on purpose despite inactivity", nil},
		{"size/XS", sizeColors[0], "0-9 changed lines, generated files excluded", nil},
		{"size/S", sizeColors[1], "10-29 changed lines, generated files excluded", nil},
		{"size/M", sizeColors[2], "30-99 changed lines, generated files excluded", nil},
		{"size/L", sizeColors[3], "100-499 changed lines, generated files excluded", nil},
		{"size/XL", sizeColors[4], "500-999 changed lines, generated files excluded", nil},
		{"size/XXL", sizeColors[5], "1000 or more changed lines, generated files excluded", nil},
	}
	for _, area := range areas {
		if area = strings.TrimSpace(area); area != "" {
			labels = append(labels, Label{Name: "area/" + area, Color: areaColor, Description: "Area: " + area})
		}
	}
	return labels
}

// WantedLabels returns the labels the settings ask for.
func WantedLabels(s config.Settings) []Label {
	if s.GitHub.Labels.Taxonomy != config.TaxonomyShipIt {
		return nil
	}
	labels := ShipItTaxonomy(s.GitHub.Labels.Areas)
	pr := s.Ship.PullRequest
	kindPrefix, sizePrefix := pr.LabelPrefixes.Kind, pr.LabelPrefixes.Size
	wanted := make([]Label, 0, len(labels))
	for _, l := range labels {
		switch {
		case strings.HasPrefix(l.Name, "size/"):
			continue // from ship.pullRequest.sizes, below
		case strings.HasPrefix(l.Name, "kind/"):
			l.Name = kindPrefix + strings.TrimPrefix(l.Name, "kind/")
		}
		wanted = append(wanted, l)
	}
	return append(wanted, sizeLabels(sizePrefix, pr.Sizes.List())...)
}

// sizeLabels describes the size labels of ship.pullRequest.sizes, smallest
// first, on the green-to-red ramp.
func sizeLabels(prefix string, sizes []pullrequest.Size) []Label {
	labels := make([]Label, 0, len(sizes))
	from := 0
	for i, s := range sizes {
		color := sizeColors[len(sizeColors)-1]
		if len(sizes) > 1 {
			color = sizeColors[i*(len(sizeColors)-1)/(len(sizes)-1)]
		}
		description := strconv.Itoa(from) + " or more changed lines, generated files excluded"
		if s.Below > 0 {
			description = strconv.Itoa(from) + "-" + strconv.Itoa(s.Below-1) + " changed lines, generated files excluded"
			from = s.Below
		}
		labels = append(labels, Label{Name: prefix + s.Name, Color: color, Description: description})
	}
	return labels
}

// ChangeKind is what a label change does.
type ChangeKind string

// Label changes. There is no deletion: labels ship-it does not know about are
// the user's.
const (
	CreateLabel ChangeKind = "create"
	// RenameLabel renames From into the wanted label, keeping its issues.
	RenameLabel ChangeKind = "rename"
	// UpdateLabel fixes the color or description of an existing label.
	UpdateLabel ChangeKind = "update"
)

// LabelChange is one step of the plan.
type LabelChange struct {
	Kind  ChangeKind `json:"kind"`
	Label Label      `json:"label"`
	// From is the current name, for renames.
	From string `json:"from,omitempty"`
}

// PlanLabels returns the changes turning existing into wanted. Names compare
// case-insensitively, as GitHub does.
func PlanLabels(wanted, existing []Label) []LabelChange {
	byName := map[string]Label{}
	for _, l := range existing {
		byName[strings.ToLower(l.Name)] = l
	}
	renamed := map[string]bool{}
	var changes []LabelChange
	for _, w := range wanted {
		if current, ok := byName[strings.ToLower(w.Name)]; ok {
			if !strings.EqualFold(current.Color, w.Color) || current.Description != w.Description {
				changes = append(changes, LabelChange{Kind: UpdateLabel, Label: w, From: current.Name})
			}
			continue
		}
		change := LabelChange{Kind: CreateLabel, Label: w}
		for _, old := range w.Replaces {
			if current, ok := byName[strings.ToLower(old)]; ok && !renamed[strings.ToLower(old)] {
				change = LabelChange{Kind: RenameLabel, Label: w, From: current.Name}
				renamed[strings.ToLower(old)] = true
				break
			}
		}
		changes = append(changes, change)
	}
	slices.SortStableFunc(changes, func(a, b LabelChange) int { return order[a.Kind] - order[b.Kind] })
	return changes
}

// order shows renames first: they are the changes users notice most.
var order = map[ChangeKind]int{RenameLabel: 0, CreateLabel: 1, UpdateLabel: 2}
