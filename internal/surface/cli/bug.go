// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package cli

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
)

// bugIssueURL is where ship-it's bugs are reported.
const bugIssueURL = "https://github.com/GregoireF/ship-it/issues/new"

// bugReport is what ship bug tells about one run (#53). Every fact comes
// from the run's journal, already scrubbed of secrets, emails and paths,
// except the git version, read now.
type bugReport struct {
	Run, Command, Version, Platform, Exit, Git string
	Problem                                    string
	Stack                                      string
	Timeline                                   string
}

// buildBugReport reads a run's events.
func buildBugReport(run string, events []obs.Event, gitVersion string) bugReport {
	b := bugReport{Run: run, Git: gitVersion}
	attr := func(e obs.Event, key string) string {
		if v, ok := e.Attrs[key]; ok && v != nil {
			if f, ok := v.(float64); ok {
				return fmt.Sprint(int64(f))
			}
			return fmt.Sprint(v)
		}
		return ""
	}
	for _, e := range events {
		switch e.Msg {
		case "run.start":
			b.Command, b.Version = attr(e, "command"), attr(e, "version")
			b.Platform = strings.Trim(attr(e, "os")+"/"+attr(e, "arch"), "/")
		case "run.end":
			b.Exit = attr(e, "exit_code")
		case "run.fault":
			b.Problem = strings.TrimSpace(attr(e, "code") + " " + attr(e, "key"))
		case "run.error":
			b.Problem = attr(e, "error")
		case "run.panic":
			b.Problem, b.Stack = "panic: "+attr(e, "panic"), attr(e, "stack")
		}
	}
	var timeline strings.Builder
	_ = obs.WriteTimeline(&timeline, obs.Steps(events))
	b.Timeline = timeline.String()
	return b
}

// Markdown is the report as it is written and attached.
func (b bugReport) Markdown() string {
	var s strings.Builder
	fmt.Fprintf(&s, "## ship bug report, run %s\n\n", b.Run)
	for _, row := range [][2]string{{"Command", "ship " + b.Command}, {"ship version", b.Version}, {"git version", b.Git},
		{"Platform", b.Platform}, {"Exit code", b.Exit}} {
		if strings.TrimSpace(row[1]) != "" && row[1] != "ship " {
			fmt.Fprintf(&s, "- %s: `%s`\n", row[0], row[1])
		}
	}
	if b.Problem != "" {
		fmt.Fprintf(&s, "\n### Error\n\n```text\n%s\n```\n", b.Problem)
	}
	if b.Stack != "" {
		fmt.Fprintf(&s, "\n### Stack\n\n```text\n%s\n```\n", strings.TrimRight(b.Stack, "\n"))
	}
	if b.Timeline != "" {
		fmt.Fprintf(&s, "\n### Timeline\n\n```text\n%s```\n", b.Timeline)
	}
	return s.String()
}

// IssueURL prefills the bug form's text fields (their ids are the URL
// parameters, per GitHub's form schema); the dropdowns are left to the
// person. The full report goes as an attachment, file.
func (b bugReport) IssueURL(file string) string {
	happened := "ship " + b.Command + " failed: " + b.Problem
	if b.Problem == "" {
		happened = "ship " + b.Command + " ended with exit code " + b.Exit + ", without an error in its journal."
	}
	happened += "\n\nThe report of run " + b.Run + " is attached: " + file
	q := url.Values{}
	q.Set("template", "1-bug-report.yml")
	q.Set("ship-version", b.Version)
	q.Set("git-version", b.Git)
	q.Set("what-happened", happened)
	q.Set("trace", "Attached: "+file)
	return bugIssueURL + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

// chooseBugRun picks the run a report is about, among runs (newest first)
// without the current one: the one named by id (whole or its last six
// characters at least), else the newest that failed, else the newest.
func chooseBugRun(runs []obs.RunFiles, current obs.RunID, id string) *obs.RunFiles {
	var past []*obs.RunFiles
	for i := range runs {
		if runs[i].ID != current {
			past = append(past, &runs[i])
		}
	}
	if id != "" {
		for _, run := range past {
			if strings.EqualFold(string(run.ID), id) || (len(id) >= 6 && strings.HasSuffix(string(run.ID), strings.ToUpper(id))) {
				return run
			}
		}
		return nil
	}
	for _, run := range past {
		if run.BlackBox {
			return run
		}
	}
	if len(past) > 0 {
		return past[0]
	}
	return nil
}

// bug builds a report from a past run's journal, writes it next to it, and
// opens a prefilled issue after the person agreed (#53). Nothing is sent by
// ship: the person attaches the file and submits the issue on GitHub.
func (r runner) bug(args []string) int {
	fs, _ := r.flags("bug")
	printOnly := fs.Bool("print", false, "print the report and the address, opening nothing")
	if code, ok := r.parse(fs, args); !ok {
		return code
	}
	t := func(key string, params map[string]string) string { return i18n.Render(r.lang, key, params) }
	dir := r.journal.Dir()
	runs, err := obs.ListRuns(dir)
	if dir == "" || err != nil {
		r.problem(t("cli.trace.no_logs", nil))
		return ExitError
	}
	chosen := chooseBugRun(runs, r.journal.RunID, fs.Arg(0))
	if chosen == nil {
		r.problem(t("cli.trace.not_found", map[string]string{"run": fs.Arg(0)}))
		return ExitError
	}
	name := string(chosen.ID) + obs.JournalSuffix
	if chosen.BlackBox {
		name = string(chosen.ID) + obs.BlackBoxSuffix
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return r.fail(err)
	}
	events, err := obs.ReadEvents(f)
	f.Close()
	if err != nil {
		return r.fail(err)
	}
	gitVersion, _ := r.deps.Git.Version(r.ctx)
	report := buildBugReport(string(chosen.ID), events, gitVersion)
	file := filepath.Join(dir, string(chosen.ID)+obs.ReportSuffix)
	if err := os.WriteFile(file, []byte(report.Markdown()), 0o600); err != nil {
		return r.fail(err)
	}
	address := report.IssueURL(filepath.Base(file))
	fmt.Fprint(r.streams.Out, report.Markdown())
	r.status("")
	r.status(t("cli.bug.written", map[string]string{"file": file}))
	r.status(t("cli.bug.address", map[string]string{"url": address}))
	if *printOnly || r.deps.Browser == nil {
		return ExitOK
	}
	if !r.streams.Interactive {
		r.note(t("cli.bug.no_terminal", nil))
		return ExitOK
	}
	if !r.confirm(t("cli.bug.confirm", nil)) {
		return ExitOK
	}
	if err := r.deps.Browser.Open(r.ctx, address); err != nil {
		r.status(t("cli.bug.open_failed", map[string]string{"error": err.Error()}))
		return ExitOK
	}
	r.note(t("cli.bug.attach", map[string]string{"file": file}))
	return ExitOK
}
