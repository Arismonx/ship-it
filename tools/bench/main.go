// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command bench runs the benchmarks that have a performance budget and
// fails when one is exceeded (docs/design/performance.md). Run it from the
// module root:
//
//	go run ./tools/bench              # every budget of tools/bench/budgets.json
//	go run ./tools/bench -run Status  # those whose name matches
//	go run ./tools/bench -count=5     # more runs per benchmark (median)
//
// Two kinds of budget: a ceiling on the median time per operation, with
// headroom for slow CI machines, and a ceiling on the git processes per
// operation (git/op), which does not depend on the machine: on Windows each
// process costs about 20 ms, so their number is what a user feels.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Budget is one entry of budgets.json.
type Budget struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	// Benchtime is passed to -benchtime ("10x", "1s"); "1s" when empty.
	Benchtime string `json:"benchtime,omitempty"`
	// MaxNs is the ceiling on the median ns/op; 0: none.
	MaxNs float64 `json:"max_ns,omitempty"`
	// MaxGit is the ceiling on git/op; nil: none.
	MaxGit *float64 `json:"max_git,omitempty"`
	// Note says where the numbers come from.
	Note string `json:"note,omitempty"`
}

type budgetFile struct {
	Comment    string   `json:"$comment,omitempty"`
	Benchmarks []Budget `json:"benchmarks"`
}

// Result is what the runs of one benchmark measured.
type Result struct {
	Ns  []float64 // ns/op of each run
	Git []float64 // git/op of each run, when reported
}

func main() {
	budgetsPath := flag.String("budgets", "tools/bench/budgets.json", "budget file")
	count := flag.Int("count", 3, "runs per benchmark; the median time is compared")
	filter := flag.String("run", "", "only the benchmarks whose name matches this regular expression")
	flag.Parse()

	budgets, err := readBudgets(*budgetsPath)
	if err != nil {
		fail(err)
	}
	var only *regexp.Regexp
	if *filter != "" {
		if only, err = regexp.Compile(*filter); err != nil {
			fail(err)
		}
	}
	var rows []row
	for _, b := range budgets {
		if only != nil && !only.MatchString(b.Name) {
			continue
		}
		fmt.Fprintf(os.Stderr, "── %s %s\n", b.Package, b.Name)
		res, err := run(b, *count, os.Stderr)
		rows = append(rows, evaluate(b, res, err))
	}
	if len(rows) == 0 {
		fail(errors.New("no benchmark selected"))
	}
	if !report(os.Stdout, rows) {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bench:", err)
	os.Exit(1)
}

func readBudgets(path string) ([]Budget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f budgetFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, b := range f.Benchmarks {
		if b.Package == "" || !strings.HasPrefix(b.Name, "Benchmark") || (b.MaxNs == 0 && b.MaxGit == nil) {
			return nil, fmt.Errorf("%s: %q needs a package, a Benchmark name and a budget", path, b.Name)
		}
	}
	return f.Benchmarks, nil
}

// run executes one benchmark count times; its output is copied to log.
func run(b Budget, count int, log io.Writer) (Result, error) {
	benchtime := b.Benchtime
	if benchtime == "" {
		benchtime = "1s"
	}
	cmd := exec.Command("go", "test", "-run=^$", "-bench=^"+b.Name+"$", "-benchtime="+benchtime,
		"-count="+strconv.Itoa(count), "-timeout=30m", b.Package)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(&out, log)
	cmd.Stderr = log
	err := cmd.Run()
	res := parse(&out, b.Name)
	if err != nil {
		return res, fmt.Errorf("go test: %w", err)
	}
	return res, nil
}

// benchLine is a result line of `go test -bench`: name, -GOMAXPROCS
// suffix, iterations, then value/unit pairs.
var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+(.+)$`)

func parse(r io.Reader, name string) Result {
	var res Result
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		m := benchLine.FindStringSubmatch(sc.Text())
		if m == nil || m[1] != name {
			continue
		}
		fields := strings.Fields(m[2])
		for i := 0; i+1 < len(fields); i += 2 {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				continue
			}
			switch fields[i+1] {
			case "ns/op":
				res.Ns = append(res.Ns, v)
			case "git/op":
				res.Git = append(res.Git, v)
			}
		}
	}
	return res
}

type row struct {
	budget  Budget
	ns, git float64
	hasGit  bool
	problem string // empty: within budget
}

func evaluate(b Budget, res Result, runErr error) row {
	r := row{budget: b}
	if len(res.Ns) == 0 {
		r.problem = "did not run"
		if runErr != nil {
			r.problem += " (" + runErr.Error() + ")"
		}
		return r
	}
	r.ns = median(res.Ns)
	if len(res.Git) > 0 {
		r.git, r.hasGit = slices.Max(res.Git), true
	}
	var problems []string
	if runErr != nil {
		problems = append(problems, runErr.Error())
	}
	if b.MaxNs > 0 && r.ns > b.MaxNs {
		problems = append(problems, "too slow")
	}
	if b.MaxGit != nil {
		switch {
		case !r.hasGit:
			problems = append(problems, "no git/op reported")
		case r.git > *b.MaxGit:
			problems = append(problems, "too many git processes")
		}
	}
	r.problem = strings.Join(problems, ", ")
	return r
}

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	if n := len(s); n%2 == 0 {
		return (s[n/2-1] + s[n/2]) / 2
	}
	return s[len(s)/2]
}

// report prints the table and tells whether every budget holds.
func report(w io.Writer, rows []row) bool {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "benchmark\ttime/op\tbudget\tgit/op\tbudget\t")
	ok := true
	for _, r := range rows {
		status := "✓"
		if r.problem != "" {
			status, ok = "✗ "+r.problem, false
		}
		git, gitBudget, timeBudget := "-", "-", "-"
		if r.hasGit {
			git = strconv.FormatFloat(r.git, 'f', -1, 64)
		}
		if r.budget.MaxGit != nil {
			gitBudget = strconv.FormatFloat(*r.budget.MaxGit, 'f', -1, 64)
		}
		if r.budget.MaxNs > 0 {
			timeBudget = duration(r.budget.MaxNs)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.budget.Name, duration(r.ns), timeBudget, git, gitBudget, status)
	}
	tw.Flush()
	return ok
}

func duration(ns float64) string {
	if ns == 0 {
		return "-"
	}
	d := time.Duration(ns)
	switch {
	case d >= time.Second:
		return d.Round(10 * time.Millisecond).String()
	case d >= time.Millisecond:
		return d.Round(100 * time.Microsecond).String()
	default:
		return d.Round(100 * time.Nanosecond).String()
	}
}
