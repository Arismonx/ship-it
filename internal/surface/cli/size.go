// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// pathList is a repeatable flag: --only a/ --only b.go.
type pathList []string

func (p *pathList) String() string { return strings.Join(*p, ",") }

func (p *pathList) Set(v string) error {
	*p = append(*p, v)
	return nil
}

// maxSplitGroups bounds the suggestions shown; the JSON has them all.
const maxSplitGroups = 5

// printSize warns that the work is large and shows the commands that ship
// each part on its own. It never blocks.
func (r runner) printSize(s *app.SizeView) {
	if s == nil {
		return
	}
	out := r.streams.Out
	fmt.Fprintln(out, "  "+i18n.Render(r.lang, "cli.plan.too_large", map[string]string{
		"lines": strconv.Itoa(s.Lines), "max": strconv.Itoa(s.Max),
	}))
	for i, g := range s.Groups {
		if i == maxSplitGroups {
			break
		}
		fmt.Fprintf(out, "      ship --only %-28s %s\n", g.Path, i18n.Render(r.lang, "cli.plan.group_lines", map[string]string{
			"lines": strconv.Itoa(g.Lines), "files": strconv.Itoa(len(g.Files)),
		}))
	}
}
