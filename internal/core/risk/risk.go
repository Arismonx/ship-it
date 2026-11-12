// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package risk scores how carefully a pull request needs reading (#45),
// from facts only: its size, code changed without tests, sensitive paths,
// removals. Every point comes with the factor that gave it, so the score is
// explained, never a guess.
//
// Allowed imports: standard library without I/O, other core packages.
package risk

import (
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/glob"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
)

// Levels of an assessment.
const (
	Low    = "low"
	Medium = "medium"
	High   = "high"
)

// Thresholds: changed lines for one and two points, points for medium and
// high. The line thresholds match the size/M and size/L labels.
const (
	someLines   = 100
	manyLines   = 500
	mediumFrom  = 2
	highFrom    = 4
	listedPaths = 3
)

// DefaultSensitivePaths are paths whose changes deserve a closer look, on top
// of continuous integration and dependencies (ship.pullRequest.sensitivePaths).
var DefaultSensitivePaths = []string{"**/migrations/**", "**/auth/**", "**/security/**", "**/*.sql", "**/Dockerfile"}

// File is one file of the pull request.
type File struct {
	Path    string
	Removed bool
}

// Input is what the assessment reads.
type Input struct {
	// ChangedLines excludes generated files and lockfiles, as size/ does.
	ChangedLines int
	Files        []File
	// RemovedDeclarations counts the functions, types… the diff removes.
	RemovedDeclarations int
	// SensitivePaths are glob patterns (internal/core/glob).
	SensitivePaths []string
}

// Factor is one reason for points, with the message key that explains it.
type Factor struct {
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
	Points int               `json:"points"`
}

// Assessment is the level, its points and the factors behind them.
type Assessment struct {
	Level   string   `json:"level"`
	Points  int      `json:"points"`
	Factors []Factor `json:"factors,omitempty"`
}

// Assess scores a pull request.
func Assess(in Input) Assessment {
	var a Assessment
	add := func(points int, key string, params ...string) {
		f := Factor{Key: key, Points: points, Params: map[string]string{}}
		for i := 0; i+1 < len(params); i += 2 {
			f.Params[params[i]] = params[i+1]
		}
		a.Factors = append(a.Factors, f)
		a.Points += points
	}

	switch {
	case in.ChangedLines >= manyLines:
		add(2, "risk.size", "lines", strconv.Itoa(in.ChangedLines))
	case in.ChangedLines >= someLines:
		add(1, "risk.size", "lines", strconv.Itoa(in.ChangedLines))
	}

	source, tests, removed := 0, 0, 0
	var sensitive []string
	for _, f := range in.Files {
		category := snapshot.Categorize(f.Path)
		switch {
		case f.Removed:
			removed++
		case category == snapshot.SourceCode:
			source++
		case category == snapshot.Tests:
			tests++
		}
		if category == snapshot.ContinuousIntegration || category == snapshot.Dependencies || glob.Any(in.SensitivePaths, f.Path) {
			sensitive = append(sensitive, f.Path)
		}
	}
	if source > 0 && tests == 0 {
		add(2, "risk.untested", "count", strconv.Itoa(source))
	}
	if len(sensitive) > 0 {
		add(2, "risk.sensitive", "files", listed(sensitive))
	}
	if removed+in.RemovedDeclarations > 0 {
		add(1, "risk.removals", "files", strconv.Itoa(removed), "declarations", strconv.Itoa(in.RemovedDeclarations))
	}

	switch {
	case a.Points >= highFrom:
		a.Level = High
	case a.Points >= mediumFrom:
		a.Level = Medium
	default:
		a.Level = Low
	}
	return a
}

// listed names the first paths, then how many more.
func listed(paths []string) string {
	if len(paths) <= listedPaths {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:listedPaths], ", ") + " (+" + strconv.Itoa(len(paths)-listedPaths) + ")"
}
