// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcpconfig

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// tomlTableFor declares ship-it for Codex. "ship-it" is a valid bare key.
func tomlTableFor(c Client) string {
	return "[mcp_servers.ship-it]\ncommand = \"ship\"\nargs = [\"mcp\", \"--client\", \"" + string(c) + "\"]\n"
}

var (
	// ourHeader is a table header naming ship-it's server, quoted or not.
	ourHeader = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*mcp_servers[ \t]*\.[ \t]*("ship-it"|'ship-it'|ship-it)[ \t]*\][ \t]*(#.*)?\r?$`)
	// inlineServers is mcp_servers given as a value (an inline table) or as
	// an array of tables: a [mcp_servers.ship-it] table cannot extend either.
	inlineServers = regexp.MustCompile(`(?m)^[ \t]*(mcp_servers[ \t]*=|\[\[[ \t]*mcp_servers\b)`)
)

// legacyTOMLTable is what ship mcp install appended before --client (0.1).
const legacyTOMLTable = "[mcp_servers.ship-it]\ncommand = \"ship\"\nargs = [\"mcp\"]\n"

// removeTOML takes out the table ship mcp install appended, with the blank
// line before it, and changes nothing else. Any other ship-it table is the
// user's: without a TOML parser its end cannot be told (ErrManualEdit).
func removeTOML(existing []byte, c Client) (Removal, error) {
	lf := bytes.ReplaceAll(existing, []byte("\r\n"), []byte("\n"))
	for _, table := range []string{tomlTableFor(c), legacyTOMLTable} {
		text := "\n" + string(lf)
		at := strings.Index(text, "\n"+table)
		if at < 0 {
			continue
		}
		before, after := text[:at], text[at+1+len(table):]
		before = strings.TrimSuffix(before, "\n") // the blank line addTOML adds
		out := strings.TrimPrefix(before+"\n"+after, "\n")
		if strings.TrimSpace(out) == "" {
			return Removal{Changed: true, Empty: true}, nil
		}
		if bytes.Contains(existing, []byte("\r\n")) {
			out = strings.ReplaceAll(out, "\n", "\r\n")
		}
		return Removal{Content: []byte(out), Changed: true}, nil
	}
	if ourHeader.Match(existing) {
		return Removal{}, fmt.Errorf("%w: its %s table was not written by ship mcp install", ErrManualEdit, ServerName)
	}
	return Removal{Content: existing}, nil
}

// addTOML appends the ship-it table to a Codex config.toml and changes
// nothing else: the standard library has no TOML parser, so the file is never
// rewritten, only extended. Whenever appending could clash with what the file
// already holds, it refuses with ErrManualEdit.
func addTOML(existing []byte, tomlTable string) (Result, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return Result{Content: []byte(tomlTable), Changed: true}, nil
	}
	lf := bytes.ReplaceAll(existing, []byte("\r\n"), []byte("\n"))
	switch {
	case bytes.Contains(append([]byte("\n"), lf...), []byte("\n"+tomlTable)):
		return Result{Content: existing}, nil
	case ourHeader.Match(existing):
		return Result{Content: existing, Kept: true}, nil
	case bytes.Contains(existing, []byte(ServerName)):
		// ship-it named some other way (a dotted key, an inline table, a
		// comment): a parser would be needed to tell.
		return Result{}, fmt.Errorf("%w: it already mentions %s", ErrManualEdit, ServerName)
	case inlineServers.Match(existing):
		return Result{}, fmt.Errorf("%w: mcp_servers is not a plain table", ErrManualEdit)
	}
	eol := "\n"
	if bytes.Contains(existing, []byte("\r\n")) {
		eol = "\r\n"
	}
	out := bytes.Clone(existing)
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, eol...)
	}
	out = append(out, eol...)
	out = append(out, bytes.ReplaceAll([]byte(tomlTable), []byte("\n"), []byte(eol))...)
	return Result{Content: out, Changed: true}, nil
}
