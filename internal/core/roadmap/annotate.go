// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package roadmap

import (
	"strconv"
	"strings"
)

// Edit changes one item line of ROADMAP.md: the issue number to append and
// whether to tick the box (an issue closed on GitHub).
type Edit struct {
	Line    int
	Number  int  // 0: keep the line's number
	Check   bool // tick the box
	Uncheck bool // untick it (an issue reopened on GitHub)
}

// Annotate applies edits to markdown line by line, keeping everything else
// byte for byte (line endings included): only the item lines named change.
// A line that is not an item, or that already holds a number, keeps it.
func Annotate(markdown []byte, edits []Edit) []byte {
	text := string(markdown)
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(text, eol)
	for _, e := range edits {
		i := e.Line - 1
		if i < 0 || i >= len(lines) {
			continue
		}
		m := itemLine.FindStringSubmatchIndex(lines[i])
		if m == nil {
			continue
		}
		line := lines[i]
		switch box := line[m[4]:m[5]]; {
		case e.Check && box == " ":
			line = line[:m[4]] + "x" + line[m[5]:]
		case e.Uncheck && box != " ":
			line = line[:m[4]] + " " + line[m[5]:]
		}
		if e.Number > 0 && !numberTail.MatchString(line) {
			line = strings.TrimRight(line, " \t") + " #" + strconv.Itoa(e.Number)
		}
		lines[i] = line
	}
	return []byte(strings.Join(lines, eol))
}
