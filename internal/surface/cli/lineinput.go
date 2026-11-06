// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// lineInput is a one-line text field of the review screen. It replaces
// bubbles/textinput, which imports github.com/atotto/clipboard for Ctrl+V
// (ADR-0012: only what we use); terminals paste with bracketed paste.
type lineInput struct {
	value       []rune
	pos         int // cursor, in runes
	focused     bool
	placeholder string
	width       int // cells shown; the text scrolls to keep the cursor visible
}

var styleCursor = lipgloss.NewStyle().Reverse(true)

func (in *lineInput) SetValue(s string) {
	in.value = []rune(s)
	in.pos = len(in.value)
}

func (in lineInput) Value() string { return string(in.value) }

// update edits the field; it reports whether msg was a key it handled.
func (in *lineInput) update(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		in.insert(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(msg.Content))
		return true
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "ctrl+b":
			in.pos = max(in.pos-1, 0)
		case "right", "ctrl+f":
			in.pos = min(in.pos+1, len(in.value))
		case "home", "ctrl+a":
			in.pos = 0
		case "end", "ctrl+e":
			in.pos = len(in.value)
		case "backspace", "ctrl+h":
			if in.pos > 0 {
				in.value = append(in.value[:in.pos-1], in.value[in.pos:]...)
				in.pos--
			}
		case "delete", "ctrl+d":
			if in.pos < len(in.value) {
				in.value = append(in.value[:in.pos], in.value[in.pos+1:]...)
			}
		case "ctrl+u":
			in.value, in.pos = in.value[in.pos:], 0
		case "ctrl+k":
			in.value = in.value[:in.pos]
		case "ctrl+w", "alt+backspace":
			start := in.pos
			for start > 0 && unicode.IsSpace(in.value[start-1]) {
				start--
			}
			for start > 0 && !unicode.IsSpace(in.value[start-1]) {
				start--
			}
			in.value = append(in.value[:start], in.value[in.pos:]...)
			in.pos = start
		default:
			if msg.Text == "" {
				return false
			}
			in.insert(msg.Text)
		}
		return true
	}
	return false
}

func (in *lineInput) insert(s string) {
	var r []rune
	for _, c := range s {
		if unicode.IsPrint(c) || c == ' ' {
			r = append(r, c)
		}
	}
	tail := append(r, in.value[in.pos:]...)
	in.value = append(in.value[:in.pos], tail...)
	in.pos += len(r)
}

// view shows the field; when focused, the cursor is the reversed cell, so it
// stays visible without colour.
func (in lineInput) view() string {
	if len(in.value) == 0 {
		if !in.focused {
			return styleFaint.Render(in.placeholder)
		}
		if in.placeholder == "" {
			return styleCursor.Render(" ")
		}
		p := []rune(in.placeholder)
		return styleCursor.Render(string(p[0])) + styleFaint.Render(string(p[1:]))
	}
	// The window [start, end) of runes that fits width and holds the cursor.
	width := max(in.width, 1)
	start := 0
	if in.focused && lipgloss.Width(string(in.value[:in.pos]))+1 > width {
		start = in.pos
		for start > 0 && lipgloss.Width(string(in.value[start-1:in.pos]))+1 <= width {
			start--
		}
	}
	end := start
	for end < len(in.value) && lipgloss.Width(string(in.value[start:end+1])) <= width {
		end++
	}
	if !in.focused {
		text := string(in.value[start:end])
		if end < len(in.value) {
			text = fit(string(in.value), width)
		}
		return text
	}
	var b strings.Builder
	b.WriteString(string(in.value[start:min(in.pos, end)]))
	if in.pos < len(in.value) {
		b.WriteString(styleCursor.Render(string(in.value[in.pos])))
		if in.pos+1 < end {
			b.WriteString(string(in.value[in.pos+1 : end]))
		}
	} else {
		b.WriteString(styleCursor.Render(" "))
	}
	return b.String()
}
