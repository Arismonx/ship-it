// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/core/settings"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/ports"
)

// PublishRequest configures a publication.
type PublishRequest struct {
	// Issue links the pull request; 0 reads it from the branch name.
	Issue int
	// Unattended: nobody reviewed the plan in a terminal, so the review moves
	// to GitHub and the pull request opens as a draft (ADR-0010), as
	// ship.pullRequest.draft says ("unattended" by default).
	Unattended bool
	// ForceDraft opens a draft whatever the settings (ship publish --draft).
	ForceDraft bool
	// SkipGates pushes without running the gates (troubleshooting); the pull
	// request body says so.
	SkipGates bool
	// GateObserver follows the gates; nil is fine.
	GateObserver GateObserver
	// Reads are those of the plan when the publication follows it at once
	// (see DeliveryRequest.Reads): the remote's default branch is not read
	// again. The status always is: the delivery has just moved HEAD.
	Reads PlanReads
	// GatesPassed are the green gates the delivery just ran on this very
	// work (ship.gates.beforeCommit): they are not run again unless the
	// branch is rebased or merged with its base first.
	GatesPassed []GateResult
}

// PublishResult reports the pushed branch and its pull request.
type PublishResult struct {
	Operation  string           `json:"operation,omitempty"`
	Repository forge.Repository `json:"repository"`
	// PullRequestRepository is where the pull request opened when it is not
	// Repository: the parent of a fork.
	PullRequestRepository *forge.Repository `json:"pull_request_repository,omitempty"`
	Branch                string            `json:"branch"`
	Base                  string            `json:"base"`
	PullRequest           ports.PullRequest `json:"pull_request"`
	// Created is false when an open pull request already existed: the push
	// updated it.
	Created bool `json:"created"`
	// Labels were added to a new pull request, among the repository's own.
	Labels []string `json:"labels,omitempty"`
	// Warnings did not stop the publication (labels could not be set…).
	Warnings []ProblemView `json:"warnings,omitempty"`
	// Gates are the checks run before the push.
	Gates []GateResult `json:"gates,omitempty"`
	// Rebased: the branch was behind the base and was replayed on it;
	// Merged: the base was merged into it (ship.publish.update merge).
	Rebased bool `json:"rebased,omitempty"`
	Merged  bool `json:"merged,omitempty"`
	// Rehearsal: on a dry run, a new pull request has no number nor URL yet.
	Rehearsal
	// ProjectStatus is the status the linked issue got in the project of
	// github.project ("In progress"); Assigned the login it was assigned to.
	ProjectStatus string `json:"project_status,omitempty"`
	Assigned      string `json:"assigned,omitempty"`
	// Stack is the GitHub stack a new layer joined (ship on a stack's top).
	Stack int `json:"stack,omitempty"`
	// Reviewers were asked to review a new pull request (users, then
	// teams as "@slug"); Assignee is who the pull request was assigned to;
	// Milestone is the milestone it was put in (ship.pullRequest.*).
	Reviewers []string `json:"reviewers,omitempty"`
	Assignee  string   `json:"assignee,omitempty"`
	Milestone int      `json:"milestone,omitempty"`
}

// requestReviewsAndMilestone applies ship.pullRequest.reviewers,
// teamReviewers, assignSelf and milestone to a new pull request. It exists
// already, so every refusal (a reviewer without access, no triage right on
// the repository) is a warning. It returns the milestone it set, for the
// operation log.
func requestReviewsAndMilestone(ctx context.Context, ws Workspace, repo forge.Repository, s config.PullRequestSettings,
	issue int, pr ports.PullRequest, result *PublishResult) int {
	warn := func(err error) { result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo))) }
	author := pr.Author
	if author == "" && (s.AssignSelf || len(s.Reviewers) > 0) {
		if user, err := ws.Forge.CurrentUser(ctx, repo); err == nil {
			author = user.Login // a dry run's pull request has no author yet
		}
	}
	// GitHub refuses a review request to the author.
	users := slices.DeleteFunc(slices.Clone(s.Reviewers), func(login string) bool { return strings.EqualFold(login, author) })
	if len(users) > 0 || len(s.TeamReviewers) > 0 {
		if err := ws.Forge.RequestReviewers(ctx, repo, pr.Number, users, s.TeamReviewers); err != nil {
			warn(err)
		} else {
			result.Reviewers = append(users, prefixed("@", s.TeamReviewers)...)
		}
	}
	if ws.Tracker == nil {
		return 0
	}
	if s.AssignSelf && author != "" {
		if err := ws.Tracker.AddAssignees(ctx, repo, pr.Number, []string{author}); err != nil {
			warn(err)
		} else {
			result.Assignee = author
		}
	}
	if s.Milestone != config.MilestoneIssue || issue == 0 {
		return 0
	}
	is, err := ws.Tracker.Issue(ctx, repo, issue)
	if err != nil || is.Milestone == 0 {
		return 0 // a missing issue or one without a milestone: nothing to follow
	}
	if err := ws.Tracker.UpdateIssue(ctx, repo, pr.Number, ports.IssueUpdate{Milestone: &is.Milestone}); err != nil {
		warn(err)
		return 0
	}
	result.Milestone = is.Milestone
	return is.Milestone
}

func prefixed(prefix string, items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = prefix + s
	}
	return out
}

// pendingLease returns the lease of a rebase an earlier publish made and
// could not push (a red gate, a refusal): the remote branch is no longer an
// ancestor of HEAD, and the operation log holds ship-it's own rebase that
// started exactly from it. Anything else — someone else's commits — gets no
// lease, so nothing is overwritten.
func pendingLease(ctx context.Context, ws Workspace, remote string, branch ports.BranchState) string {
	remoteTip, err := ws.Git.CommitOf(ctx, "refs/remotes/"+remote+"/"+branch.Name)
	if err != nil || remoteTip == "" || remoteTip == branch.Commit {
		return ""
	}
	if fastForward, err := ws.Git.IsAncestor(ctx, remoteTip, branch.Commit); err != nil || fastForward {
		return ""
	}
	if rebasedByShip(ctx, ws, branch.Name, remoteTip) {
		return remoteTip
	}
	return ""
}

// rebasedByShip reports whether the operation log holds a rebase of branch
// by ship-it that started from remoteTip: the remote branch then holds our
// own commits before the rebase, and a lease on it overwrites nobody's work.
func rebasedByShip(ctx context.Context, ws Workspace, branch, remoteTip string) bool {
	if ws.Operations == nil {
		return false
	}
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return false
	}
	for _, op := range ops {
		if op.Kind == oplog.Rebase && op.Before.Branch == branch && op.Before.Head == remoteTip {
			return true
		}
	}
	return false
}

// baseUpdate is how updateOnBase brings a branch up to date
// (ship.publish.update) and how the commits it makes are signed.
type baseUpdate struct {
	mode string // config.UpdateRebase, UpdateMerge or UpdateNone
	opts ports.CommitOptions
}

// updateOnBase brings a branch that is behind base up to date: by default
// it rebases it (decision of 2026-10-23: linear history), with
// ship.publish.update merge it merges the base into it, with none it leaves
// it. It returns the lease for the push: the remote tip, when the branch was
// already pushed and is now rewritten.
//
// It does nothing when someone else pushed commits the local branch lacks
// (their work must come first), and on conflict it leaves the branch as it
// was: the pull request will show the conflict (docs/reference/error-playbook.md, "Rebase conflict").
func updateOnBase(ctx context.Context, ws Workspace, remote string, branch ports.BranchState, base string, update baseUpdate, result *PublishResult) (string, error) {
	if base == "" {
		return "", nil
	}
	if update.mode == config.UpdateNone {
		return pendingLease(ctx, ws, remote, branch), nil
	}
	_, behind, err := ws.Git.CompareWith(ctx, base)
	if err != nil {
		return "", err
	}
	if behind == 0 {
		return pendingLease(ctx, ws, remote, branch), nil
	}
	remoteTip, err := ws.Git.CommitOf(ctx, "refs/remotes/"+remote+"/"+branch.Name)
	if err != nil {
		return "", err
	}
	if remoteTip != "" && remoteTip != branch.Commit {
		mine, err := ws.Git.IsAncestor(ctx, remoteTip, branch.Commit)
		if err != nil {
			return "", err
		}
		// Diverged: someone else's commits, unless the divergence is
		// ship-it's own earlier rebase that never got pushed.
		if !mine && !rebasedByShip(ctx, ws, branch.Name, remoteTip) {
			result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.RemoteAhead), Key: "publish.remote_ahead",
				Params: map[string]string{"branch": branch.Name}})
			return "", nil
		}
	}
	if update.mode == config.UpdateMerge {
		return pendingLease(ctx, ws, remote, branch), mergeBase(ctx, ws, branch, base, update.opts, result)
	}
	if _, err := ws.Checkpoints.Create(ctx, "before-rebase"); err != nil {
		return "", err
	}
	rebased := ws.begin(ctx, "publish/rebase")
	conflicted, err := ws.Git.RebaseWith(ctx, base, update.opts)
	rebased(err, slog.Bool("conflict", conflicted))
	if err != nil {
		return "", err
	}
	if conflicted {
		result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.RebaseConflict), Key: "publish.rebase_conflict",
			Params: map[string]string{"base": base, "branch": branch.Name}})
		return "", nil
	}
	result.Rebased = true
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return "", err
	}
	after := status.Branch.Commit
	// Logged so that ship log shows it; the previous commit stays reachable
	// through the reflog and the checkpoint.
	if _, err := ws.Operations.Append(ctx, oplog.Operation{
		Kind:   oplog.Rebase,
		Before: repositoryViewOf(branch), After: oplog.RepositoryView{Branch: branch.Name, Head: after},
		Title: base,
	}); err != nil {
		return "", err
	}
	return remoteTip, nil
}

// mergeBase merges base into the branch (ship.publish.update merge): no
// commit is rewritten, so the push needs no lease. A conflict is handled as
// a rebase conflict is: cancelled, the branch published as it is.
func mergeBase(ctx context.Context, ws Workspace, branch ports.BranchState, base string, opts ports.CommitOptions, result *PublishResult) error {
	if _, err := ws.Checkpoints.Create(ctx, "before-merge"); err != nil {
		return err
	}
	merged := ws.begin(ctx, "publish/merge")
	conflicted, err := ws.Git.Merge(ctx, base, opts)
	merged(err, slog.Bool("conflict", conflicted))
	if err != nil {
		return err
	}
	if conflicted {
		result.Warnings = append(result.Warnings, ProblemView{Code: string(fault.MergeConflict), Key: "publish.merge_conflict",
			Params: map[string]string{"base": base, "branch": branch.Name}})
		return nil
	}
	result.Merged = true
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return err
	}
	_, err = ws.Operations.Append(ctx, oplog.Operation{
		Kind:   oplog.Merge,
		Before: repositoryViewOf(branch), After: oplog.RepositoryView{Branch: branch.Name, Head: status.Branch.Commit},
		Title: base,
	})
	return err
}

// measure returns the number of changed lines, generated files (lockfiles,
// linguist-generated) excluded as docs/design/git-github-standards.md requires for size labels.
func measure(ctx context.Context, ws Workspace, stats []ports.FileStat) (changedLines int) {
	files := make([]string, len(stats))
	for i, s := range stats {
		files[i] = s.Path
	}
	generated, err := ws.Git.GeneratedPaths(ctx, files)
	if err != nil {
		generated = map[string]bool{}
	}
	for _, s := range stats {
		if !generated[s.Path] && !snapshot.IsLockfile(s.Path) {
			changedLines += s.Added + s.Deleted
		}
	}
	return changedLines
}

// Publish pushes the current branch to the remote (ship.git.remote) and
// opens its pull request against the default branch (or
// ship.pullRequest.base), on the parent repository for a fork, or updates
// the one already open.
func Publish(ctx context.Context, ws Workspace, req PublishRequest) (PublishResult, error) {
	done := ws.begin(ctx, "publish")
	mark := ws.planMark()
	result, err := publish(ctx, ws, req)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, operation(result.Operation)...)
	return result, err
}

func publish(ctx context.Context, ws Workspace, req PublishRequest) (PublishResult, error) {
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return PublishResult{}, err
	}
	branch := status.Branch
	if branch.Commit == "" {
		return PublishResult{}, fault.New(fault.NothingToPublish, "publish.nothing")
	}
	if branch.Detached || isTrunk(branch.Name, req.Reads.defaultBranch(ctx, ws)) {
		return PublishResult{}, fault.New(fault.PublishFromTrunk, "publish.trunk", "branch", branch.Name)
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return PublishResult{}, err
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return PublishResult{}, err
	}
	prSettings := eff.Settings.Ship.PullRequest
	remote := remoteOf(eff)
	result := PublishResult{Repository: repo, Branch: branch.Name}

	// Asking GitHub first proves the token works before anything is pushed.
	info, err := ws.Forge.Repository(ctx, repo)
	if err != nil {
		return result, forgeFault(err, repo)
	}
	// Where the pull request opens: the parent of a fork, on its default
	// branch or ship.pullRequest.base.
	target, err := resolvePublication(ctx, ws, eff, repo, info)
	if err != nil {
		return result, err
	}
	result.Base = target.base
	if target.fork() {
		result.PullRequestRepository = &target.prRepo
	}
	if branch.Name == info.DefaultBranch || branch.Name == target.base {
		// The repository's real default branch, whatever its name ("dev",
		// "production"), or the base of its pull requests: never pushed to
		// directly.
		return result, fault.New(fault.PublishFromTrunk, "publish.trunk", "branch", branch.Name)
	}
	// A layer of a stack targets the layer below it.
	layer, err := stackLayerOf(ctx, ws, repo, branch.Name)
	if err != nil {
		return result, err
	}
	if layer != nil {
		result.Base, target.base = layer.parent, layer.parent
	}

	// Fresh remote branches: the base to rebase on, the lease to push with.
	base, err := fetchBase(ctx, ws, target)
	if err != nil {
		return result, err
	}
	commitOpts, err := commitOptions(ctx, ws, eff)
	if err != nil {
		return result, err
	}
	update := baseUpdate{mode: eff.Settings.Ship.Publish.Update, opts: commitOpts}
	lease, err := updateOnBase(ctx, ws, remote, branch, base, update, &result)
	if err != nil {
		return result, err
	}
	if result.Rebased || result.Merged {
		if status, err = ws.Git.Status(ctx); err != nil {
			return result, err
		}
		branch = status.Branch
	}
	messages, err := ws.Git.CommitMessagesSince(ctx, base)
	if err != nil {
		return result, err
	}
	if len(messages) == 0 {
		return result, fault.New(fault.NothingToPublish, "publish.nothing")
	}
	commits, headers := branchCommits(eff.Settings.Conventions(), messages)
	stats, err := ws.Git.DiffStatSince(ctx, base)
	if err != nil {
		return result, err
	}
	changedLines := measure(ctx, ws, stats)
	// Now that the files are known, per-path overrides apply (a docs-only
	// branch may skip the gates, for example).
	statPaths := make([]string, len(stats))
	for i, s := range stats {
		statPaths[i] = s.Path
	}
	if eff, err = LoadSettingsFor(ctx, ws, statPaths); err != nil {
		return result, err
	}
	prSettings = eff.Settings.Ship.PullRequest
	if !target.closesIssues() {
		// GitHub reads closing keywords only in pull requests to the default
		// branch: elsewhere the issue is linked, not closed, and not moved.
		eff.Settings.Ship.Issues.Keyword = string(convention.ReferencesIssue)
	}

	// What the push sends is scanned too: a commit made outside ship-it
	// (git commit, an IDE) never went through the delivery's scan.
	// The commits the push sends: those not on the remote branch yet nor on
	// the base (after a rebase, the base's new commits are not ours).
	var already []string
	if exists, err := ws.Git.RemoteBranchExists(ctx, remote, branch.Name); err == nil && exists {
		already = append(already, remote+"/"+branch.Name)
	}
	if base != "" {
		already = append(already, base)
	}
	if found, err := scanPushed(ctx, ws, already); err != nil {
		return result, err
	} else if len(found) > 0 {
		// The advice rewrites unpushed commits only: from the remote branch
		// when it exists, else from the base (--root on a first publication).
		since := ""
		if len(already) > 0 {
			since = already[0]
		}
		return result, &SecretsFoundError{Findings: found, Unpushed: true, Base: since}
	}
	// The repository's rulesets are checked before the gates: a refusal
	// costs nothing, and the push would fail anyway.
	rename, err := checkRulesets(ctx, ws, repo, branch, base)
	if err != nil {
		return result, err
	}
	deliveredAs := branch.Name

	// Gates run before the push only: the local commit stays instant and
	// undoable, while red work never reaches GitHub (decision of 2026-10-23).
	gatesOn := eff.Settings.Ship.Gates.Enabled && ws.Runner != nil
	if gatesOn && req.SkipGates && !eff.Settings.Ship.Gates.AllowSkip {
		return result, fault.New(fault.GateSkipRefused, "publish.skip_refused", "source", eff.Source("ship.gates.allowSkip"))
	}
	if gatesOn && !req.SkipGates && len(req.GatesPassed) > 0 && !result.Rebased && !result.Merged {
		result.Gates = req.GatesPassed
	} else if gatesOn && !req.SkipGates {
		gateResults, err := RunGates(ctx, ws, base, req.GateObserver)
		result.Gates = gateResults
		if err != nil {
			return result, err
		}
	}

	if rename != nil {
		if branch, err = applyRename(ctx, ws, branch, rename, &result); err != nil {
			return result, err
		}
	}

	pushed := ws.begin(ctx, "publish/push")
	err = ws.Git.Push(ctx, remote, branch.Name, lease)
	pushed(err, slog.Bool("lease", lease != ""))
	if err != nil {
		return result, fault.New(fault.PushRejected, "publish.push_rejected", "branch", branch.Name, "detail", lastLine(err.Error()))
	}

	// From here on, the pull request's repository: the parent of a fork.
	repo = target.prRepo
	pr, found, err := ws.Forge.FindOpenPullRequest(ctx, repo, target.head(branch.Name))
	if err != nil {
		return result, forgeFault(err, repo)
	}
	var title string
	var linked *oplog.LinkedIssueChange
	var milestone int
	if found && prSettings.Labels {
		refreshSize(ctx, ws, repo, pr, eff.Settings, headers, changedLines, &result)
	}
	if !found {
		issue := req.Issue
		if issue == 0 {
			issue = convention.IssueFromBranch(branch.Name)
		}
		ticket := convention.FindTicket(branch.Name, eff.Settings.Ship.Issues.TrackerKeys)
		titled := make([]convention.Header, len(headers))
		for i, h := range headers {
			titled[i] = convention.WithTicket(h, ticket) // commits made outside ship-it may lack it
		}
		title = pullrequest.TitleWith(eff.Settings.Conventions(), titled, commitPlacement(ctx, ws, eff))
		if title == "" {
			title, _ = pullrequest.ParseMessage(messages[len(messages)-1])
		}
		in := pullrequest.BodyInput{
			Issue: issue, Keyword: convention.IssueKeyword(eff.Settings.Ship.Issues.Keyword), Commits: commits,
			Checks: passedChecks(result.Gates), ChecksSkipped: gatesOn && req.SkipGates,
			AIAssistant: aiAssistantOf(ctx, ws, deliveredAs),
			Omit:        pullrequest.Omit{Why: !prSettings.Sections.Why, Testing: !prSettings.Sections.Testing, Issue: !prSettings.Sections.Issue},
		}
		if prSettings.Sections.Changes {
			// A summary is a help, never a reason to stop the publication.
			if diff, err := ws.Git.DiffSince(ctx, base); err == nil {
				in.Changes = pullrequest.ChangesOf(snapshot.ParseUnifiedDiff(diff))
			}
		}
		if issue > 0 {
			// Context only: a missing or private issue changes nothing.
			in.IssueTitle, _ = ws.Forge.IssueTitle(ctx, repo, issue)
		}
		if in.AIAssistant != "" {
			// The repository's own AI label, when it has one, discloses the
			// assistance its way (decision of 2026-10-23).
			if existing, err := ws.Forge.Labels(ctx, repo); err == nil {
				in.AILabel = pullrequest.AILabel(labelNames(existing))
			}
		}
		templateRev := base
		if templateRev == "" {
			templateRev = "HEAD" // first publication: no default branch yet
		}
		opened := ws.begin(ctx, "publish/pull_request")
		pr, err = ws.Forge.CreatePullRequest(ctx, repo, ports.PullRequestSpec{
			Title: title,
			Body:  pullRequestBody(in, pullRequestTemplate(ctx, ws, templateRev, prSettings.Template)) + layer.chainNote(ctx, ws, repo),
			Head:  target.head(branch.Name),
			Base:  target.base,
			Draft: draftOf(ws, req, prSettings.Draft, headers),
		})
		opened(err, slog.Bool("draft", pr.Draft))
		if err != nil {
			return result, forgeFault(err, repo)
		}
		result.Created = true
		layer.join(ctx, ws, repo, pr.Number, &result)
		if prSettings.Labels {
			labelPullRequest(ctx, ws, repo, pr.Number, eff.Settings, headers, changedLines, &result)
		}
		if in.AILabel != "" {
			// The body counts on this label to disclose the AI assistance.
			if err := ws.Forge.AddLabels(ctx, repo, pr.Number, []string{in.AILabel}); err != nil {
				result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
			} else {
				result.Labels = append(result.Labels, in.AILabel)
			}
		}
		milestone = requestReviewsAndMilestone(ctx, ws, repo, prSettings, issue, pr, &result)
		linked = moveLinkedIssue(ctx, ws, repo, eff, issue, pr, &result)
	}
	result.PullRequest = pr
	if title == "" {
		title = pr.URL // updated pull request: its URL names it best
	}

	view := repositoryViewOf(branch)
	op, err := ws.Operations.Append(ctx, oplog.Operation{
		Kind: oplog.Publish, Before: view, After: view, Commit: branch.Commit, Title: title,
		Remote: remote, PullRequest: pr.Number, PullRequestURL: pr.URL, PullRequestCreated: result.Created,
		LinkedIssue: linked, PullRequestMilestone: milestone, PullRequestRepository: target.pullRequestSlug(),
	})
	result.Operation = op.ID
	return result, err
}

// draftOf decides whether a new pull request is a draft: --draft, an AI
// agent (whatever the settings: a human marks it ready), work in progress
// commits, then ship.pullRequest.draft.
func draftOf(ws Workspace, req PublishRequest, setting string, headers []convention.Header) bool {
	if req.ForceDraft || ws.Agent {
		return true
	}
	var wanted bool
	switch setting {
	case config.DraftAlways:
		wanted = true
	case config.DraftNever:
	default:
		wanted = req.Unattended
	}
	return pullrequest.Draft(wanted, headers)
}

// labelPullRequest adds the labels the repository has. The pull request
// already exists, so a problem here is only a warning.
func labelPullRequest(ctx context.Context, ws Workspace, repo forge.Repository, number int,
	s config.Settings, headers []convention.Header, changedLines int, result *PublishResult) {
	existing, err := ws.Forge.Labels(ctx, repo)
	if err != nil {
		result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
		return
	}
	labels := s.PullRequestLabels().Labels(s.Conventions(), headers, changedLines, labelNames(existing))
	if len(labels) == 0 {
		return
	}
	if err := ws.Forge.AddLabels(ctx, repo, number, labels); err != nil {
		result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
		return
	}
	result.Labels = labels
}

// refreshSize replaces the size/ label of an updated pull request. Like
// labelPullRequest, a problem is only a warning.
func refreshSize(ctx context.Context, ws Workspace, repo forge.Repository, pr ports.PullRequest,
	s config.Settings, headers []convention.Header, changedLines int, result *PublishResult) {
	existing, err := ws.Forge.Labels(ctx, repo)
	if err != nil {
		result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
		return
	}
	tax := s.PullRequestLabels()
	add, remove := tax.RefreshLabels(pr.Labels, tax.Labels(s.Conventions(), headers, changedLines, labelNames(existing)))
	for _, label := range remove {
		if err := ws.Forge.RemoveLabel(ctx, repo, pr.Number, label); err != nil {
			result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
			return
		}
	}
	if len(add) > 0 {
		if err := ws.Forge.AddLabels(ctx, repo, pr.Number, add); err != nil {
			result.Warnings = append(result.Warnings, *problemOf(forgeFault(err, repo)))
			return
		}
		result.Labels = add
	}
}

// aiAssistantOf names the AI that wrote a plan delivered on branch, if any:
// the pull request footer then says "AI-assisted (…)" (docs/reference/conventions.md).
func aiAssistantOf(ctx context.Context, ws Workspace, branch string) string {
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return ""
	}
	for _, op := range oplog.Undoable(ops) {
		if op.Kind == oplog.Deliver && op.After.Branch == branch && op.AIAssistant != "" {
			return op.AIAssistant
		}
	}
	return ""
}

func labelNames(labels []settings.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}

// forgeFault turns the ports errors into faults with a remedy.
func forgeFault(err error, repo forge.Repository) error {
	switch {
	case errors.Is(err, ports.ErrNotAuthenticated):
		return fault.New(fault.NotAuthenticated, "publish.not_authenticated", "host", repo.Host)
	case errors.Is(err, ports.ErrTokenRejected):
		return fault.New(fault.TokenRejected, "publish.token_rejected", "host", repo.Host)
	case errors.Is(err, ports.ErrNotFound):
		return fault.New(fault.GitHubRepositoryNotFound, "publish.repository_not_found", "repository", repo.Slug())
	case errors.Is(err, ports.ErrRateLimited):
		return fault.New(fault.RateLimited, "publish.rate_limited")
	}
	return err
}

// lastLine returns the line of a git error that says what went wrong: the
// first "fatal:" or "error:" line (git puts the cause there and adds advice
// after it), else the last line.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, line := range lines {
		for _, marker := range []string{"fatal:", "error:"} {
			if i := strings.Index(line, marker); i >= 0 {
				return strings.TrimSpace(line[i:])
			}
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}
