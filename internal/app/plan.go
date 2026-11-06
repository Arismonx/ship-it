// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/commitlint"
	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/plan"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/core/split"
	"github.com/GregoireF/ship-it/internal/ports"
)

// untrackedReadLimit bounds how much of an untracked file is read to plan.
// Larger files are planned as binary additions (name only).
const untrackedReadLimit = 1 << 20

// PlanRequest carries what the snapshot cannot know.
type PlanRequest struct {
	Issue int
	Agent string
	// Only selects part of the work: files, or directories with all under
	// them (`ship --only src/api/`). Empty means everything.
	Only []string
}

// SizeView warns that the work is larger than a pull request reviews well,
// and suggests parts that can ship on their own.
type SizeView struct {
	Lines  int           `json:"lines"`
	Max    int           `json:"max"`
	Groups []split.Group `json:"groups,omitempty"`
}

// PlanView is the plan as surfaces render it.
type PlanView struct {
	Title  string `json:"title"`
	Branch string `json:"branch"`
	// CurrentBranch is where the work sits (empty when detached), and
	// CreatesBranch whether the delivery moves it to Branch first.
	CurrentBranch string       `json:"current_branch,omitempty"`
	CreatesBranch bool         `json:"creates_branch"`
	Files         []string     `json:"files"`
	Reasons       []ReasonView `json:"reasons"`
	// Secrets found in the work: the delivery will refuse until they are gone.
	Secrets []SecretView `json:"secrets,omitempty"`
	// TypeChoices are the commit types the project's commitlint allows when
	// it refuses the planned one and several could fit: the human picks one
	// at review; without a review (--yes, an agent) the delivery refuses.
	TypeChoices []string `json:"type_choices,omitempty"`
	// Size is set when the work exceeds ship.pullRequest.maxLines. It never
	// blocks (decision of 2026-10-23).
	Size *SizeView `json:"size,omitempty"`

	reads *repositoryReads // see Reads
}

// ReasonView is one explained decision of the planner.
type ReasonView struct {
	Key    string            `json:"key"`
	Params map[string]string `json:"params,omitempty"`
	// Text is the reason rendered by a surface for a client that has no
	// catalogue (the VS Code extension); empty from the app layer.
	Text string `json:"text,omitempty"`
}

// ProposePlan reads the work in progress and returns the deterministic plan,
// with the secrets it contains. It changes nothing in the repository.
func ProposePlan(ctx context.Context, ws Workspace, req PlanRequest) (PlanView, error) {
	reads, err := readStatus(ctx, ws.Git)
	if err != nil {
		return PlanView{}, err
	}
	status := reads.status
	diffs, err := reads.selectedWork(ctx, ws.Git, req.Only)
	if err != nil {
		return PlanView{}, err
	}
	findings, err := scanSecrets(ctx, ws, diffs)
	if err != nil {
		return PlanView{}, err
	}
	paths := diffPaths(diffs)
	eff, err := LoadSettingsFor(ctx, ws, paths)
	if err != nil {
		return PlanView{}, err
	}
	// A failed lookup only loses the hint: lockfiles are still recognised.
	generated, _ := ws.Git.GeneratedPaths(ctx, paths)
	snap := snapshot.New(diffs, snapshot.Options{GeneratedPaths: generated})
	rules := eff.Settings.Conventions()
	p, err := plan.Propose(snap, plan.Options{
		Issue: req.Issue, Agent: convention.AgentPrefix(req.Agent),
		Rules: rules, NoScope: eff.Settings.Ship.Commits.ScopeFrom == config.ScopeFromNone,
	})
	if err != nil {
		return PlanView{}, err
	}
	placement := placementWith(ctx, ws, eff, reads) // reads the history once
	view := PlanView{
		Title:   rules.Format(p.Header, placement),
		Branch:  p.Branch,
		Files:   p.Files,
		Secrets: findings,
		reads:   reads,
	}
	for _, r := range p.Reasons {
		view.Reasons = append(view.Reasons, ReasonView{Key: r.Key, Params: r.Params})
	}
	view.Size = sizeOf(snap, eff.Settings.Ship.PullRequest.MaxLines)
	view.CurrentBranch, view.CreatesBranch = status.Branch.Name, createsBranch(status.Branch, reads.defaultBranch(ctx, ws))
	if !view.CreatesBranch && onStackTop(ctx, ws, status.Branch.Name) {
		// A new layer on top of the stack (decision of 2026-11-02).
		view.CreatesBranch = true
		view.Reasons = append(view.Reasons, ReasonView{Key: "plan.reason.stack_layer", Params: map[string]string{"parent": status.Branch.Name}})
	}
	if ticket := convention.FindTicket(status.Branch.Name, eff.Settings.Ship.Issues.TrackerKeys); ticket != "" && !view.CreatesBranch {
		// The branch names a tracker ticket: the title carries it, where
		// Conventional Commits stay valid (decision of 2026-10-24).
		view.Title = rules.Format(convention.WithTicket(p.Header, ticket), placement)
		view.Reasons = append(view.Reasons, ReasonView{Key: "plan.reason.ticket", Params: map[string]string{"ticket": ticket}})
	}
	if status.Branch.Commit == "" && len(req.Only) == 0 {
		// A repository's first commit is its initial commit, whatever it
		// holds; it lands on the current branch (see createsBranch).
		initial := initialCommit
		if t, replaced := rules.Closest(initial.Type); replaced {
			initial.Type = t // the repository removed chore
		}
		view.Title, view.Branch = rules.Format(initial, placement), status.Branch.Name
		view.Reasons = []ReasonView{{Key: "plan.reason.initial_commit"}}
	}
	return planWithCommitlint(ctx, ws, rules, placement, view)
}

// planWithCommitlint gives the project's commitlint, when installed, the last
// word on the title, gitmoji placement included; the reasons say so. When it
// refuses the planned type and several could fit, the title stays and the
// plan offers the allowed types for the human to pick (rule zero).
func planWithCommitlint(ctx context.Context, ws Workspace, rules convention.Rules, placement convention.GitmojiPlacement, view PlanView) (PlanView, error) {
	cfg := projectCommitlint(ctx, ws)
	if cfg == nil {
		return view, nil
	}
	header, err := rules.ParseHeader(view.Title)
	if err != nil {
		return view, nil
	}
	if choices := commitlint.TypeChoices(header.Type, *cfg, rules); len(choices) > 0 {
		for _, c := range choices {
			view.TypeChoices = append(view.TypeChoices, string(c))
		}
		view.Reasons = append(view.Reasons, ReasonView{Key: "plan.reason.commitlint.type_choice", Params: map[string]string{
			"type": string(header.Type), "choices": strings.Join(view.TypeChoices, ", "),
		}})
		return view, nil
	}
	title, reasons, err := followCommitlint(ctx, ws, cfg, rules, header, placement, nil)
	if err != nil {
		return PlanView{}, err
	}
	view.Title, view.Reasons = title, append(view.Reasons, reasons...)
	return view, nil
}

// initialCommit uses 🎉, the gitmoji of "Begin a project".
var initialCommit = convention.Header{Gitmoji: "🎉", Type: convention.Maintenance, Subject: "initial commit"}

// sizeOf counts the changed lines that a reviewer reads (generated files
// excluded) and suggests a split above maxLines.
func sizeOf(s snapshot.Snapshot, maxLines int) *SizeView {
	var files []split.File
	total := 0
	for _, f := range s.Files {
		if f.Generated {
			continue
		}
		lines := len(f.Added) + len(f.Removed)
		files = append(files, split.File{Path: f.Path, Lines: lines})
		total += lines
	}
	if total <= maxLines {
		return nil
	}
	return &SizeView{Lines: total, Max: maxLines, Groups: split.Suggest(files, maxLines)}
}

// selectedWorkIn is the work in progress, restricted to the --only
// selection, for a status the caller already read: each read is a git
// process (about 20 ms on Windows, more on a large tree).
func selectedWorkIn(ctx context.Context, git ports.Git, status ports.WorkingTreeStatus, only []string) ([]snapshot.FileDiff, error) {
	return (&repositoryReads{status: status}).selectedWork(ctx, git, only)
}

// workInProgress returns the changes against HEAD, untracked files included;
// NothingToShip when there are none.
func workInProgress(ctx context.Context, git ports.Git, status ports.WorkingTreeStatus) ([]snapshot.FileDiff, error) {
	var diffs []snapshot.FileDiff
	if status.Branch.Commit != "" {
		raw, err := git.DiffFromHead(ctx)
		if err != nil {
			return nil, err
		}
		diffs = snapshot.ParseUnifiedDiff(raw)
	}
	for _, f := range status.Files {
		// Before the first commit nothing is in HEAD: every file is an addition.
		if f.State == ports.Untracked || (status.Branch.Commit == "" && f.State == ports.Tracked) {
			diffs = append(diffs, newFileDiff(ctx, git, f.Path))
		}
	}
	if len(diffs) == 0 {
		return nil, fault.New(fault.NothingToShip, "plan.nothing_to_ship")
	}
	return diffs, nil
}

// newFileDiff describes an untracked file as an addition of all its lines.
func newFileDiff(ctx context.Context, git ports.Git, path string) snapshot.FileDiff {
	d := snapshot.FileDiff{Path: path, Status: snapshot.Added}
	content, err := git.ReadWorkingFile(ctx, path, untrackedReadLimit)
	switch {
	case errors.Is(err, ports.ErrFileTooLarge), err == nil && bytes.IndexByte(content, 0) >= 0:
		d.Binary = true
		return d
	case err != nil:
		return d
	}
	text := strings.TrimSuffix(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if text == "" {
		return d
	}
	for i, line := range strings.Split(text, "\n") {
		d.Added = append(d.Added, snapshot.Line{Number: i + 1, Text: line})
	}
	return d
}
