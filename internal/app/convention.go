// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"strings"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
)

// Fault is re-exported so surfaces can render results without deciding
// anything from core directly.
type Fault = fault.Fault

// Conventions are a repository's commit and branch conventions: limits,
// commit types and scopes (ship.commits.*, ship.branches.*). Surfaces load
// them once and check what the human types against them. The zero value is
// ship-it's built-in convention.
type Conventions struct {
	rules convention.Rules
}

// LoadConventions returns the conventions of a delivery of paths (nil: the
// repository as a whole), from the effective settings.
func LoadConventions(ctx context.Context, ws Workspace, paths []string) (Conventions, error) {
	eff, err := LoadSettingsFor(ctx, ws, paths)
	if err != nil {
		return Conventions{}, err
	}
	return conventionsOf(eff), nil
}

func conventionsOf(eff config.Effective) Conventions {
	return Conventions{rules: eff.Settings.Conventions()}
}

// TitleCheck is the result of checking a commit or pull request title.
type TitleCheck struct {
	Title string `json:"title"`
	// Canonical is the title rewritten with the canonical gitmoji placement,
	// empty when the title cannot be parsed.
	Canonical string   `json:"canonical,omitempty"`
	Faults    []*Fault `json:"faults,omitempty"`
}

// Valid reports whether the title breaks no rule.
func (c TitleCheck) Valid() bool { return len(c.Faults) == 0 }

// CheckTitle parses and validates a title against the built-in convention.
func CheckTitle(title string) TitleCheck { return Conventions{}.CheckTitle(title) }

// CheckTitle parses and validates a title against the conventions.
func (c Conventions) CheckTitle(title string) TitleCheck {
	return checkTitleAs(c.rules, title, convention.GitmojiFirst, "")
}

// checkTitleAs validates a title and writes its canonical form with the
// repository's gitmoji placement and, when the branch names one, the tracker
// ticket at the end of the subject.
func checkTitleAs(rules convention.Rules, title string, placement convention.GitmojiPlacement, ticket string) TitleCheck {
	result := TitleCheck{Title: title}
	header, err := rules.ParseHeader(title)
	if err != nil {
		result.Faults = []*Fault{asFault(err)}
		return result
	}
	header = convention.WithTicket(header, ticket)
	result.Canonical = rules.Format(header, placement)
	result.Faults = rules.Validate(header)
	return result
}

// habitDepth is how many recent commits tell a repository's habits.
const habitDepth = 50

// commitPlacement is where this repository writes its gitmoji: the setting
// ship.commits.gitmoji, or with "auto" the habit of its history (decision of
// 2026-10-23: what a project does wins over ship-it's format).
func commitPlacement(ctx context.Context, ws Workspace, eff config.Effective) convention.GitmojiPlacement {
	return placementWith(ctx, ws, eff, nil)
}

// placementWith is commitPlacement reading the history through reads, once
// for a plan and the delivery that follows it; nil reads it.
func placementWith(ctx context.Context, ws Workspace, eff config.Effective, reads *repositoryReads) convention.GitmojiPlacement {
	if p, ok := convention.ParsePlacement(eff.Settings.Ship.Commits.Gitmoji); ok {
		return p
	}
	if ws.Git == nil {
		return convention.GitmojiFirst
	}
	if reads == nil {
		reads = &repositoryReads{}
	}
	subjects, err := reads.recentSubjects(ctx, ws)
	if err != nil {
		return convention.GitmojiFirst
	}
	p, _ := eff.Settings.Conventions().DetectGitmojiPlacement(subjects) // the repository's own types count
	return p
}

// TypeOfTitle returns the commit type of a title with the built-in types.
func TypeOfTitle(title string) string { return Conventions{}.TypeOfTitle(title) }

// TypeOfTitle returns the commit type of a title, "" when it does not parse.
func (c Conventions) TypeOfTitle(title string) string {
	h, err := c.rules.ParseHeader(title)
	if err != nil {
		return ""
	}
	return string(h.Type)
}

// RetypeTitle puts commitType in place of the type of title, with the
// built-in types.
func RetypeTitle(title, commitType string) (string, bool) {
	return Conventions{}.RetypeTitle(title, commitType)
}

// RetypeTitle puts commitType in place of the type of title, keeping the
// rest as written (gitmoji, scope, subject): the human's pick among the
// types a project's commitlint allows. ok is false when the title does not
// parse or commitType is not one of the conventions.
func (c Conventions) RetypeTitle(title, commitType string) (string, bool) {
	return c.rules.WithType(title, convention.CommitType(commitType))
}

// BranchCheck is the result of checking a branch name.
type BranchCheck struct {
	Branch string `json:"branch"`
	Fault  *Fault `json:"fault,omitempty"`
}

// Valid reports whether the branch name follows Conventional Branch.
func (c BranchCheck) Valid() bool { return c.Fault == nil }

// CheckBranch validates a branch name against Conventional Branch 1.1.0.
func CheckBranch(name string) BranchCheck { return Conventions{}.CheckBranch(name) }

// CheckBranch validates a branch name against Conventional Branch 1.1.0, the
// branch prefixes of the repository's commit types accepted.
func (c Conventions) CheckBranch(name string) BranchCheck {
	result := BranchCheck{Branch: name}
	if err := c.rules.ValidateBranch(name, nil); err != nil {
		result.Fault = asFault(err)
	}
	return result
}

// MaxTitleLength bounds a commit or pull request title, gitmoji included.
func (c Conventions) MaxTitleLength() int { return c.rules.HeaderMax() }

// DeliveryBranchCheck returns the check DeliverLocally applies to the branch
// it creates for paths: Conventional Branch with the repository's tracker
// keys and branch prefixes. Surfaces call it as the human edits the branch;
// the settings are read once. When they cannot be read, the check runs with
// the built-in convention (the delivery reports the broken configuration
// itself).
func DeliveryBranchCheck(ctx context.Context, ws Workspace, paths []string) func(name string) BranchCheck {
	var keys []string
	var rules convention.Rules
	if eff, err := LoadSettingsFor(ctx, ws, paths); err == nil {
		keys, rules = eff.Settings.Ship.Issues.TrackerKeys, eff.Settings.Conventions()
	}
	return func(name string) BranchCheck {
		result := BranchCheck{Branch: name}
		if err := rules.ValidateBranch(name, keys); err != nil {
			result.Fault = asFault(err)
		}
		return result
	}
}

// BranchRequest describes the work a branch name is suggested for.
type BranchRequest struct {
	CommitType string
	Subject    string
	Issue      int
	Agent      string
	Hotfix     bool
}

// SuggestBranch proposes a branch name with the built-in convention.
func SuggestBranch(req BranchRequest) (string, error) { return Conventions{}.SuggestBranch(req) }

// SuggestBranch proposes a short, valid branch name for a change.
func (c Conventions) SuggestBranch(req BranchRequest) (string, error) {
	spec, ok := c.rules.LookupCommitType(convention.CommitType(req.CommitType))
	if !ok {
		return "", fault.New(fault.UnknownCommitType, "convention.header.unknown_type", "type", req.CommitType)
	}
	return c.rules.BranchName(spec.Type, req.Subject, convention.BranchOptions{
		Issue:  req.Issue,
		Agent:  convention.AgentPrefix(req.Agent),
		Hotfix: req.Hotfix,
	}), nil
}

// BranchForTitle proposes the branch of a conventional title, as the planner
// would for its own title: an agent's title gets a matching branch.
func BranchForTitle(title string, issue int) (string, error) {
	return Conventions{}.BranchForTitleOf(title, BranchRequest{Issue: issue})
}

// BranchForTitleOf is BranchForTitle with the built-in convention.
func BranchForTitleOf(title string, req BranchRequest) (string, error) {
	return Conventions{}.BranchForTitleOf(title, req)
}

// BranchForTitleOf is BranchForTitle with the rest of the request (issue,
// agent prefix, hotfix) given: the review screen proposes the branch of a
// title the human edited, as the planner would have for that title.
func (c Conventions) BranchForTitleOf(title string, req BranchRequest) (string, error) {
	h, err := c.rules.ParseHeader(title)
	if err != nil {
		return "", asFault(err)
	}
	req.CommitType, req.Subject = string(h.Type), h.Subject
	return c.SuggestBranch(req)
}

// CommitTypeView describes a commit type for display.
type CommitTypeView struct {
	Type           string `json:"type"`
	Gitmoji        string `json:"gitmoji"`
	BranchPrefix   string `json:"branch_prefix"`
	DescriptionKey string `json:"description_key"`
	// Label is the kind of its pull requests, after "kind/"; empty for none.
	Label string `json:"label,omitempty"`
}

// CommitTypes lists the built-in commit types in display order.
func CommitTypes() []CommitTypeView { return Conventions{}.CommitTypes() }

// CommitTypes lists the commit types of the conventions in display order.
func (c Conventions) CommitTypes() []CommitTypeView {
	specs := c.rules.CommitTypes()
	views := make([]CommitTypeView, 0, len(specs))
	for _, s := range specs {
		views = append(views, CommitTypeView{
			Type:           string(s.Type),
			Gitmoji:        s.Gitmoji,
			BranchPrefix:   string(s.BranchPrefix),
			DescriptionKey: s.DescriptionKey(),
			Label:          s.Label,
		})
	}
	return views
}

func asFault(err error) *Fault {
	if f, ok := Known(err).(*Fault); ok {
		return f
	}
	return fault.New(fault.Unexpected, "app.unexpected_error", "error", err.Error())
}

// TypeAllowed reports whether title's type is one of choices (the types a
// project's commitlint allows, PlanView.TypeChoices); with no choices to
// make, every type is. It returns the fault to show otherwise. The type is
// read by the header grammar alone, so that a type of the repository's own
// table is read too.
func TypeAllowed(title string, choices []string) *Fault {
	if len(choices) == 0 {
		return nil
	}
	t := string(convention.WrittenType(title))
	for _, c := range choices {
		if c == t {
			return nil
		}
	}
	return fault.New(fault.CommitlintUnsatisfiable, "deliver.commitlint.type_not_allowed",
		"type", t, "allowed", strings.Join(choices, ", "))
}
