// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package review

import (
	"reflect"
	"testing"
)

func TestSummarize(t *testing.T) {
	ok := CheckRun{Name: "test", Status: "completed", Conclusion: "success"}
	for _, tt := range []struct {
		name     string
		runs     []CheckRun
		statuses []CommitStatus
		want     CI
	}{
		{"no checks", nil, nil, CI{State: CINone}},
		{"all green", []CheckRun{ok, {Name: "lint", Status: "completed", Conclusion: "skipped"}}, []CommitStatus{{Context: "ci/old", State: "success"}}, CI{State: CIPassing, Total: 3}},
		{"pending", []CheckRun{ok, {Name: "e2e", Status: "in_progress"}}, nil, CI{State: CIPending, Pending: 1, Total: 2}},
		{"failing beats pending", []CheckRun{{Name: "e2e", Status: "queued"}, {Name: "lint", Status: "completed", Conclusion: "failure"}},
			[]CommitStatus{{Context: "deploy", State: "error"}}, CI{State: CIFailing, Failing: []string{"deploy", "lint"}, Pending: 1, Total: 3}},
		{"cancelled and timed out fail", []CheckRun{{Name: "b", Status: "completed", Conclusion: "cancelled"}, {Name: "a", Status: "completed", Conclusion: "timed_out"}},
			nil, CI{State: CIFailing, Failing: []string{"a", "b"}, Total: 2}},
	} {
		if got := Summarize(tt.runs, tt.statuses); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestDecide(t *testing.T) {
	for _, tt := range []struct {
		name    string
		reviews []Review
		want    Verdict
	}{
		{"none", nil, Verdict{Decision: NoDecision}},
		{"comments only", []Review{{"ada", "COMMENTED"}, {"bob", "PENDING"}}, Verdict{Decision: NoDecision}},
		{"approved", []Review{{"ada", "APPROVED"}, {"bob", "COMMENTED"}}, Verdict{Decision: Approved, Approvers: []string{"ada"}}},
		{"latest review of a reviewer stands", []Review{{"ada", "CHANGES_REQUESTED"}, {"Ada", "APPROVED"}}, Verdict{Decision: Approved, Approvers: []string{"ada"}}},
		{"one change request blocks", []Review{{"ada", "APPROVED"}, {"bob", "CHANGES_REQUESTED"}},
			Verdict{Decision: ChangesRequested, Approvers: []string{"ada"}, Requesters: []string{"bob"}}},
		{"dismissed request clears", []Review{{"bob", "CHANGES_REQUESTED"}, {"bob", "DISMISSED"}, {"ada", "APPROVED"}},
			Verdict{Decision: Approved, Approvers: []string{"ada"}}},
	} {
		if got := Decide(tt.reviews); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
