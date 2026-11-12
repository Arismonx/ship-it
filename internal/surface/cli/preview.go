// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"strings"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
)

// preview prints the pull request ship publish would open, as reviewers will
// read it, without pushing or opening anything (#43, #44).
func (r runner) preview(args []string) int {
	fs, asJSON := r.flags("preview")
	issue := fs.Int("issue", 0, "linked issue number (default: from the branch name)")
	draft := fs.Bool("draft", false, "preview it as a draft")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	if fs.NArg() > 0 {
		r.status(i18n.Render(r.lang, "cli.error.unexpected_argument_of", map[string]string{"argument": fs.Arg(0), "command": commandLine(fs.Name())}))
		return ExitUsage
	}
	view, err := app.Preview(r.ctx, r.deps, app.PublishRequest{Issue: *issue, ForceDraft: *draft})
	if err != nil {
		return r.fail(err)
	}
	if *asJSON {
		return r.writeJSON(view)
	}
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	out := r.streams.Out
	where := map[string]string{"head": view.Head, "base": view.Base, "repository": view.Repository.Owner + "/" + view.Repository.Name}
	if view.Existing != nil {
		fmt.Fprintln(out, t("cli.preview.existing", map[string]string{"number": fmt.Sprint(view.Existing.Number), "url": view.Existing.URL}))
	} else {
		key := "cli.preview.header"
		if view.Draft {
			key = "cli.preview.header_draft"
		}
		fmt.Fprintln(out, t(key, where))
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  "+view.Title)
		if len(view.Labels) > 0 {
			fmt.Fprintln(out, "  "+t("cli.preview.labels", map[string]string{"labels": strings.Join(view.Labels, ", ")}))
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, strings.TrimRight(view.Body, "\n"))
	}
	if view.Risk != nil {
		fmt.Fprintln(out)
		fmt.Fprintln(out, t("cli.preview.risk", map[string]string{"level": t("risk.level."+view.Risk.Level, nil), "points": fmt.Sprint(view.Risk.Points)}))
		for _, f := range view.Risk.Factors {
			fmt.Fprintln(out, t("cli.preview.risk_factor", map[string]string{"points": fmt.Sprint(f.Points), "reason": t(f.Key, f.Params)}))
		}
	}
	for _, w := range view.Warnings {
		r.status("  ⚠ " + w.Code + "  " + t(w.Key, w.Params))
	}
	r.note(t("cli.preview.nothing_sent", nil))
	return ExitOK
}
