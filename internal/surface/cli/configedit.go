// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/ports"
)

// configSubcommand runs `ship config get|set|unset|explain`; ok is false for
// any other argument, left to `ship config`.
func (r runner) configSubcommand(args []string) (code int, ok bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "get":
		return r.configGet(args[1:]), true
	case "set":
		return r.configEdit("set", args[1:]), true
	case "unset":
		return r.configEdit("unset", args[1:]), true
	case "explain":
		return r.configExplain(args[1:]), true
	}
	return 0, false
}

// parseInterspersed parses flags wherever they are, so that
// `ship config set <key> <value> --user` works; after "--" everything is an
// argument (a value starting with "-").
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	if i := slices.Index(args, "--"); i >= 0 {
		args, rest = args[:i], args[i+1:]
	}
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return append(positional, rest...), nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// configValueText shows a JSON value for humans: a string without quotes.
func configValueText(value string) string {
	var s string
	if json.Unmarshal([]byte(value), &s) == nil {
		return s
	}
	return value
}

func (r runner) configGet(args []string) int {
	fs, asJSON := r.flags("config get")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return r.parseFailed(fs, err)
	}
	if len(positional) != 1 {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": "<key>"}))
		return ExitUsage
	}
	value, err := app.GetSetting(r.ctx, r.deps, positional[0])
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(value)
	}
	fmt.Fprintln(r.streams.Out, configValueText(string(value.Value)))
	switch {
	case value.Source == "default":
		r.note(i18n.Render(r.lang, "cli.config.get.default", nil))
	case value.Path != "":
		r.note(i18n.Render(r.lang, "cli.config.get.from_file", map[string]string{"source": value.Source, "file": value.Path}))
	default:
		r.note(i18n.Render(r.lang, "cli.config.get.from", map[string]string{"source": value.Source}))
	}
	return ExitOK
}

func (r runner) configEdit(action string, args []string) int {
	fs, asJSON := r.flags("config " + action)
	user := fs.Bool("user", false, "edit the user file")
	repo := fs.Bool("repo", false, "edit .github/ship.json of the repository (default inside one)")
	dryRun := dryRunFlag(fs)
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return r.parseFailed(fs, err)
	}
	r = r.rehearse(*dryRun)
	want, argument := 2, "<key> <value>"
	if action == "unset" {
		want, argument = 1, "<key>"
	}
	if len(positional) != want {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": argument}))
		return ExitUsage
	}
	if *user && *repo {
		r.status(i18n.Render(r.lang, "cli.config.one_scope", nil))
		return ExitUsage
	}
	req := app.ConfigEditRequest{Key: positional[0]}
	switch {
	case *user:
		req.Scope = ports.ScopeUser
	case *repo:
		req.Scope = ports.ScopeRepository
	}
	var result app.ConfigEditResult
	if action == "set" {
		req.Value = positional[1]
		result, err = app.SetSetting(r.ctx, r.deps, req)
	} else {
		result, err = app.UnsetSetting(r.ctx, r.deps, req)
	}
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(result)
	}
	if *dryRun && result.Changed {
		return r.printPlanned(result.Rehearsal)
	}
	params := map[string]string{"key": result.Key, "value": configValueText(string(result.Value)), "file": result.Path, "scope": result.Scope}
	key := "cli.config." + action + ".done"
	switch {
	case !result.Changed && action == "set":
		key = "cli.config.set.unchanged"
	case !result.Changed:
		key = "cli.config.unset.absent"
	case result.Operation == "":
		key += "_local"
	}
	r.note(i18n.Render(r.lang, key, params))
	return ExitOK
}

func (r runner) configExplain(args []string) int {
	fs, asJSON := r.flags("config explain")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return r.parseFailed(fs, err)
	}
	if len(positional) != 1 {
		r.status(i18n.Render(r.lang, "cli.error.missing_argument", map[string]string{"argument": "<key>"}))
		return ExitUsage
	}
	x, err := app.ExplainSetting(r.ctx, r.deps, positional[0])
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(x)
	}
	r.printExplanation(x)
	return ExitOK
}

func (r runner) printExplanation(x app.SettingExplanation) {
	out := r.streams.Out
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	fmt.Fprintln(out, x.Key)
	if x.Doc != "" {
		fmt.Fprintln(out, "  "+x.Doc)
	}
	fmt.Fprintln(out, "  "+t("cli.config.explain.type", map[string]string{"type": t("cli.config.type."+x.Type, nil)}))
	if len(x.Allowed) > 0 {
		fmt.Fprintln(out, "  "+t("cli.config.explain.allowed", map[string]string{"allowed": strings.Join(x.Allowed, ", ")}))
	}
	if x.Minimum != nil {
		fmt.Fprintln(out, "  "+t("cli.config.explain.minimum", map[string]string{"minimum": strconv.Itoa(*x.Minimum)}))
	}
	fmt.Fprintln(out, "  "+t("cli.config.explain.default", map[string]string{"value": x.Default}))
	fmt.Fprintln(out, "  "+t("cli.config.explain.env", map[string]string{"env": x.Env}))
	if len(x.Layers) == 0 {
		fmt.Fprintln(out, "  "+t("cli.config.explain.no_layer", nil))
	} else {
		fmt.Fprintln(out, "  "+t("cli.config.explain.layers", nil))
		sourceWidth, valueWidth := 0, 0
		for _, l := range x.Layers {
			sourceWidth = max(sourceWidth, utf8.RuneCountInString(l.Source))
			valueWidth = max(valueWidth, utf8.RuneCountInString(l.Value))
		}
		for _, l := range x.Layers {
			marker := "  "
			if l.Source == x.Source {
				marker = "→ "
			}
			fmt.Fprintf(out, "    %s%s%s  %s%s  %s\n", marker, l.Source, strings.Repeat(" ", sourceWidth-utf8.RuneCountInString(l.Source)),
				l.Value, strings.Repeat(" ", valueWidth-utf8.RuneCountInString(l.Value)), l.Path)
		}
	}
	fmt.Fprintln(out, "  "+t("cli.config.explain.effective", map[string]string{"value": x.Value, "source": x.Source}))
	switch {
	case x.Source == "default":
		fmt.Fprintln(out, "  "+t("cli.config.explain.why_default", nil))
	case x.Source == "policy" && x.Policy != nil:
		fmt.Fprintln(out, "  "+t("cli.config.explain.why_policy", map[string]string{"organization": x.Policy.Organization, "contact": x.Policy.Contact}))
	default:
		fmt.Fprintln(out, "  "+t("cli.config.explain.why_layer", map[string]string{"source": x.Source}))
	}
}
