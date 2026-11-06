// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package fuzztargets finds the fuzz targets of the module, for the short
// fuzzing of tools/check and the weekly long fuzzing of fuzz.yml
// (docs/design/workflows.md §8).
package fuzztargets

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
)

// Target is one Fuzz function of a package.
type Target struct {
	Package string `json:"package"` // import path
	Name    string `json:"name"`    // FuzzXxx
}

// List runs `go test -list ^Fuzz ./...` in dir and returns every target.
func List(dir string) ([]Target, error) {
	cmd := exec.Command("go", "test", "-list", "^Fuzz", "./...")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return Parse(out), nil
}

// Parse reads the output of `go test -list`: the names of a package come
// before its "ok <package> <duration>" line.
func Parse(out []byte) []Target {
	var targets []Target
	var pending []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Fuzz"):
			pending = append(pending, strings.TrimSpace(line))
		case strings.HasPrefix(line, "ok"):
			fields := strings.Fields(line)
			if len(fields) < 2 {
				pending = nil
				continue
			}
			for _, name := range pending {
				targets = append(targets, Target{Package: fields[1], Name: name})
			}
			pending = nil
		}
	}
	return targets
}
