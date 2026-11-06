// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package pins is the single source of truth for the versions of the
// third-party tools that Dependabot cannot update, because they are not
// actions nor Go modules of ship-it: they are downloaded by a workflow or run
// by tools/check (docs/design/workflows.md §5).
//
// Every place that uses one of these versions outside Go code carries a
// marker comment right after the version, for example in a workflow:
//
//	version: 1.31.1 # pin: zizmor
//
// The package test checks that every marker matches this file, and
// `go run ./tools/outdated` compares this file with the latest releases.
package pins

// Source tells where the latest release of a tool is published.
type Source int

const (
	// GitHub releases: Repo is "owner/name", the "Latest" release counts.
	GitHub Source = iota
	// PyPI: Repo is the project name.
	PyPI
)

// Tool is one pinned tool.
type Tool struct {
	Name    string // marker name: "# pin: <Name>"
	Version string // exactly as written where it is used ("1.31.1", "v2.19.0")
	Source  Source
	Repo    string
}

// Tools lists every pinned tool.
var Tools = []Tool{
	// Workflow linter, Rust, installed from PyPI by uvx and by zizmor-action.
	Zizmor,
	// Workflow linter, built with `go run pkg@version`.
	Actionlint,
	// Release builder (release.yml), installed by goreleaser-action.
	GoReleaser,
	// Keyless signature of checksums.txt (release.yml), installed by
	// cosign-installer, which verifies the binary it downloads.
	Cosign,
	// Image builder (release.yml), installed by setup-buildx-action.
	Buildx,
}

var (
	Zizmor     = Tool{Name: "zizmor", Version: "1.31.1", Source: PyPI, Repo: "zizmor"}
	Actionlint = Tool{Name: "actionlint", Version: "v1.7.12", Source: GitHub, Repo: "rhysd/actionlint"}
	GoReleaser = Tool{Name: "goreleaser", Version: "v2.19.0", Source: GitHub, Repo: "goreleaser/goreleaser"}
	Cosign     = Tool{Name: "cosign", Version: "v3.1.3", Source: GitHub, Repo: "sigstore/cosign"}
	Buildx     = Tool{Name: "buildx", Version: "v0.38.0", Source: GitHub, Repo: "docker/buildx"}
)

// ActionlintPackage is what `go run` builds.
func ActionlintPackage() string {
	return "github.com/rhysd/actionlint/cmd/actionlint@" + Actionlint.Version
}

// ZizmorPackage is what `uvx` runs.
func ZizmorPackage() string {
	return "zizmor@" + Zizmor.Version
}
