// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// docs/reference/errors.md lists every SHIP-#### code with the messages ship
// prints for it. Nothing in it is written by hand: the codes and their notes
// come from internal/core/fault, the messages from the English catalogue, and
// which messages go with which code from the source itself — every call or
// literal that names a fault.X code next to catalogue keys (fault.New,
// ProblemView{Code, Key}, doctor checks, remedies). A message added to the
// code reaches the page at the next `go run ./tools/gen`.

const errorsFile = "docs/reference/errors.md"

// faultCode is one constant of package fault.
type faultCode struct {
	Name, Value, Note, Group string
}

// faultCodes reads the constants of internal/core/fault/fault.go in order,
// with the note of each one and the title of its block.
func faultCodes(root string) ([]faultCode, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "internal", "core", "fault", "fault.go"), nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var codes []faultCode
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		group := groupTitle(gen.Doc.Text())
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			if len(value.Values) != 1 {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}
			text, _ := strconv.Unquote(literal.Value)
			if strings.HasPrefix(text, "SHIP-I") {
				// A passing check, not a problem to look up.
				continue
			}
			name := value.Names[0].Name
			note := value.Doc.Text()
			if gen.Lparen == token.NoPos {
				note = ""
			}
			codes = append(codes, faultCode{Name: name, Value: text, Note: codeNote(name, note), Group: group})
		}
	}
	return codes, nil
}

var rangeSuffix = regexp.MustCompile(`\s*\(\d\dxx range\).*$`)

// groupTitle turns "Codes of the convention package (01xx range)." into
// "Convention package".
func groupTitle(doc string) string {
	doc = strings.TrimSpace(strings.SplitN(doc, "\n", 2)[0])
	if !strings.HasPrefix(doc, "Codes of ") {
		return "General"
	}
	title := rangeSuffix.ReplaceAllString(strings.TrimPrefix(doc, "Codes of "), "")
	title = strings.TrimPrefix(strings.TrimSuffix(title, "."), "the ")
	return strings.ToUpper(title[:1]) + title[1:]
}

// codeNote turns "ConfigFileChanged: a configuration file…" into "A
// configuration file…", on one line.
func codeNote(name, doc string) string {
	note := strings.Join(strings.Fields(strings.TrimPrefix(doc, name+":")), " ")
	if note == "" {
		return ""
	}
	return strings.ToUpper(note[:1]) + note[1:]
}

// faultKeys finds, in the Go sources under internal/, the catalogue keys
// used together with each fault code: in the same call or composite literal
// as a fault.X selector. A key held in a variable is followed to the string
// literals assigned to it in the same function, and a key built as
// "prefix." + value stands for every key of that prefix.
func faultKeys(root string, catalogue []string) (map[string][]string, error) {
	known := map[string]bool{}
	for _, key := range catalogue {
		known[key] = true
	}
	found := map[string]map[string]bool{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			assigned := assignedStrings(fn.Body)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				var parts []ast.Expr
				switch node := n.(type) {
				case *ast.CallExpr:
					parts = node.Args
				case *ast.CompositeLit:
					for _, elt := range node.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							parts = append(parts, kv.Value)
						}
					}
				default:
					return true
				}
				var codes, keys []string
				for _, part := range parts {
					if name := faultSelector(part); name != "" {
						codes = append(codes, name)
						continue
					}
					keys = append(keys, keysOf(part, assigned, known, catalogue)...)
				}
				for _, code := range codes {
					if found[code] == nil {
						found[code] = map[string]bool{}
					}
					for _, key := range keys {
						found[code][key] = true
					}
				}
				return true
			})
		}
		return nil
	})
	result := map[string][]string{}
	for code, keys := range found {
		for key := range keys {
			result[code] = append(result[code], key)
		}
		slices.Sort(result[code])
	}
	return result, err
}

// faultSelector returns X for fault.X or string(fault.X), else "".
func faultSelector(expr ast.Expr) string {
	if call, ok := expr.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "string" {
			expr = call.Args[0]
		}
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "fault" {
		return selector.Sel.Name
	}
	return ""
}

// keysOf returns the catalogue keys an argument stands for.
func keysOf(expr ast.Expr, assigned map[string][]string, known map[string]bool, catalogue []string) []string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if text, err := strconv.Unquote(e.Value); err == nil && known[text] {
			return []string{text}
		}
	case *ast.Ident:
		var keys []string
		for _, text := range assigned[e.Name] {
			if known[text] {
				keys = append(keys, text)
			}
		}
		return keys
	case *ast.BinaryExpr:
		left, ok := e.X.(*ast.BasicLit)
		if !ok || e.Op != token.ADD {
			return nil
		}
		prefix, err := strconv.Unquote(left.Value)
		if err != nil || !strings.HasSuffix(prefix, ".") {
			return nil
		}
		var keys []string
		for _, key := range catalogue {
			if strings.HasPrefix(key, prefix) {
				keys = append(keys, key)
			}
		}
		return keys
	}
	return nil
}

// assignedStrings maps each local variable of body to the string literals
// assigned to it.
func assignedStrings(body *ast.BlockStmt) map[string][]string {
	assigned := map[string][]string{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			literal, isLiteral := assign.Rhs[i].(*ast.BasicLit)
			if !ok || !isLiteral || literal.Kind != token.STRING {
				continue
			}
			if text, err := strconv.Unquote(literal.Value); err == nil {
				assigned[ident.Name] = append(assigned[ident.Name], text)
			}
		}
		return true
	})
	return assigned
}

// exitStatus says how ship ends on a code, following internal/app and the
// ranges documented in package fault.
func exitStatus(code string) string {
	switch {
	case strings.HasPrefix(code, "SHIP-I"):
		return "not an error"
	case strings.HasPrefix(code, "SHIP-W"):
		return "warning: ship goes on"
	case app.NeedsHuman(fault.New(fault.Code(code), "")):
		return "exit code 2: a person must act"
	case app.IsPolicyRefusal(fault.New(fault.Code(code), "")):
		return "exit code 5: refused by the organisation policy"
	case strings.HasPrefix(code, "SHIP-E09"):
		return "exit code 3: a safety check refused the work"
	default:
		return "exit code 1"
	}
}

// errorsReference renders docs/reference/errors.md.
func errorsReference(root string) ([]byte, error) {
	codes, err := faultCodes(root)
	if err != nil {
		return nil, err
	}
	catalogue := i18n.Keys()
	keys, err := faultKeys(root, catalogue)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(codes, func(a, b faultCode) int { return strings.Compare(a.Value[6:], b.Value[6:]) })

	var b bytes.Buffer
	b.WriteString("<!-- Generated by go run ./tools/gen from internal/core/fault and the English catalogue. Do not edit. -->\n\n")
	b.WriteString("# Error codes\n\n")
	b.WriteString("Every problem ship reports carries a stable code: `SHIP-E` for errors, `SHIP-W` for warnings, `SHIP-I` for information. Codes are never reused, so you can search for one. ")
	b.WriteString("The messages below are the ones ship prints, with `{placeholders}` for the values it fills in; most of them say what to do. ")
	b.WriteString("`ship doctor` finds the most common problems before they stop you, and `ship fix` repairs several of them.\n\n")
	b.WriteString("Exit codes: `0` success, `1` error, `2` a person must act, `3` a safety check refused the work, `5` refused by the organisation policy, `64` wrong usage, `130` cancelled.\n")

	group := ""
	for _, code := range codes {
		if code.Group != group {
			group = code.Group
			fmt.Fprintf(&b, "\n## %s\n", group)
		}
		fmt.Fprintf(&b, "\n### %s\n\n", code.Value)
		if code.Note != "" {
			fmt.Fprintf(&b, "%s\n\n", code.Note)
		}
		fmt.Fprintf(&b, "*%s.*\n", strings.ToUpper(exitStatus(code.Value)[:1])+exitStatus(code.Value)[1:])
		// Messages first, then what ship suggests: a doctor remedy to type, or
		// what ship fix does.
		var messages, remedies, fixes []string
		for _, key := range keys[code.Name] {
			message := strings.ReplaceAll(i18n.Render(i18n.English, key, nil), "\n", " ")
			switch {
			case strings.HasPrefix(key, "doctor.remedy."):
				remedies = append(remedies, strings.Join(strings.Fields(message), " "))
			case strings.HasPrefix(key, "fix."):
				fixes = append(fixes, message)
			default:
				messages = append(messages, message)
			}
		}
		for _, message := range messages {
			fmt.Fprintf(&b, "\n> %s\n", message)
		}
		if len(remedies) > 0 {
			fmt.Fprintf(&b, "\nTo fix it: %s\n", strings.Join(remedies, " · "))
		}
		for _, fix := range fixes {
			fmt.Fprintf(&b, "\n`ship fix` can do it: %s.\n", strings.TrimSuffix(fix, "."))
		}
	}
	return b.Bytes(), nil
}
