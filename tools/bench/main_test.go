// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"strings"
	"testing"
)

const sample = `goos: windows
BenchmarkStatusOffline-20    	      10	 170000000 ns/op	         5.000 git/op
BenchmarkStatusOffline-20    	      10	 210000000 ns/op	         5.000 git/op
BenchmarkStatusOffline-20    	      10	 190000000 ns/op	         5.000 git/op
BenchmarkStatusOfflineOther-20	      10	 1 ns/op
PASS
`

func TestParseAndEvaluate(t *testing.T) {
	res := parse(strings.NewReader(sample), "BenchmarkStatusOffline")
	if len(res.Ns) != 3 || len(res.Git) != 3 {
		t.Fatalf("parsed %+v", res)
	}
	five, four := 5.0, 4.0
	if r := evaluate(Budget{Name: "BenchmarkStatusOffline", MaxNs: 2e8, MaxGit: &five}, res, nil); r.problem != "" || r.ns != 1.9e8 {
		t.Errorf("within budget: %+v", r)
	}
	if r := evaluate(Budget{Name: "BenchmarkStatusOffline", MaxNs: 1e8, MaxGit: &four}, res, nil); r.problem != "too slow, too many git processes" {
		t.Errorf("over budget: %q", r.problem)
	}
	if r := evaluate(Budget{Name: "BenchmarkX", MaxGit: &four}, Result{}, nil); r.problem != "did not run" {
		t.Errorf("missing: %q", r.problem)
	}
	var out bytes.Buffer
	if report(&out, []row{evaluate(Budget{Name: "BenchmarkStatusOffline", MaxGit: &four}, res, nil)}) {
		t.Error("report must fail over budget")
	}
	if !strings.Contains(out.String(), "190ms") {
		t.Errorf("table:\n%s", out.String())
	}
}

// TestBudgetsFileIsValid reads the committed budgets.
func TestBudgetsFileIsValid(t *testing.T) {
	budgets, err := readBudgets("budgets.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(budgets) == 0 {
		t.Fatal("no budget")
	}
}
