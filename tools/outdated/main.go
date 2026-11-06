// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command outdated compares the pinned tool versions (tools/internal/pins)
// with their latest releases, for what Dependabot cannot update
// (docs/design/workflows.md §5). Run it from the module root:
//
//	go run ./tools/outdated                   # table on stdout
//	go run ./tools/outdated -format=markdown  # body of the tracking issue
//
// A newer release counts only once it is older than -cooldown (7 days, the
// Dependabot cooldown): a hijacked release is usually yanked by then.
// GH_TOKEN, when set, authenticates the GitHub API calls (higher rate limit).
//
// Exit codes: 0 up to date, 1 something is behind, 2 error.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GregoireF/ship-it/tools/internal/pins"
)

// release is the latest published version of a tool.
type release struct {
	Version   string
	Published time.Time
}

// client queries the release registries. The base URLs are fields so that
// tests can point them at an httptest server.
type client struct {
	http   *http.Client
	github string // https://api.github.com
	pypi   string // https://pypi.org
	token  string
}

func (c *client) latest(ctx context.Context, tool pins.Tool) (release, error) {
	switch tool.Source {
	case pins.GitHub:
		var body struct {
			TagName     string    `json:"tag_name"`
			PublishedAt time.Time `json:"published_at"`
		}
		err := c.getJSON(ctx, c.github+"/repos/"+tool.Repo+"/releases/latest", true, &body)
		return release{body.TagName, body.PublishedAt}, err
	case pins.PyPI:
		var body struct {
			Info struct {
				Version string `json:"version"`
			} `json:"info"`
			URLs []struct {
				UploadTime time.Time `json:"upload_time_iso_8601"`
			} `json:"urls"`
		}
		if err := c.getJSON(ctx, c.pypi+"/pypi/"+tool.Repo+"/json", false, &body); err != nil {
			return release{}, err
		}
		r := release{Version: body.Info.Version}
		for _, u := range body.URLs { // the files of the latest version
			if r.Published.IsZero() || u.UploadTime.Before(r.Published) {
				r.Published = u.UploadTime
			}
		}
		return r, nil
	}
	return release{}, fmt.Errorf("%s: unknown source", tool.Name)
}

func (c *client) getJSON(ctx context.Context, url string, github bool, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ship-it-tools-outdated")
	if github {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(v)
}

// row is one line of the report.
type row struct {
	tool   pins.Tool
	latest release
	behind bool
	note   string
}

func check(ctx context.Context, c *client, tools []pins.Tool, now time.Time, cooldown time.Duration) ([]row, error) {
	var rows []row
	for _, tool := range tools {
		latest, err := c.latest(ctx, tool)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tool.Name, err)
		}
		r := row{tool: tool, latest: latest}
		newer, err := isNewer(latest.Version, tool.Version)
		switch {
		case err != nil:
			return nil, fmt.Errorf("%s: %w", tool.Name, err)
		case !newer:
			r.note = "up to date"
		case now.Sub(latest.Published) < cooldown:
			r.note = fmt.Sprintf("newer, in cooldown until %s", latest.Published.Add(cooldown).Format(time.DateOnly))
		default:
			r.behind = true
			r.note = "behind"
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// isNewer reports whether version a is greater than b (both "vX.Y.Z" or
// "X.Y.Z"). A pre-release is never newer: only stable releases count.
func isNewer(a, b string) (bool, error) {
	pa, preA, err := parse(a)
	if err != nil {
		return false, err
	}
	pb, _, err := parse(b)
	if err != nil {
		return false, err
	}
	if preA {
		return false, nil
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i], nil
		}
	}
	return false, nil
}

func parse(v string) (parts [3]int, prerelease bool, err error) {
	core := strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		prerelease = core[i] == '-'
		core = core[:i]
	}
	fields := strings.Split(core, ".")
	if len(fields) != 3 {
		return parts, false, fmt.Errorf("version %q is not X.Y.Z", v)
	}
	for i, f := range fields {
		if parts[i], err = strconv.Atoi(f); err != nil {
			return parts, false, fmt.Errorf("version %q is not X.Y.Z", v)
		}
	}
	return parts, prerelease, nil
}

func render(w io.Writer, rows []row, format string) error {
	switch format {
	case "text":
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "TOOL\tPINNED\tLATEST\tSTATUS")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.tool.Name, r.tool.Version, r.latest.Version, r.note)
		}
		return tw.Flush()
	case "markdown":
		fmt.Fprintln(w, "Pinned tools that Dependabot cannot update (`tools/internal/pins`), compared with their latest releases by `go run ./tools/outdated`.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| Tool | Pinned | Latest | Status |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, r := range rows {
			status := r.note
			if r.behind {
				status = "**behind**"
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", r.tool.Name, r.tool.Version, r.latest.Version, status)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "To update: change the version in `tools/internal/pins/pins.go` and at every `# pin: <tool>` marker, then run `go run ./tools/check -workflows`.")
		return nil
	}
	return fmt.Errorf("unknown format %q (text, markdown)", format)
}

func run(ctx context.Context, args []string, stdout io.Writer, c *client, now time.Time) (int, error) {
	fs := flag.NewFlagSet("outdated", flag.ContinueOnError)
	format := fs.String("format", "text", "output format: text or markdown")
	cooldown := fs.Duration("cooldown", 7*24*time.Hour, "ignore releases younger than this")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	rows, err := check(ctx, c, pins.Tools, now, *cooldown)
	if err != nil {
		return 2, err
	}
	if err := render(stdout, rows, *format); err != nil {
		return 2, err
	}
	for _, r := range rows {
		if r.behind {
			return 1, nil
		}
	}
	return 0, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := &client{
		http:   &http.Client{Timeout: 30 * time.Second},
		github: "https://api.github.com",
		pypi:   "https://pypi.org",
		token:  os.Getenv("GH_TOKEN"),
	}
	code, err := run(ctx, os.Args[1:], os.Stdout, c, time.Now())
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "outdated:", err)
	}
	os.Exit(code)
}
