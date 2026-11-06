// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package situation

import (
	"testing"

	"github.com/GregoireF/ship-it/internal/core/review"
)

func TestNext(t *testing.T) {
	feature := Facts{Branch: "feat/vat", Upstream: "origin/feat/vat", Base: "main", AheadOfBase: 2, GitHub: GitHubKnown}
	with := func(change func(*Facts)) Facts {
		f := feature
		change(&f)
		return f
	}
	tests := []struct {
		name  string
		facts Facts
		want  string
	}{
		{"rebase in progress wins", with(func(f *Facts) { f.OperationInProgress = "rebase"; f.Uncommitted = 3 }), "status.next.finish_operation"},
		{"uncommitted work", with(func(f *Facts) { f.Uncommitted = 3 }), "status.next.ship"},
		{"clean trunk", Facts{Branch: "main", OnTrunk: true}, "status.next.start"},
		{"empty repository", Facts{Branch: "main", Unborn: true}, "status.next.start"},
		{"remote moved", with(func(f *Facts) { f.Behind = 1; f.Ahead = 1 }), "status.next.pull"},
		{"branch without work", with(func(f *Facts) { f.AheadOfBase = 0 }), "status.next.nothing_ahead"},
		{"never pushed", with(func(f *Facts) { f.Upstream = "" }), "status.next.publish"},
		{"commits not pushed", with(func(f *Facts) { f.Ahead = 2 }), "status.next.publish"},
		{"pushed, no pull request", feature, "status.next.publish"},
		{"remote not on GitHub", with(func(f *Facts) { f.Upstream, f.ForeignRemote, f.Remote = "", true, "origin" }), "status.next.local_only"},
		{"no remote", with(func(f *Facts) { f.Upstream, f.NoRemote, f.Remote = "", true, "origin" }), "status.next.local_only_no_remote"},
		{"draft", with(func(f *Facts) { f.PullRequest = &PullRequest{Number: 7, Draft: true} }), "status.next.mark_ready"},
		{"in review", with(func(f *Facts) { f.PullRequest = &PullRequest{Number: 7} }), "status.next.await_review"},
		{"offline, pushed", with(func(f *Facts) { f.GitHub = GitHubUnknown }), "status.next.up_to_date"},
	}
	for _, tt := range tests {
		if got := Next(tt.facts); got.Key != tt.want {
			t.Errorf("%s: Next = %s, want %s", tt.name, got.Key, tt.want)
		}
	}
}

func TestNextFollowsChecksAndReviews(t *testing.T) {
	base := Facts{Branch: "feat/vat", Upstream: "origin/feat/vat", Base: "main", AheadOfBase: 1, GitHub: GitHubKnown}
	pr := func(mut func(*PullRequest)) Facts {
		f := base
		p := PullRequest{Number: 7, URL: "https://github.com/acme/shop/pull/7"}
		mut(&p)
		f.PullRequest = &p
		return f
	}
	for _, tt := range []struct {
		name  string
		facts Facts
		want  string
	}{
		{"red check first, even in a draft", pr(func(p *PullRequest) {
			p.Draft, p.CI = true, review.CI{State: review.CIFailing, Failing: []string{"lint", "test"}}
		}), "status.next.fix_ci"},
		{"changes requested", pr(func(p *PullRequest) {
			p.Review = review.Verdict{Decision: review.ChangesRequested, Requesters: []string{"bob"}}
		}), "status.next.address_review"},
		{"draft", pr(func(p *PullRequest) { p.Draft = true }), "status.next.mark_ready"},
		{"checks running", pr(func(p *PullRequest) { p.CI = review.CI{State: review.CIPending, Pending: 2} }), "status.next.await_ci"},
		{"approved and green", pr(func(p *PullRequest) {
			p.CI, p.Review = review.CI{State: review.CIPassing}, review.Verdict{Decision: review.Approved, Approvers: []string{"ada"}}
		}), "status.next.merge"},
		{"nothing yet", pr(func(*PullRequest) {}), "status.next.await_review"},
	} {
		if got := Next(tt.facts); got.Key != tt.want {
			t.Errorf("%s: %s, want %s", tt.name, got.Key, tt.want)
		}
	}
	if got := Next(pr(func(p *PullRequest) { p.CI = review.CI{State: review.CIFailing, Failing: []string{"lint", "test"}} })); got.Params["checks"] != "lint, test" {
		t.Errorf("params = %v", got.Params)
	}
}
