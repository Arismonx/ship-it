// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

// StripJSONC turns JSON with comments (JSONC, as tsconfig.json and the VS Code
// settings accept it) into strict JSON for encoding/json: `//` line comments,
// `/* */` block comments and trailing commas before `}` or `]` are replaced
// by spaces. Newlines are kept and every other byte stays at its offset, so a
// syntax error still points at the right line and column of the file
// (decision of 2026-10-23: teams comment their choices in .github/ship.json).
//
// Strings are copied as they are, escapes included. An unterminated block
// comment is blanked to the end; the decoder then reports the missing value.
func StripJSONC(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	blank := func(from, to int) {
		for i := from; i < to; i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	// lastComma is the offset of a comma that may turn out to be trailing;
	// prev is the last significant byte, as a comma that follows no value
	// ("{,}", "[1,,]") stays for the decoder to reject.
	lastComma := -1
	prev := byte(0)
	for i := 0; i < len(out); i++ {
		c := out[i]
		switch {
		case c == '"':
			lastComma, prev = -1, '"'
			i++
			for i < len(out) && out[i] != '"' {
				if out[i] == '\\' {
					i++
				}
				i++
			}
		case c == '/' && i+1 < len(out) && out[i+1] == '/':
			end := i
			for end < len(out) && out[end] != '\n' {
				end++
			}
			blank(i, end)
			i = end - 1
		case c == '/' && i+1 < len(out) && out[i+1] == '*':
			end := i + 2
			for end+1 < len(out) && !(out[end] == '*' && out[end+1] == '/') {
				end++
			}
			end = min(end+2, len(out))
			blank(i, end)
			i = end - 1
		case c == ',':
			lastComma = -1
			if prev != 0 && prev != '{' && prev != '[' && prev != ',' {
				lastComma = i
			}
			prev = c
		case c == '}' || c == ']':
			if lastComma >= 0 {
				out[lastComma] = ' '
			}
			lastComma, prev = -1, c
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// whitespace keeps a pending comma pending
		default:
			lastComma, prev = -1, c
		}
	}
	return out
}
