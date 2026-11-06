// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// SetValue returns doc, a JSONC object, with the value at path replaced by
// valueJSON (any JSON value), or inserted when absent, creating the missing
// intermediate objects. The rest of the file is kept byte for byte:
// comments, key order, indentation, line endings (decision of 2026-10-23:
// `ship config set` edits the file a team comments, it does not rewrite it).
// A new key goes after the last member of its object, indented like its
// siblings; when the key appears twice, the last one (the one encoding/json
// keeps) is edited.
func SetValue(doc []byte, path []string, valueJSON string) ([]byte, error) {
	if len(path) == 0 {
		return nil, errors.New("config: empty key")
	}
	var value bytes.Buffer
	if err := json.Compact(&value, []byte(valueJSON)); err != nil {
		return nil, errors.New("config: the value is not JSON: " + err.Error())
	}
	root, err := parseJSONC(doc)
	if err != nil {
		return nil, err
	}
	e := editor{doc: doc, newline: newlineOf(doc)}
	e.unit = e.indentUnit(root)
	object := root
	for i, segment := range path {
		index := object.find(segment)
		if index < 0 {
			return e.insert(object, path[i:], value.String()), nil
		}
		member := object.members[index]
		if i == len(path)-1 {
			return splice(doc, edit{member.value.start, member.value.end, value.String()}), nil
		}
		if member.value.kind != '{' {
			return nil, errors.New("config: " + strings.Join(path[:i+1], ".") + " is not an object")
		}
		object = member.value
	}
	panic("unreachable")
}

// Unset returns doc without the key at path (every occurrence of it), then
// without the parent objects left empty, so that unsetting the only key of
// "ship.pullRequest" leaves no `"pullRequest": {}` behind. A parent that still
// holds a comment is kept: comments are never dropped, except one on the same
// line as the removed key, which was about it. A missing key leaves doc as it
// is.
func Unset(doc []byte, path []string) ([]byte, error) {
	if len(path) == 0 {
		return nil, errors.New("config: empty key")
	}
	// One removal per round, then parse again: another occurrence of the key
	// (a duplicate that the removed one hid), then the emptied parents.
	for {
		root, err := parseJSONC(doc)
		if err != nil {
			return nil, err
		}
		// chain[i] is the object at path[:i], as encoding/json resolves it.
		chain := []*node{root}
		for _, segment := range path[:len(path)-1] {
			object := chain[len(chain)-1]
			index := object.find(segment)
			if index < 0 || object.members[index].value.kind != '{' {
				break
			}
			chain = append(chain, object.members[index].value)
		}
		e := editor{doc: doc}
		if len(chain) == len(path) {
			if index := chain[len(chain)-1].find(path[len(path)-1]); index >= 0 {
				doc = e.remove(chain[len(chain)-1], index)
				continue
			}
		}
		removed := false
		for depth := len(chain) - 1; depth >= 1 && !removed; depth-- {
			object := chain[depth]
			if len(object.members) == 0 && len(bytes.TrimSpace(doc[object.start+1:object.end-1])) == 0 {
				parent := chain[depth-1]
				doc = e.remove(parent, parent.find(path[depth-1]))
				removed = true
			}
		}
		if !removed {
			return doc, nil
		}
	}
}

// node is a JSON value of a parsed document, with its byte span.
type node struct {
	kind       byte // '{' for an object, '[' for an array, 0 otherwise
	start, end int  // the value, brackets included
	members    []member
}

// member is a key of an object and its value.
type member struct {
	key      string // decoded
	keyStart int
	value    *node
	comma    int // offset of the comma after the value, -1 when none
}

// find returns the index of the last member named key, or -1.
func (n *node) find(key string) int {
	for i := len(n.members) - 1; i >= 0; i-- {
		if n.members[i].key == key {
			return i
		}
	}
	return -1
}

// parseJSONC parses a JSONC document whose root is an object, as Merge reads
// it: StripJSONC then encoding/json decide what is valid, this parser only
// records where things are.
func parseJSONC(doc []byte) (*node, error) {
	dec := json.NewDecoder(bytes.NewReader(StripJSONC(doc)))
	dec.UseNumber() // 1E1000 is valid JSON, too big for a float64
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, errors.New("config: not valid JSON: " + err.Error())
	}
	if dec.More() {
		return nil, errors.New("config: more than one JSON value")
	}
	if _, isObject := root.(map[string]any); !isObject {
		return nil, errors.New("config: the document is not a JSON object")
	}
	p := parser{doc: doc}
	n, err := p.value()
	if err != nil {
		return nil, err
	}
	return n, nil
}

type parser struct {
	doc []byte
	pos int
}

var errUnexpected = errors.New("config: unexpected JSONC structure")

// skip moves past whitespace and comments.
func (p *parser) skip() {
	for p.pos < len(p.doc) {
		c := p.doc[p.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		case c == '/' && p.pos+1 < len(p.doc) && p.doc[p.pos+1] == '/':
			for p.pos < len(p.doc) && p.doc[p.pos] != '\n' {
				p.pos++
			}
		case c == '/' && p.pos+1 < len(p.doc) && p.doc[p.pos+1] == '*':
			end := bytes.Index(p.doc[p.pos+2:], []byte("*/"))
			if end < 0 {
				p.pos = len(p.doc)
			} else {
				p.pos += 2 + end + 2
			}
		default:
			return
		}
	}
}

func (p *parser) peek() byte {
	if p.pos < len(p.doc) {
		return p.doc[p.pos]
	}
	return 0
}

func (p *parser) value() (*node, error) {
	p.skip()
	n := &node{start: p.pos}
	switch p.peek() {
	case 0:
		return nil, errUnexpected
	case '{':
		n.kind = '{'
		p.pos++
		for {
			p.skip()
			if p.peek() == '}' {
				p.pos++
				n.end = p.pos
				return n, nil
			}
			if p.peek() != '"' {
				return nil, errUnexpected
			}
			m := member{keyStart: p.pos, comma: -1}
			raw := p.str()
			if err := json.Unmarshal(raw, &m.key); err != nil {
				return nil, errUnexpected
			}
			p.skip()
			if p.peek() != ':' {
				return nil, errUnexpected
			}
			p.pos++
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			m.value = v
			p.skip()
			switch p.peek() {
			case ',':
				m.comma = p.pos
				p.pos++
			case '}':
			default:
				return nil, errUnexpected
			}
			n.members = append(n.members, m)
		}
	case '[':
		n.kind = '['
		p.pos++
		for {
			p.skip()
			if p.peek() == ']' {
				p.pos++
				n.end = p.pos
				return n, nil
			}
			if _, err := p.value(); err != nil {
				return nil, err
			}
			p.skip()
			switch p.peek() {
			case ',':
				p.pos++
			case ']':
			default:
				return nil, errUnexpected
			}
		}
	case '"':
		p.str()
	default:
		for p.pos < len(p.doc) && !strings.ContainsRune(" \t\r\n,:{}[]\"/", rune(p.doc[p.pos])) {
			p.pos++
		}
	}
	n.end = p.pos
	return n, nil
}

// str moves past the string at pos and returns it, quotes included.
func (p *parser) str() []byte {
	start := p.pos
	p.pos++
	for p.pos < len(p.doc) && p.doc[p.pos] != '"' {
		if p.doc[p.pos] == '\\' {
			p.pos++
		}
		p.pos++
	}
	p.pos = min(p.pos+1, len(p.doc))
	return p.doc[start:p.pos]
}

// edit replaces doc[from:to] with text.
type edit struct {
	from, to int
	text     string
}

// splice applies non-overlapping edits.
func splice(doc []byte, edits ...edit) []byte {
	slices.SortFunc(edits, func(a, b edit) int { return b.from - a.from })
	out := slices.Clone(doc)
	for _, e := range edits {
		out = slices.Concat(out[:e.from], []byte(e.text), out[e.to:])
	}
	return out
}

// editor holds the style of the document being edited.
type editor struct {
	doc     []byte
	newline string
	unit    string // one level of indentation
}

func newlineOf(doc []byte) string {
	if bytes.Contains(doc, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// indentUnit finds one level of indentation: the difference between an
// object's line and its first member's line, for the first object written
// over several lines. Two spaces, as `ship init` writes, otherwise.
func (e editor) indentUnit(root *node) string {
	queue := []*node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if len(n.members) > 0 && e.firstOnLine(n.members[0].keyStart) {
			outer, inner := e.lineIndent(n.start), e.lineIndent(n.members[0].keyStart)
			if unit, ok := strings.CutPrefix(inner, outer); ok && unit != "" {
				return unit
			}
		}
		for _, m := range n.members {
			if m.value.kind == '{' {
				queue = append(queue, m.value)
			}
		}
	}
	return "  "
}

func (e editor) lineStart(offset int) int {
	return bytes.LastIndexByte(e.doc[:offset], '\n') + 1
}

// lineIndent returns the spaces and tabs that start the line of offset.
func (e editor) lineIndent(offset int) string {
	start := e.lineStart(offset)
	end := start
	for end < len(e.doc) && (e.doc[end] == ' ' || e.doc[end] == '\t') {
		end++
	}
	return string(e.doc[start:end])
}

// firstOnLine reports whether only spaces and tabs precede offset on its line.
func (e editor) firstOnLine(offset int) bool {
	return len(bytes.Trim(e.doc[e.lineStart(offset):offset], " \t")) == 0
}

// restOfLine returns where the line of pos ends, newline excluded, when only
// whitespace and comments follow pos on it; ok is false otherwise.
func (e editor) restOfLine(pos int) (end int, ok bool) {
	for pos < len(e.doc) {
		switch c := e.doc[pos]; {
		case c == '\n', c == '\r' && pos+1 < len(e.doc) && e.doc[pos+1] == '\n':
			return pos, true
		case c == ' ' || c == '\t' || c == '\r': // a lone CR is whitespace
			pos++
		case bytes.HasPrefix(e.doc[pos:], []byte("//")):
			// As StripJSONC reads it: up to the line feed.
			for pos < len(e.doc) && e.doc[pos] != '\n' {
				pos++
			}
			if pos < len(e.doc) && e.doc[pos-1] == '\r' {
				pos--
			}
		case bytes.HasPrefix(e.doc[pos:], []byte("/*")):
			close := bytes.Index(e.doc[pos+2:], []byte("*/"))
			if close < 0 || bytes.ContainsAny(e.doc[pos:pos+2+close], "\r\n") {
				return pos, false
			}
			pos += 2 + close + 2
		default:
			return pos, false
		}
	}
	return pos, true
}

// quoteKey writes a key as a JSON string, without HTML escaping.
func quoteKey(key string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(key) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

// memberText writes `"a": {"b": value}` for path [a b], over several lines
// indented from indent, or on one line when inline.
func (e editor) memberText(path []string, value, indent string, inline bool) string {
	key := quoteKey(path[0]) + ": "
	switch {
	case len(path) == 1:
		return key + value
	case inline:
		return key + "{ " + e.memberText(path[1:], value, "", true) + " }"
	}
	inner := indent + e.unit
	return key + "{" + e.newline + inner + e.memberText(path[1:], value, inner, false) + e.newline + indent + "}"
}

// insert adds path (its first key missing from object) after the last member.
func (e editor) insert(object *node, path []string, value string) []byte {
	if len(object.members) == 0 {
		outer := e.lineIndent(object.start)
		inner := outer + e.unit
		text := e.memberText(path, value, inner, false)
		interior := e.doc[object.start+1 : object.end-1]
		if len(bytes.TrimSpace(interior)) == 0 {
			return splice(e.doc, edit{object.start + 1, object.end - 1, e.newline + inner + text + e.newline + outer})
		}
		// Keep the comments of the empty object after the new key.
		return splice(e.doc, edit{object.start + 1, object.start + 1, e.newline + inner + text})
	}
	last := object.members[len(object.members)-1]
	anchor := last.value.end
	if last.comma >= 0 {
		anchor = last.comma + 1
	}
	if !bytes.ContainsAny(e.doc[anchor:object.end-1], "\n") {
		// The object closes on the line of its last member: stay on it.
		text := e.memberText(path, value, "", true)
		if last.comma >= 0 {
			return splice(e.doc, edit{anchor, anchor, " " + text + ","})
		}
		return splice(e.doc, edit{anchor, anchor, ", " + text})
	}
	indent := e.lineIndent(object.start) + e.unit
	if e.firstOnLine(last.keyStart) {
		indent = e.lineIndent(last.keyStart)
	}
	text := e.memberText(path, value, indent, false)
	// After the comment that ends the last member's line, if any.
	end, _ := e.restOfLine(anchor)
	if last.comma >= 0 {
		return splice(e.doc, edit{end, end, e.newline + indent + text + ","}) // keep the trailing comma style
	}
	return splice(e.doc, edit{end, end, e.newline + indent + text}, edit{last.value.end, last.value.end, ","})
}

// remove deletes object.members[index] and the comma that separated it.
func (e editor) remove(object *node, index int) []byte {
	m := object.members[index]
	to := m.value.end
	if m.comma >= 0 {
		to = m.comma + 1
	}
	// The comma before the last member goes with it (a trailing comma is
	// allowed, but the file did not have one).
	var previousComma []edit
	if m.comma < 0 && index > 0 {
		c := object.members[index-1].comma
		previousComma = []edit{{c, c + 1, ""}}
	}
	if e.firstOnLine(m.keyStart) {
		if end, ok := e.restOfLine(to); ok {
			// Whole lines: the member and the comment that ends its line.
			if end < len(e.doc) && e.doc[end] == '\r' {
				end++
			}
			if end < len(e.doc) && e.doc[end] == '\n' {
				end++
			}
			return splice(e.doc, append(previousComma, edit{e.lineStart(m.keyStart), end, ""})...)
		}
	}
	if len(previousComma) > 0 && !bytes.ContainsAny(e.doc[previousComma[0].from:m.keyStart], "\r\n/") {
		// On one line: from the previous comma to the end of the value.
		return splice(e.doc, edit{previousComma[0].from, m.value.end, ""})
	}
	for to < len(e.doc) && (e.doc[to] == ' ' || e.doc[to] == '\t') {
		to++
	}
	return splice(e.doc, append(previousComma, edit{m.keyStart, to, ""})...)
}
