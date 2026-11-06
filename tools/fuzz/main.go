// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command fuzz drives the weekly long fuzzing (.github/workflows/fuzz.yml,
// docs/design/workflows.md §8). Run it from the module root:
//
//	go run ./tools/fuzz -list                                  # targets, as JSON
//	go run ./tools/fuzz -run -package P -name FuzzX -time 10m -out DIR
//
// With -run, a failure exits 1 and leaves in DIR what the tracking issue
// needs: title.txt, report.md (reproducer paths and command, output tail),
// crashers/ (the failing inputs go wrote under testdata/fuzz) and corpus/
// (the corpus generated in the build cache).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/tools/internal/fuzztargets"
)

// maxCorpusFiles caps the corpus copied into the artifact.
const maxCorpusFiles = 2000

func main() {
	list := flag.Bool("list", false, "print the fuzz targets as a JSON array")
	runTarget := flag.Bool("run", false, "fuzz one target")
	pkg := flag.String("package", "", "import path of the target's package (-run)")
	name := flag.String("name", "", "name of the Fuzz function (-run)")
	duration := flag.Duration("time", 10*time.Minute, "how long to fuzz (-run)")
	out := flag.String("out", "fuzz-report", "directory for the failure report (-run)")
	flag.Parse()

	switch {
	case *list:
		targets, err := fuzztargets.List(".")
		if err != nil {
			fail(err)
		}
		if targets == nil {
			targets = []fuzztargets.Target{}
		}
		if err := json.NewEncoder(os.Stdout).Encode(targets); err != nil {
			fail(err)
		}
	case *runTarget:
		if *pkg == "" || !strings.HasPrefix(*name, "Fuzz") {
			fail(fmt.Errorf("-run needs -package and -name FuzzXxx"))
		}
		failed, err := fuzzOne(fuzztargets.Target{Package: *pkg, Name: *name}, *duration, *out)
		if err != nil {
			fail(err)
		}
		if failed {
			os.Exit(1)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fuzz:", err)
	os.Exit(2)
}

// fuzzOne fuzzes t and, when it fails, writes the report into out.
func fuzzOne(t fuzztargets.Target, duration time.Duration, out string) (failed bool, err error) {
	dir, err := goOutput("list", "-f", "{{.Dir}}", t.Package)
	if err != nil {
		return false, err
	}
	cache, err := goOutput("env", "GOCACHE")
	if err != nil {
		return false, err
	}
	seeds := filepath.Join(dir, "testdata", "fuzz", t.Name)
	before, err := filesIn(seeds)
	if err != nil {
		return false, err
	}

	var output bytes.Buffer
	cmd := exec.Command("go", "test", "-run=^$", "-fuzz=^"+t.Name+"$",
		"-fuzztime="+duration.String(), "-timeout="+(duration+10*time.Minute).String(), t.Package)
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	runErr := cmd.Run()
	if runErr == nil {
		return false, nil
	}
	if _, ok := runErr.(*exec.ExitError); !ok {
		return false, runErr // go itself could not run
	}

	after, err := filesIn(seeds)
	if err != nil {
		return true, err
	}
	var crashers []string
	for _, f := range after {
		if !slices.Contains(before, f) {
			crashers = append(crashers, f)
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return true, err
	}
	relDir, err := filepath.Rel(root, dir)
	if err != nil {
		relDir = dir
	}
	if err := os.MkdirAll(filepath.Join(out, "crashers"), 0o755); err != nil {
		return true, err
	}
	for _, f := range crashers {
		if err := copyFile(filepath.Join(seeds, f), filepath.Join(out, "crashers", f)); err != nil {
			return true, err
		}
	}
	corpus := filepath.Join(cache, "fuzz", t.Package, t.Name)
	if err := copyCorpus(corpus, filepath.Join(out, "corpus")); err != nil {
		return true, err
	}
	title, report := failureReport(t, filepath.ToSlash(relDir), crashers, output.String())
	if err := os.WriteFile(filepath.Join(out, "title.txt"), []byte(title+"\n"), 0o644); err != nil {
		return true, err
	}
	return true, os.WriteFile(filepath.Join(out, "report.md"), []byte(report), 0o644)
}

// failureReport returns the issue title and body of a failing target.
func failureReport(t fuzztargets.Target, dir string, crashers []string, output string) (title, body string) {
	title = fmt.Sprintf("fuzz: %s fails in %s", t.Name, dir)
	var b strings.Builder
	fmt.Fprintf(&b, "The weekly long fuzzing (`.github/workflows/fuzz.yml`) found a failure in `%s` (`%s`).\n\n", t.Name, t.Package)
	if len(crashers) == 0 {
		b.WriteString("No failing input was written (a timeout or a crash of the fuzzer itself?): see the output below.\n\n")
	} else {
		b.WriteString("Reproduce: download the run's `fuzz-" + t.Name + "` artifact, copy `crashers/` into the path below, then run the command.\n\n")
		for _, c := range crashers {
			path := dir + "/testdata/fuzz/" + t.Name + "/" + c
			fmt.Fprintf(&b, "- `%s`\n  ```sh\n  go test -run='^%s$/^%s$' ./%s\n  ```\n", path, t.Name, c, dir)
		}
		b.WriteString("\nOnce fixed, commit the input under `testdata/fuzz/`: it becomes a regression test.\n\n")
	}
	b.WriteString("Last lines of the output:\n\n```text\n" + tail(output, 60) + "\n```\n")
	return title, b.String()
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	// A fence inside the output must not close the code block.
	return strings.ReplaceAll(strings.Join(lines, "\n"), "```", "'''")
}

func goOutput(args ...string) (string, error) {
	out, err := exec.Command("go", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// filesIn lists the file names of dir; a missing dir is empty.
func filesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, err
}

func copyCorpus(from, to string) error {
	copied := 0
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == from {
			return filepath.SkipDir
		}
		if err != nil || d.IsDir() {
			return err
		}
		if copied >= maxCorpusFiles {
			return filepath.SkipAll
		}
		copied++
		return copyFile(path, filepath.Join(to, d.Name()))
	})
	return err
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}
