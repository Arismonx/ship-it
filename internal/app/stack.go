// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/core/plan"
	"github.com/GregoireF/ship-it/internal/core/pullrequest"
	"github.com/GregoireF/ship-it/internal/core/snapshot"
	"github.com/GregoireF/ship-it/internal/core/split"
	"github.com/GregoireF/ship-it/internal/core/stack"
	"github.com/GregoireF/ship-it/internal/ports"
)

// StackPlanRequest asks for the layers of a stack.
type StackPlanRequest struct {
	// Layers are the human's own layers, bottom first: each one files or
	// directories, as --only takes them. Empty: by area (ship.stack.maxLayers).
	Layers [][]string
	Agent  string
}

// StackLayerPlan is one layer as the plan proposes it.
type StackLayerPlan struct {
	Title  string   `json:"title"`
	Branch string   `json:"branch"`
	Area   string   `json:"area"`
	Lines  int      `json:"lines"`
	Files  []string `json:"files"`
}

// StackPlanView is the plan of a stack, reviewed like the plan of a delivery.
type StackPlanView struct {
	// Base is the branch the bottom layer targets: the default branch.
	Base   string           `json:"base"`
	Layers []StackLayerPlan `json:"layers"`
	// Left are the changed files no layer holds: they stay uncommitted.
	Left    []string     `json:"left,omitempty"`
	Secrets []SecretView `json:"secrets,omitempty"`
	// MaxLayers is ship.stack.maxLayers.
	MaxLayers int `json:"max_layers"`
}

// ProposeStack splits the work in progress into layers, each with its
// title and branch from the deterministic planner. It changes nothing.
func ProposeStack(ctx context.Context, ws Workspace, req StackPlanRequest) (StackPlanView, error) {
	reads, err := readStatus(ctx, ws.Git)
	if err != nil {
		return StackPlanView{}, err
	}
	trunk := reads.defaultBranch(ctx, ws)
	if !createsBranch(reads.status.Branch, trunk) {
		return StackPlanView{}, fault.New(fault.StackNotFromTrunk, "stack.not_from_trunk", "branch", reads.status.Branch.Name)
	}
	diffs, err := reads.selectedWork(ctx, ws.Git, nil)
	if err != nil {
		return StackPlanView{}, err
	}
	findings, err := scanSecrets(ctx, ws, diffs)
	if err != nil {
		return StackPlanView{}, err
	}
	paths := diffPaths(diffs)
	eff, err := LoadSettingsFor(ctx, ws, paths)
	if err != nil {
		return StackPlanView{}, err
	}
	generated, _ := ws.Git.GeneratedPaths(ctx, paths)
	snap := snapshot.New(diffs, snapshot.Options{GeneratedPaths: generated})
	files := make([]split.File, 0, len(snap.Files))
	for _, f := range snap.Files {
		lines := len(f.Added) + len(f.Removed)
		if f.Generated {
			lines = 0 // never decides an area's rank
		}
		files = append(files, split.File{Path: f.Path, Lines: lines})
	}
	maxLayers := eff.Settings.Ship.Stack.MaxLayers
	var layers []stack.Layer
	if len(req.Layers) > 0 {
		layers, err = stack.Choose(files, req.Layers)
	} else {
		layers, err = stack.Layers(files, maxLayers)
	}
	if err != nil {
		return StackPlanView{}, err
	}
	if trunk == "" {
		trunk = reads.status.Branch.Name
	}
	view := StackPlanView{Base: trunk, Secrets: findings, MaxLayers: maxLayers}
	placement := placementWith(ctx, ws, eff, reads)
	held := map[string]bool{}
	taken := map[string]bool{}
	for _, l := range layers {
		var part []snapshot.FileDiff
		for _, d := range diffs {
			if split.Matches(d.Path, l.Files) {
				part = append(part, d)
			}
		}
		p, err := plan.Propose(snapshot.New(part, snapshot.Options{GeneratedPaths: generated}), plan.Options{Agent: convention.AgentPrefix(req.Agent)})
		if err != nil {
			return StackPlanView{}, err
		}
		branch := convention.UniqueBranchName(p.Branch, func(name string) bool {
			exists, err := ws.Git.LocalBranchExists(ctx, name)
			return taken[name] || (err == nil && exists)
		})
		taken[branch] = true
		for _, f := range l.Files {
			held[f] = true
		}
		view.Layers = append(view.Layers, StackLayerPlan{
			Title: convention.Format(p.Header, placement), Branch: branch, Area: l.Area, Lines: l.Lines, Files: l.Files,
		})
	}
	for _, p := range paths {
		if !held[p] {
			view.Left = append(view.Left, p)
		}
	}
	return view, nil
}

// StackBranchTaken is the fault of a layer named like another layer of the
// same stack: each layer is a branch of its own.
func StackBranchTaken(branch string) *Fault {
	return fault.New(fault.InvalidBranch, "stack.duplicate_branch", "branch", branch)
}

// StackLayerSpec is one accepted layer: its title, branch and files.
type StackLayerSpec struct {
	Title  string   `json:"title"`
	Branch string   `json:"branch"`
	Files  []string `json:"files"`
}

// StackDeliveryRequest is an accepted stack plan, possibly edited.
type StackDeliveryRequest struct {
	Layers []StackLayerSpec
	// AIAssistant names who wrote the titles when not the planner.
	AIAssistant string
}

// StackLayerResult is one layer that was committed.
type StackLayerResult struct {
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Title  string `json:"title"`
}

// StackDeliveryResult reports the branches and commits of a new stack.
type StackDeliveryResult struct {
	Operation  string             `json:"operation,omitempty"`
	Checkpoint CheckpointView     `json:"checkpoint"`
	Base       string             `json:"base"`
	Layers     []StackLayerResult `json:"layers"`
	Rehearsal
}

// DeliverStack records an accepted stack: a checkpoint, then for each layer
// from the bottom up a branch based on the previous one and one commit of
// its files. Whatever it made, even when interrupted, is logged so that
// `ship undo` puts the work back uncommitted.
func DeliverStack(ctx context.Context, ws Workspace, req StackDeliveryRequest) (result StackDeliveryResult, err error) {
	done := ws.begin(ctx, "stack")
	mark := ws.planMark()
	defer func() {
		if ws.IsDryRun() {
			result.Rehearsal = ws.rehearsed(mark)
		}
		done(err, append(operation(result.Operation), slog.Int("layers", len(result.Layers)))...)
	}()
	if len(req.Layers) < stack.MinLayers {
		return StackDeliveryResult{}, fault.New(fault.StackNeedsLayers, "stack.too_few_layers", "count", strconv.Itoa(len(req.Layers)))
	}
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return StackDeliveryResult{}, err
	}
	if report := doctorWith(ctx, ws.Git, ws.OperatingSystem, gitRemote(ctx, ws), &status); report.Blocking() {
		return StackDeliveryResult{}, fault.New(fault.DoctorBlocking, "deliver.doctor_blocking")
	}
	trunk := defaultBranchOf(ctx, ws)
	if !createsBranch(status.Branch, trunk) {
		return StackDeliveryResult{}, fault.New(fault.StackNotFromTrunk, "stack.not_from_trunk", "branch", status.Branch.Name)
	}
	var all []string
	for _, l := range req.Layers {
		if len(l.Files) == 0 {
			return StackDeliveryResult{}, fault.New(fault.StackNeedsLayers, "stack.empty_layer", "layer", l.Branch)
		}
		all = append(all, l.Files...)
	}
	diffs, err := selectedWorkIn(ctx, ws.Git, status, all)
	if err != nil {
		return StackDeliveryResult{}, err
	}
	if found, err := scanSecrets(ctx, ws, diffs); err != nil {
		return StackDeliveryResult{}, err
	} else if len(found) > 0 {
		return StackDeliveryResult{}, &SecretsFoundError{Findings: found}
	}
	eff, err := LoadSettingsFor(ctx, ws, diffPaths(diffs))
	if err != nil {
		return StackDeliveryResult{}, err
	}
	keys := eff.Settings.Ship.Issues.TrackerKeys
	placement := commitPlacement(ctx, ws, eff)
	rules := eff.Settings.Conventions()
	titles := make([]string, len(req.Layers))
	named := map[string]bool{}
	for i, l := range req.Layers {
		check := checkTitleAs(rules, l.Title, placement, "")
		if !check.Valid() {
			return StackDeliveryResult{}, check.Faults[0]
		}
		if err := rules.ValidateBranch(l.Branch, keys); err != nil {
			return StackDeliveryResult{}, err
		}
		if named[l.Branch] {
			return StackDeliveryResult{}, StackBranchTaken(l.Branch)
		}
		named[l.Branch] = true
		titles[i] = check.Canonical
	}
	// Sign-off and signature, as for one commit: refused here without a key.
	commitOpts, err := commitOptions(ctx, ws, eff)
	if err != nil {
		return StackDeliveryResult{}, err
	}

	checkpoint, err := ws.Checkpoints.Create(ctx, "stack")
	if err != nil {
		return StackDeliveryResult{}, err
	}
	base := trunk
	if base == "" {
		base = status.Branch.Name
	}
	result = StackDeliveryResult{Checkpoint: viewOf(checkpoint), Base: base}
	before := repositoryViewOf(status.Branch)
	defer func() {
		if len(result.Layers) == 0 {
			return // nothing to undo
		}
		record := &oplog.StackRecord{Base: base}
		for _, l := range result.Layers {
			record.Layers = append(record.Layers, oplog.StackLayer{Branch: l.Branch, Commit: l.Commit, Title: l.Title})
		}
		top := result.Layers[len(result.Layers)-1]
		after := oplog.RepositoryView{Branch: top.Branch, Head: top.Commit}
		if top.Commit == "" {
			after.Head = before.Head // the last branch was made, not its commit
		}
		op, logErr := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.Stack, Checkpoint: checkpoint.ID, Before: before, After: after, CreatedBranch: true,
			Commit: top.Commit, Title: result.Layers[0].Title, AIAssistant: req.AIAssistant, Stack: record,
		})
		result.Operation = op.ID
		err = errors.Join(err, logErr)
	}()
	taken := map[string]bool{}
	for i, l := range req.Layers {
		branch := convention.UniqueBranchName(l.Branch, func(name string) bool {
			exists, err := ws.Git.LocalBranchExists(ctx, name)
			return taken[name] || (err == nil && exists)
		})
		taken[branch] = true
		if err := ws.Git.SwitchToNewBranch(ctx, branch); err != nil {
			return result, err
		}
		result.Layers = append(result.Layers, StackLayerResult{Branch: branch, Title: titles[i]})
		commit, err := ws.Git.CommitPaths(ctx, convention.CommitMessage(titles[i], "", ""), l.Files, commitOpts)
		if err != nil {
			return result, err
		}
		result.Layers[i].Commit = commit
	}
	return result, nil
}

// StackPublishRequest configures the publication of a stack.
type StackPublishRequest struct {
	// Issue links the pull requests; 0 reads it from the layer branch names.
	Issue int
	// Unattended and ForceDraft decide drafts as for a single pull request
	// (PublishRequest): every agent's pull request is a draft.
	Unattended   bool
	ForceDraft   bool
	SkipGates    bool
	GateObserver GateObserver
}

// StackLayerStatus is a layer and its pull request.
type StackLayerStatus struct {
	Branch      string            `json:"branch"`
	Base        string            `json:"base"`
	PullRequest ports.PullRequest `json:"pull_request"`
	// Created: the publication opened it; else it was open already.
	Created bool `json:"created"`
	// Labels were added to a new pull request, among the repository's own.
	Labels []string `json:"labels,omitempty"`
}

// StackPublishResult reports the pull requests of a published stack.
type StackPublishResult struct {
	Operation  string           `json:"operation,omitempty"`
	Repository forge.Repository `json:"repository"`
	Base       string           `json:"base"`
	// Native: a GitHub stack, Number; else a classic chain (GHES).
	Native   bool               `json:"native"`
	Number   int                `json:"number,omitempty"`
	Layers   []StackLayerStatus `json:"layers"`
	Gates    []GateResult       `json:"gates,omitempty"`
	Warnings []ProblemView      `json:"warnings,omitempty"`
	Rehearsal
}

// currentChain returns the stack the current branch belongs to.
func currentChain(ctx context.Context, ws Workspace, branch string) (stack.Chain, error) {
	ops, err := ws.Operations.Recent(ctx, operationSearchDepth)
	if err != nil {
		return stack.Chain{}, err
	}
	c, ok := stack.Find(stack.Chains(ops), branch)
	if !ok {
		return stack.Chain{}, fault.New(fault.NotInStack, "stack.not_in_stack", "branch", branch)
	}
	return c, nil
}

// existing keeps the layers whose local branch is still there: a sync
// deletes the merged ones.
func existing(ctx context.Context, ws Workspace, c stack.Chain) stack.Chain {
	return c.Without(func(b string) bool {
		ok, err := ws.Git.LocalBranchExists(ctx, b)
		return err == nil && !ok
	})
}

// stacksSupported asks whether the server has the stacks API.
func stacksSupported(ctx context.Context, ws Workspace, repo forge.Repository) (bool, error) {
	if ws.Stacks == nil {
		return false, nil
	}
	_, err := ws.Stacks.StacksOf(ctx, repo, 0)
	switch {
	case errors.Is(err, ports.ErrStacksUnavailable):
		return false, nil
	case err != nil:
		return false, forgeFault(err, repo)
	}
	return true, nil
}

// PublishStack pushes every layer, opens one pull request per layer, each
// based on the branch below it, and groups them in a native stack, or
// writes a classic chain where the server has no stacks (GHES). It runs on
// the top layer, which holds every layer's commits.
func PublishStack(ctx context.Context, ws Workspace, req StackPublishRequest) (StackPublishResult, error) {
	done := ws.begin(ctx, "stack/publish")
	mark := ws.planMark()
	result, err := publishStack(ctx, ws, req)
	result.Rehearsal = ws.rehearsed(mark)
	done(err, operation(result.Operation)...)
	return result, err
}

func publishStack(ctx context.Context, ws Workspace, req StackPublishRequest) (result StackPublishResult, err error) {
	status, err := ws.Git.Status(ctx)
	if err != nil {
		return result, err
	}
	c, err := currentChain(ctx, ws, status.Branch.Name)
	if err != nil {
		return result, err
	}
	c = existing(ctx, ws, c)
	if len(c.Branches) == 0 || c.Top() != status.Branch.Name {
		return result, fault.New(fault.StackNotOnTop, "stack.not_on_top", "branch", status.Branch.Name, "top", c.Top())
	}
	repo, err := remoteRepository(ctx, ws)
	if err != nil {
		return result, err
	}
	eff, err := LoadSettings(ctx, ws)
	if err != nil {
		return result, err
	}
	prSettings := eff.Settings.Ship.PullRequest
	result = StackPublishResult{Repository: repo, Base: c.Base}
	if _, err := ws.Forge.Repository(ctx, repo); err != nil {
		return result, forgeFault(err, repo)
	}
	fetched := ws.begin(ctx, "stack/fetch")
	err = ws.Git.Fetch(ctx, gitRemote(ctx, ws))
	fetched(err)
	if err != nil {
		return result, fault.New(fault.FetchFailed, "tidy.fetch_failed", "remote", gitRemote(ctx, ws), "detail", lastLine(err.Error()))
	}
	baseRef := ""
	if exists, err := ws.Git.RemoteBranchExists(ctx, gitRemote(ctx, ws), c.Base); err == nil && exists {
		baseRef = gitRemote(ctx, ws) + "/" + c.Base
	} else {
		return result, fault.New(fault.StackParentUnpublished, "stack.parent_unpublished", "branch", c.Branches[0], "parent", c.Base)
	}
	// What the pushes send, every layer at once from the top.
	var already []string
	if baseRef != "" {
		already = append(already, baseRef)
	}
	if found, err := scanPushed(ctx, ws, already); err != nil {
		return result, err
	} else if len(found) > 0 {
		return result, &SecretsFoundError{Findings: found, Unpushed: true, Base: baseRef}
	}
	// Every layer meets the repository's rulesets before anything is pushed:
	// one refused layer would leave half a stack on GitHub.
	renames, err := checkLayerRulesets(ctx, ws, repo, c, baseRef)
	if err != nil {
		return result, err
	}
	gatesOn := eff.Settings.Ship.Gates.Enabled && ws.Runner != nil
	if gatesOn && req.SkipGates && !eff.Settings.Ship.Gates.AllowSkip {
		return result, fault.New(fault.GateSkipRefused, "publish.skip_refused", "source", eff.Source("ship.gates.allowSkip"))
	}
	if gatesOn && !req.SkipGates {
		result.Gates, err = RunGates(ctx, ws, baseRef, req.GateObserver)
		if err != nil {
			return result, err
		}
	}
	if result.Native, err = stacksSupported(ctx, ws, repo); err != nil {
		return result, err
	}
	record := &oplog.StackRecord{Base: c.Base, Native: result.Native}
	var linked *oplog.LinkedIssueChange
	defer func() {
		if len(record.Layers) == 0 && len(record.Renames) == 0 {
			return
		}
		record.Number = result.Number
		before := repositoryViewOf(status.Branch)
		after := before
		after.Branch = c.Top() // renamed, maybe
		var top oplog.StackLayer
		if len(record.Layers) > 0 {
			top = record.Layers[len(record.Layers)-1]
		}
		op, logErr := ws.Operations.Append(ctx, oplog.Operation{
			Kind: oplog.StackPublish, Before: before, After: after, Commit: top.Commit, Title: top.URL,
			Remote: gitRemote(ctx, ws), Stack: record, LinkedIssue: linked,
		})
		result.Operation = op.ID
		err = errors.Join(err, logErr)
	}()
	for _, rn := range renames {
		if err := renameLayer(ctx, ws, &c, rn, record, &result.Warnings); err != nil {
			return result, err
		}
	}

	template := ""
	if baseRef != "" {
		template = pullRequestTemplate(ctx, ws, baseRef, prSettings.Template)
	}
	placement := commitPlacement(ctx, ws, eff)
	issue := req.Issue
	for _, b := range c.Branches {
		if issue == 0 {
			issue = convention.IssueFromBranch(b)
		}
	}
	var issueTitle string
	if issue > 0 {
		issueTitle, _ = ws.Forge.IssueTitle(ctx, repo, issue) // context only
	}
	var numbers []int
	for i, branch := range c.Branches {
		parent := c.Parent(i)
		layer := StackLayerStatus{Branch: branch, Base: parent}
		commit, err := ws.Git.CommitOf(ctx, "refs/heads/"+branch)
		if err != nil {
			return result, err
		}
		pushed := ws.begin(ctx, "stack/push")
		err = ws.Git.Push(ctx, gitRemote(ctx, ws), branch, "")
		pushed(err)
		if err != nil {
			return result, fault.New(fault.PushRejected, "publish.push_rejected", "branch", branch, "detail", lastLine(err.Error()))
		}
		pr, found, err := ws.Forge.FindOpenPullRequest(ctx, repo, branch)
		if err != nil {
			return result, forgeFault(err, repo)
		}
		if !found {
			from := parent
			if i == 0 {
				from = baseRef
			}
			records, err := ws.Git.CommitsBetween(ctx, from, branch)
			if err != nil && !ws.IsDryRun() {
				return result, err // a dry run's layers have no commit yet
			}
			messages := make([]string, len(records))
			for j, r := range records {
				messages[j] = r.Message
			}
			commits, headers := branchCommits(eff.Settings.Conventions(), messages)
			title := pullrequest.TitleAs(headers, placement)
			if title == "" && len(messages) > 0 {
				title, _ = pullrequest.ParseMessage(messages[len(messages)-1])
			}
			if title == "" {
				title = layerTitle(ctx, ws, branch)
			}
			// GitHub reads a closing keyword only on a pull request whose base
			// is the default branch, which only the bottom layer has. The layers
			// above target branches: a keyword there would close nothing until
			// GitHub retargets them, then close the issue a second time. Refs
			// keeps them linked to the issue without it.
			keyword := convention.IssueKeyword(eff.Settings.Ship.Issues.Keyword)
			if i > 0 {
				keyword = convention.ReferencesIssue
			}
			in := pullrequest.BodyInput{
				Issue: issue, IssueTitle: issueTitle, Keyword: keyword, Commits: commits,
				Checks: passedChecks(result.Gates), ChecksSkipped: gatesOn && req.SkipGates,
			}
			body := pullRequestBody(in, template)
			if !result.Native {
				// GitHub shows no stack here: the body says where the layer sits.
				after := 0
				if i > 0 {
					after = numbers[i-1]
				}
				body += "\n" + stack.ChainNote(i+1, len(c.Branches), after) + "\n"
			}
			opened := ws.begin(ctx, "stack/pull_request")
			pr, err = ws.Forge.CreatePullRequest(ctx, repo, ports.PullRequestSpec{
				Title: title, Body: body, Head: branch, Base: parent,
				Draft: draftOf(ws, PublishRequest{ForceDraft: req.ForceDraft, Unattended: req.Unattended}, prSettings.Draft, headers),
			})
			opened(err, slog.Bool("draft", pr.Draft))
			if err != nil {
				return result, forgeFault(err, repo)
			}
			layer.Created = true
			// What a single pull request gets: kind/ and size/ labels, and the
			// linked issue moved in the project by the layer that closes it.
			side := PublishResult{}
			if prSettings.Labels {
				stats, err := ws.Git.DiffStatBetween(ctx, from, "refs/heads/"+branch)
				if err != nil && !ws.IsDryRun() {
					return result, err
				}
				labelPullRequest(ctx, ws, repo, pr.Number, eff.Settings, headers, measure(ctx, ws, stats), &side)
				layer.Labels = side.Labels
			}
			if i == 0 {
				linked = moveLinkedIssue(ctx, ws, repo, eff, issue, pr, &side)
			}
			result.Warnings = append(result.Warnings, side.Warnings...)
		}
		layer.PullRequest = pr
		numbers = append(numbers, pr.Number)
		result.Layers = append(result.Layers, layer)
		record.Layers = append(record.Layers, oplog.StackLayer{
			Branch: branch, Commit: commit, PullRequest: pr.Number, URL: pr.URL, Created: layer.Created,
		})
	}
	if result.Native {
		result.Number = groupInStack(ctx, ws, repo, numbers, &result.Warnings)
	} else {
		// Layers open already say where they sat before this publication.
		renumberChain(ctx, ws, repo, c.Branches, &result.Warnings)
	}
	return result, nil
}

// checkLayerRulesets is checkRulesets for each layer of c, bottom up: its
// branch name and the commits it adds to the layer below. It returns the
// renames to apply, each to a name no other layer has.
func checkLayerRulesets(ctx context.Context, ws Workspace, repo forge.Repository, c stack.Chain, baseRef string) ([]*rulesetRename, error) {
	taken := map[string]bool{}
	for _, b := range c.Branches {
		taken[b] = true
	}
	var renames []*rulesetRename
	for i, b := range c.Branches {
		commit, err := ws.Git.CommitOf(ctx, "refs/heads/"+b)
		if err != nil {
			return nil, err
		}
		base := baseRef
		if i > 0 {
			base = c.Branches[i-1]
		}
		if commit == "" && ws.IsDryRun() {
			base = "" // a rehearsed layer has no commit to check yet
		}
		rename, err := checkRulesetsAt(ctx, ws, repo, ports.BranchState{Name: b, Commit: commit}, base, "refs/heads/"+b)
		if err != nil {
			return nil, err
		}
		if rename == nil {
			continue
		}
		if taken[rename.to] {
			return nil, branchRefused(b, rename.broken, "")
		}
		taken[rename.to] = true
		renames = append(renames, rename)
	}
	return renames, nil
}

// renameLayer gives a layer the name a ruleset requires, in c and in the
// record (ship undo gives the old name back), and says so.
func renameLayer(ctx context.Context, ws Workspace, c *stack.Chain, rn *rulesetRename, record *oplog.StackRecord, warnings *[]ProblemView) error {
	commit, err := ws.Git.CommitOf(ctx, "refs/heads/"+rn.from)
	if err != nil {
		return err
	}
	renamed := ws.begin(ctx, "stack/rename")
	err = ws.Git.RenameBranch(ctx, rn.from, rn.to, commit)
	renamed(err)
	if err != nil {
		return err
	}
	record.Renames = append(record.Renames, oplog.StackRename{From: rn.from, To: rn.to, Commit: commit})
	c.Branches[c.Index(rn.from)] = rn.to
	*warnings = append(*warnings, ProblemView{Code: string(fault.BranchRenamed), Key: "publish.rules.renamed",
		Params: map[string]string{"from": rn.from, "to": rn.to, "rule": rn.broken.Label(), "requirement": rn.broken.Describe()}})
	return nil
}

// groupInStack makes the pull requests (bottom first) one GitHub stack: a
// new one, or the one the lower ones already form, extended on top. A
// refusal leaves the pull requests as a chain and only warns: they are open
// already.
func groupInStack(ctx context.Context, ws Workspace, repo forge.Repository, numbers []int, warnings *[]ProblemView) int {
	refused := func(err error) int {
		*warnings = append(*warnings, ProblemView{Code: string(fault.StackRefused), Key: "stack.refused",
			Params: map[string]string{"detail": lastLine(err.Error())}})
		return 0
	}
	if len(numbers) > 0 && numbers[0] != 0 {
		stacks, err := ws.Stacks.StacksOf(ctx, repo, numbers[0])
		if err != nil {
			return refused(err)
		}
		if len(stacks) > 0 {
			s := stacks[0]
			in := map[int]bool{}
			for _, p := range s.PullRequests {
				in[p.Number] = true
			}
			var missing []int
			for _, n := range numbers {
				if !in[n] {
					missing = append(missing, n)
				}
			}
			if len(missing) == 0 {
				return s.Number
			}
			if _, err := ws.Stacks.AddToStack(ctx, repo, s.Number, missing); err != nil {
				return refused(err)
			}
			return s.Number
		}
	}
	s, err := ws.Stacks.CreateStack(ctx, repo, numbers)
	if err != nil {
		return refused(err)
	}
	return s.Number
}
