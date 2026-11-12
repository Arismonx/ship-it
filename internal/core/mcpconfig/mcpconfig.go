// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package mcpconfig adds ship-it to the project MCP configuration of a
// client (Claude Code, VS Code, Cursor, Codex) without disturbing the rest of
// the file: other servers, the order of keys and, in TOML, every comment are
// kept, and a ship-it entry the user wrote is never replaced.
//
// Allowed imports: standard library without I/O, other core packages.
package mcpconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ServerName is the key of ship-it among the servers.
const ServerName = "ship-it"

// entry is the stdio launch of ship-it. The command is "ship" from the PATH:
// every file written is a project file shared through version control, so it
// must not hold a path of one machine. --client names the agent, which ship
// records with each operation it makes (`ship log --agent`, #49).
type entry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Entry is the launch of ship-it for a client.
func (t Target) Entry() entry {
	return entry{Type: "stdio", Command: "ship", Args: []string{"mcp", "--client", string(t.Client)}}
}

// Client names an MCP client ship mcp install knows.
type Client string

// The clients, by the name --client takes.
const (
	Claude Client = "claude"
	VSCode Client = "vscode"
	Cursor Client = "cursor"
	Codex  Client = "codex"
)

// Format is the syntax of a client's configuration file.
type Format string

// Formats.
const (
	JSON Format = "json"
	TOML Format = "toml"
)

// Target is where and how a client reads its project MCP servers.
type Target struct {
	Client Client
	// Path is relative to the repository root.
	Path   string
	Format Format
	// ServersKey is the JSON object that holds the servers.
	ServersKey string
}

// Targets lists the clients, the default first. Each location comes from the
// client's own documentation, read on 2026-11-05.
var Targets = []Target{
	// https://code.claude.com/docs/en/mcp: project scope, ".mcp.json" at the
	// project root, "mcpServers", meant to be checked into version control.
	{Client: Claude, Path: ".mcp.json", Format: JSON, ServersKey: "mcpServers"},
	// https://code.visualstudio.com/docs/copilot/reference/mcp-configuration:
	// the workspace file ".vscode/mcp.json" holds "servers"; a stdio server
	// has "type": "stdio", "command" (on the PATH or a full path) and "args".
	{Client: VSCode, Path: ".vscode/mcp.json", Format: JSON, ServersKey: "servers"},
	// https://cursor.com/docs/context/mcp: the project file
	// ".cursor/mcp.json" holds "mcpServers"; stdio servers take "type",
	// "command" and "args".
	{Client: Cursor, Path: ".cursor/mcp.json", Format: JSON, ServersKey: "mcpServers"},
	// https://developers.openai.com/codex/mcp: a project-scoped
	// ".codex/config.toml" (trusted projects only), one [mcp_servers.<name>]
	// table per server with "command" and "args".
	{Client: Codex, Path: ".codex/config.toml", Format: TOML},
}

// TargetOf returns the target of a client name; "" is the default client.
func TargetOf(name string) (Target, bool) {
	if name == "" {
		return Targets[0], true
	}
	for _, t := range Targets {
		if string(t.Client) == name {
			return t, true
		}
	}
	return Target{}, false
}

// Result is the configuration after adding ship-it.
type Result struct {
	Content []byte
	// Changed: Content differs from the file and must be written.
	Changed bool
	// Kept: the file already declares a ship-it server launched another way;
	// it is the user's and stays as it is.
	Kept bool
}

// ErrManualEdit refuses a file ship-it cannot change safely: the user adds
// Snippet by hand.
var ErrManualEdit = errors.New("mcpconfig: the file must be edited by hand")

// Snippet is the configuration that declares only ship-it: the whole file
// for a new one, the part to add by hand otherwise.
func (t Target) Snippet() []byte {
	if t.Format == TOML {
		return []byte(tomlTableFor(t.Client))
	}
	out, _ := t.Add(nil)
	return out.Content
}

// Add returns the configuration with ship-it among the servers. existing may
// be empty (no file yet).
func (t Target) Add(existing []byte) (Result, error) {
	if t.Format == TOML {
		return addTOML(existing, tomlTableFor(t.Client))
	}
	return addJSON(existing, t.ServersKey, t.Entry())
}

// AddServer adds ship-it to a Claude Code .mcp.json.
func AddServer(existing []byte) ([]byte, bool, error) {
	r, err := Targets[0].Add(existing)
	return r.Content, r.Changed, err
}

// object is a JSON object with its keys in file order.
type object struct {
	keys   []string
	values map[string]json.RawMessage
}

func parseObject(data []byte) (object, error) {
	o := object{values: map[string]json.RawMessage{}}
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return o, errors.New("not a JSON object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return o, err
		}
		key, ok := t.(string)
		if !ok {
			return o, errors.New("invalid object key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return o, err
		}
		if _, dup := o.values[key]; !dup {
			o.keys = append(o.keys, key)
		}
		o.values[key] = value
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return o, errors.New("unterminated JSON object")
	}
	if dec.More() {
		return o, errors.New("more than one JSON value")
	}
	return o, nil
}

func (o *object) set(key string, value json.RawMessage) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// encode writes the object indented by two spaces, nested values included.
func (o object) encode(prefix string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, k := range o.keys {
		var value bytes.Buffer
		if err := json.Indent(&value, o.values[k], prefix+"  ", "  "); err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "%s  %s: %s", prefix, strconv.Quote(k), value.Bytes())
		if i < len(o.keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(prefix + "}")
	return b.Bytes(), nil
}

func addJSON(existing []byte, serversKey string, launch entry) (Result, error) {
	root := object{values: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(existing)) > 0 {
		var err error
		if root, err = parseObject(existing); err != nil {
			return Result{}, fmt.Errorf("mcpconfig: %w", err)
		}
	}
	servers := object{values: map[string]json.RawMessage{}}
	if raw, ok := root.values[serversKey]; ok {
		var err error
		if servers, err = parseObject(raw); err != nil {
			return Result{}, fmt.Errorf("mcpconfig: %s: %w", serversKey, err)
		}
	}
	entry, _ := json.Marshal(launch)
	if current, ok := servers.values[ServerName]; ok {
		return Result{Content: existing, Kept: !sameJSON(current, entry)}, nil
	}
	servers.set(ServerName, entry)
	encodedServers, err := servers.encode("")
	if err != nil {
		return Result{}, err
	}
	root.set(serversKey, encodedServers)
	out, err := root.encode("")
	if err != nil {
		return Result{}, err
	}
	return Result{Content: append(out, '\n'), Changed: true}, nil
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return bytes.Equal(ja, jb)
}
