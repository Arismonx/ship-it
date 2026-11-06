// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcpconfig

import (
	"bytes"
	"fmt"
	"regexp"
)

// tomlTable declares ship-it for Codex. "ship-it" is a valid bare key.
const tomlTable = "[mcp_servers.ship-it]\ncommand = \"ship\"\nargs = [\"mcp\"]\n"

var (
	// ourHeader is a table header naming ship-it's server, quoted or not.
	ourHeader = regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*mcp_servers[ \t]*\.[ \t]*("ship-it"|'ship-it'|ship-it)[ \t]*\][ \t]*(#.*)?\r?$`)
	// inlineServers is mcp_servers given as a value (an inline table) or as
	// an array of tables: a [mcp_servers.ship-it] table cannot extend either.
	inlineServers = regexp.MustCompile(`(?m)^[ \t]*(mcp_servers[ \t]*=|\[\[[ \t]*mcp_servers\b)`)
)

// addTOML appends the ship-it table to a Codex config.toml and changes
// nothing else: the standard library has no TOML parser, so the file is never
// rewritten, only extended. Whenever appending could clash with what the file
// already holds, it refuses with ErrManualEdit.
func addTOML(existing []byte) (Result, error) {
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
