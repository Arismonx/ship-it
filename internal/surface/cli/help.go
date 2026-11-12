// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/i18n"
)

// commandArguments are the arguments a command takes after its flags, shown
// in its usage line.
var commandArguments = map[string]string{
	"branch":             "<type> <subject>",
	"check title":        "<title>",
	"check branch":       "<name>",
	"checkpoint restore": "<checkpoint>",
	"config get":         "<key>",
	"config set":         "<key> <value>",
	"config unset":       "<key>",
	"config explain":     "<key>",
	"trace":              "[<run>]",
	"log":                "[--agent [<name>]]",
	"bug":                "[<run>]",
}

// commandList names the subcommands a command's help lists, by flag set
// name, with the catalogue key of the block that describes them.
var commandList = map[string]string{
	"stack": "cli.help.stack_commands",
}

// commandForms replace the usage line of a command whose forms differ by
// their flags.
var commandForms = map[string]string{
	"checkpoint": "ship checkpoint [--label <text>] [--dry-run] [--json]\n       ship checkpoint list [--json]\n       ship checkpoint restore <id> [--dry-run] [--json]",
}

// commandGroups are the commands made only of subcommands: their --help
// lists them, with the catalogue key of the block that describes them.
var commandGroups = map[string]struct{ usage, key string }{
	"check":   {"ship check <command> [flags] [arguments]", "cli.help.check_commands"},
	"mcp":     {"ship mcp [<command>] [flags]", "cli.help.mcp_commands"},
	"roadmap": {"ship roadmap <command> [flags]", "cli.help.roadmap_commands"},
}

// groupHelp prints the usage of a group of subcommands when args ask for
// it (--help or -h first); ok says it did.
func (r runner) groupHelp(name string, args []string) (ok bool) {
	if len(args) == 0 || (args[0] != "--help" && args[0] != "-h") {
		return false
	}
	group := commandGroups[name]
	fmt.Fprintln(r.streams.Out, i18n.Render(r.lang, "cli.help.usage", map[string]string{"usage": group.usage}))
	fmt.Fprintf(r.streams.Out, "\n%s\n", i18n.Render(r.lang, group.key, nil))
	return true
}

// parse parses the flags of a command, wherever they are among its
// arguments: `ship branch feat add login --issue 42` is
// `ship branch --issue 42 feat add login`. After "--" everything is an
// argument. With --help it prints the command's usage on stdout and the
// command stops with ExitOK, before anything else; a wrong flag stops it with
// ExitUsage, and so does an argument given to a command that takes none. ok
// is false when the command must stop with code.
func (r runner) parse(fs *flag.FlagSet, args []string) (code int, ok bool) {
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return r.parseFailed(fs, err), false
	}
	if _, takes := commandArguments[fs.Name()]; !takes && fs.NArg() > 0 {
		if fs.Name() == "ship" {
			// `ship --yes plan`: a command written in the wrong place.
			r.status(i18n.Render(r.lang, "cli.error.unexpected_argument", map[string]string{"argument": fs.Arg(0)}))
		} else {
			r.status(i18n.Render(r.lang, "cli.error.unexpected_argument_of", map[string]string{"argument": fs.Arg(0), "command": commandLine(fs.Name())}))
		}
		return ExitUsage, false
	}
	return ExitOK, true
}

// flagsFirst moves the flags of fs before the other arguments, each with its
// value when it takes one, so that the flag package, which stops at the first
// argument, sees them all. Unknown flags move too: parsing then rejects them.
func flagsFirst(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	if len(positional) == 0 {
		return flags
	}
	return append(append(flags, "--"), positional...)
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// parseFailed answers a flag parsing error: the usage for --help, else a
// pointer to it (the flag package already said what was wrong).
func (r runner) parseFailed(fs *flag.FlagSet, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		r.printUsage(fs)
		return ExitOK
	}
	r.status(i18n.Render(r.lang, "cli.help.hint", map[string]string{"command": commandLine(fs.Name())}))
	return ExitUsage
}

// commandLine is how a flag set is typed: "ship" alone, or "ship <name>".
func commandLine(name string) string {
	if name = strings.TrimSpace(name); name == "ship" {
		return "ship"
	}
	return "ship " + name
}

// printUsage prints a command's usage line, its subcommands if it has any,
// and its flags written as they are typed: --name, or -n for one letter, as
// in ship --help.
func (r runner) printUsage(fs *flag.FlagSet) {
	out := r.streams.Out
	usage := commandLine(fs.Name()) + " [flags]"
	if commandList[fs.Name()] != "" {
		usage = commandLine(fs.Name()) + " [<command>] [flags]"
	}
	if arguments := commandArguments[fs.Name()]; arguments != "" {
		usage += " " + arguments
	}
	if forms := commandForms[fs.Name()]; forms != "" {
		usage = forms
	}
	fmt.Fprintln(out, i18n.Render(r.lang, "cli.help.usage", map[string]string{"usage": usage}))
	if key := commandList[fs.Name()]; key != "" {
		fmt.Fprintf(out, "\n%s\n", i18n.Render(r.lang, key, nil))
	}
	type row struct{ left, usage string }
	var rows []row
	width := 0
	fs.VisitAll(func(f *flag.Flag) {
		kind, usage := flag.UnquoteUsage(f)
		left := "--" + f.Name
		if utf8.RuneCountInString(f.Name) == 1 {
			left = "-" + f.Name
		}
		if kind != "" {
			left += " " + kind
		}
		switch f.DefValue {
		case "", "false", "0", "[]":
		default:
			usage += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		rows = append(rows, row{left, usage})
		width = max(width, utf8.RuneCountInString(left))
	})
	fmt.Fprintf(out, "\n%s\n", i18n.Render(r.lang, "cli.help.flags", nil))
	for _, x := range rows {
		fmt.Fprintf(out, "  %s%s  %s\n", x.left, strings.Repeat(" ", width-utf8.RuneCountInString(x.left)), x.usage)
	}
}
