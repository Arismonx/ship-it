// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The GitHub Marketplace lists one action per repository, with action.yml at
// its root (docs/design/workflows.md §6). build/ship-it-action/ is the content of
// that repository, GregoireF/ship-it-action, made from actions/check-title so
// that both stay the same action.

const (
	marketplaceDir     = "build/ship-it-action"
	checkTitleAction   = "actions/check-title/action.yml"
	omitBegin, omitEnd = "# marketplace:omit-begin", "# marketplace:omit-end"
)

// marketplaceFiles returns the files of build/ship-it-action/, read from root.
func marketplaceFiles(root string) (map[string][]byte, error) {
	source, err := os.ReadFile(filepath.Join(root, checkTitleAction))
	if err != nil {
		return nil, err
	}
	action, err := marketplaceAction(strings.ReplaceAll(string(source), "\r\n", "\n"))
	if err != nil {
		return nil, err
	}
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		filepath.Join(marketplaceDir, "action.yml"): []byte(action),
		filepath.Join(marketplaceDir, "README.md"):  []byte(marketplaceReadme),
		filepath.Join(marketplaceDir, "LICENSE"):    []byte(strings.ReplaceAll(string(license), "\r\n", "\n")),
	}, nil
}

// marketplaceAction drops the source mode (it builds ship from the action's
// own checkout, which is ship-it only inside ship-it) and rewrites what
// mentions it. Each rewrite must apply: a change of the source fails here.
func marketplaceAction(source string) (string, error) {
	var b strings.Builder
	omitting, omitted := false, false
	for _, line := range strings.SplitAfter(source, "\n") {
		switch trimmed := strings.TrimSpace(line); {
		case strings.HasPrefix(trimmed, omitBegin):
			omitting, omitted = true, true
		case strings.HasPrefix(trimmed, omitEnd):
			omitting = false
		case !omitting:
			b.WriteString(line)
		}
	}
	if !omitted || omitting {
		return "", fmt.Errorf("%s: missing %q … %q block", checkTitleAction, omitBegin, omitEnd)
	}
	action := b.String()
	for _, r := range []struct{ old, new string }{
		{"# Checks a pull request title (or any commit header) with ship-it's own\n" +
			"# parser, `ship check title` (docs/reference/conventions.md). See README.md here and\n" +
			"# docs/design/workflows.md §6. go run ./tools/gen copies it, without the source\n" +
			"# mode, to build/ship-it-action/ (the Marketplace repository GregoireF/ship-it-action).\n",
			"# Checks a pull request title (or any commit header) with ship-it's own\n" +
				"# parser, `ship check title`. Generated from actions/check-title of\n" +
				"# https://github.com/GregoireF/ship-it by `go run ./tools/gen`: change it there.\n"},
		{"      \"source\" builds ship from the source of this action's own commit\n" +
			"      (needs Go, installed with actions/setup-go).\n",
			"      (\"source\" exists only in GregoireF/ship-it/actions/check-title.)\n"},
		{"version must be X.Y.Z, vX.Y.Z or source, got", "version must be X.Y.Z or vX.Y.Z, got"},
		{"      if: inputs.version != 'source'\n", ""},
		{"      if: inputs.version != 'source' && inputs.verify-attestation == 'true'\n",
			"      if: inputs.verify-attestation == 'true'\n"},
		{"        SHIP: ${{ steps.build.outputs.path || steps.download.outputs.path }}\n",
			"        SHIP: ${{ steps.download.outputs.path }}\n"},
	} {
		if !strings.Contains(action, r.old) {
			return "", fmt.Errorf("%s: %q not found, update tools/gen/marketplace.go", checkTitleAction, r.old)
		}
		action = strings.Replace(action, r.old, r.new, 1)
	}
	return action, nil
}

const marketplaceReadme = `# ship-it check title

Check pull request titles against [ship-it](https://github.com/GregoireF/ship-it)'s commit conventions: gitmoji first, a Conventional Commits type, a subject of at most 50 characters, a header of at most 72. With squash merges, the title becomes the commit header.

` + "```yaml" + `
name: pr-title
on:
  pull_request:
    types: [opened, edited, synchronize, reopened]
permissions: {}
jobs:
  title:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: GregoireF/ship-it-action@<full commit SHA> # vX.Y.Z
        with:
          title: ${{ github.event.pull_request.title }}
          version: X.Y.Z
` + "```" + `

Pin the action by full commit SHA. The major tag (` + "`v1`" + `) follows the latest compatible release; every ` + "`vX.Y.Z`" + ` release is immutable.

## Inputs

| Input | Required | Meaning |
|---|---|---|
| ` + "`title`" + ` | yes | The title to check. It reaches ` + "`ship`" + ` through an environment variable, never through the script text, so a hostile title cannot inject commands. |
| ` + "`version`" + ` | yes | A ship release (` + "`1.2.0`" + ` or ` + "`v1.2.0`" + `): the archive for the runner's OS and architecture is downloaded from the GitHub release of GregoireF/ship-it and its SHA-256 checked against the release's ` + "`checksums.txt`" + `. |
| ` + "`verify-attestation`" + ` | no | ` + "`\"true\"`" + ` also runs ` + "`gh attestation verify`" + ` on the archive (built by ship-it's release workflow), with the job's token. Default ` + "`\"false\"`" + `. |

Linux, macOS and Windows runners, x64 and ARM64. No permission is needed.

## Source and license

This repository is generated from [` + "`actions/check-title`" + `](https://github.com/GregoireF/ship-it/tree/main/actions/check-title) of GregoireF/ship-it: issues and changes go there. Licensed under the Apache License 2.0 (see LICENSE).
`
