// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/commitlint"
	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/convention"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/ports"
)

// commitlintTimeout bounds one commitlint run: npx and Node start slowly,
// a hung one must not hang ship.
const commitlintTimeout = time.Minute

// CommitlintRefusedError stops a delivery the project's commitlint rejects.
// It wraps SHIP-E0905 and carries commitlint's own output.
type CommitlintRefusedError struct {
	Title  string
	Output string
}

func (e *CommitlintRefusedError) Error() string { return e.Unwrap().Error() }

// Unwrap exposes the fault.
func (e *CommitlintRefusedError) Unwrap() error {
	return fault.New(fault.CommitlintRefused, "deliver.commitlint.refused", "title", e.Title)
}

// CommitlintOutputIn returns commitlint's output an error carries, if it is
// commitlint's refusal.
func CommitlintOutputIn(err error) (string, bool) {
	var refused *CommitlintRefusedError
	if errors.As(err, &refused) {
		return refused.Output, true
	}
	return "", false
}

// projectCommitlint returns the project's resolved commitlint configuration,
// or nil when the project has none installed: a configuration file (or a
// package.json field) and the local binary, never a download. Anything
// unreadable means none: commitlint itself would say more at commit time.
func projectCommitlint(ctx context.Context, ws Workspace) *commitlint.Config {
	if ws.Runner == nil || ws.Git == nil {
		return nil
	}
	exists := func(path string) bool {
		_, err := ws.Git.ReadWorkingFile(ctx, path, 0)
		return err == nil || errors.Is(err, ports.ErrFileTooLarge)
	}
	configured := slices.ContainsFunc(commitlint.ConfigFiles(), exists)
	if !configured {
		if pkg, err := ws.Git.ReadWorkingFile(ctx, "package.json", packageJSONLimit); err == nil {
			configured = commitlint.DeclaredInPackageJSON(pkg)
		}
	}
	if !configured || !slices.ContainsFunc(commitlint.Binaries(), exists) {
		return nil
	}
	run, err := runCommitlint(ctx, ws, commitlint.PrintConfigCommand(), nil)
	if err != nil || run.ExitCode != 0 {
		ws.logger().LogAttrs(ctx, slog.LevelWarn, "commitlint.unreadable",
			slog.Int("exit_code", run.ExitCode), slog.Bool("ran", err == nil))
		return nil
	}
	cfg, err := commitlint.ParseConfig(run.Output)
	if err != nil {
		ws.logger().LogAttrs(ctx, slog.LevelWarn, "commitlint.unreadable", slog.String("error", err.Error()))
		return nil
	}
	return &cfg
}

func runCommitlint(ctx context.Context, ws Workspace, command []string, stdin []byte) (ports.CommandResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, commitlintTimeout)
	defer cancel()
	step := ws.begin(ctx, "commitlint")
	var run ports.CommandResult
	var err error
	if stdin == nil {
		run, err = ws.Runner.Run(runCtx, command)
	} else {
		run, err = ws.Runner.RunWithInput(runCtx, command, stdin)
	}
	step(err, slog.String("cmd", strings.Join(command, " ")), slog.Int("exit_code", run.ExitCode))
	return run, err
}

// followCommitlint writes h so that the project's commitlint accepts it:
// adapted to its rules, then linted for real. The preferred gitmoji
// placement (setting or history) goes first; when commitlint rejects it, the
// title goes without gitmoji, since the project wins over the setting too
// (rule zero). message writes the full commit message of a title (its body
// and trailers), which is what commitlint checks; nil means the title alone.
// It returns the title and the reasons to show in the plan.
func followCommitlint(ctx context.Context, ws Workspace, cfg *commitlint.Config, rules convention.Rules, h convention.Header,
	preferred convention.GitmojiPlacement, message func(title string) string) (string, []ReasonView, error) {
	if message == nil {
		message = func(title string) string { return title + "\n" }
	}
	placements := []convention.GitmojiPlacement{preferred}
	if preferred != convention.GitmojiOmitted {
		placements = append(placements, convention.GitmojiOmitted)
	}
	var unsatisfiable error
	var refused *CommitlintRefusedError
	for i, placement := range placements {
		adapted, reasons, f := commitlint.Adapt(h, placement, *cfg, rules)
		if f != nil {
			if unsatisfiable == nil {
				unsatisfiable = f
			}
			continue
		}
		title := rules.Format(adapted, placement)
		run, err := runCommitlint(ctx, ws, commitlint.LintCommand(), []byte(message(title)))
		if err != nil {
			return "", nil, err
		}
		if run.ExitCode != 0 {
			if refused == nil {
				refused = &CommitlintRefusedError{Title: title, Output: string(run.Output)}
			}
			continue
		}
		views := make([]ReasonView, 0, len(reasons)+2)
		for _, r := range reasons {
			views = append(views, ReasonView{Key: r.Key, Params: r.Params})
		}
		if i > 0 {
			views = append(views, ReasonView{Key: "plan.reason.commitlint.gitmoji"})
		}
		views = append(views, ReasonView{Key: "plan.reason.commitlint"})
		return title, views, nil
	}
	if refused != nil {
		return "", nil, refused
	}
	return "", nil, unsatisfiable
}

// commitlintStyle are ship-it's own style rules, which give way to the
// project's commitlint when it has one (subject length and case, period,
// scope characters, header length).
var commitlintStyle = []fault.Code{
	fault.SubjectTooLong, fault.HeaderTooLong, fault.SubjectNotLowercase, fault.SubjectEndsWithPeriod, fault.InvalidScope,
}

// withoutStyleFaults keeps the faults commitlint does not decide: grammar,
// unknown type, empty subject, breaking change on a type that cannot be.
func withoutStyleFaults(faults []*Fault) []*Fault {
	return withoutFaults(faults, commitlintStyle)
}

func withoutFaults(faults []*Fault, codes []fault.Code) []*Fault {
	return slices.DeleteFunc(slices.Clone(faults), func(f *Fault) bool { return slices.Contains(codes, f.Code) })
}

// commitlintLengths pairs ship-it's length settings with the commitlint rule
// that decides the same length, and the fault of ship-it's own check.
var commitlintLengths = []struct {
	key, rule string
	code      fault.Code
}{
	{"ship.commits.subjectMaxLength", commitlint.SubjectMaxLength, fault.SubjectTooLong},
	{"ship.commits.headerMaxLength", commitlint.HeaderMaxLength, fault.HeaderTooLong},
}

// explicitlySet reports whether a layer above the defaults set key.
func explicitlySet(eff config.Effective, key string) bool {
	_, set := eff.Sources[key]
	return set
}

// givingWayTo returns the style faults ship-it leaves to the project's
// commitlint (decision of 2026-11-05): case, period and scope characters
// always; a length when commitlint has a rule for it, or when nobody set
// ship-it's own limit. An explicitly set limit commitlint has no rule for
// still applies.
func givingWayTo(cfg *commitlint.Config, eff config.Effective) []fault.Code {
	codes := []fault.Code{fault.SubjectNotLowercase, fault.SubjectEndsWithPeriod, fault.InvalidScope}
	for _, l := range commitlintLengths {
		if _, ruled := cfg.MaxLength(l.rule); ruled || !explicitlySet(eff, l.key) {
			codes = append(codes, l.code)
		}
	}
	return codes
}

// bodyWidthWith is the column bodies are wrapped at with the project's
// commitlint: its body-max-line-length wins, so the body never exceeds it
// (ship-it's own width, explicit or default, is kept when narrower);
// without the rule, ship.commits.bodyWidth applies.
func bodyWidthWith(cfg *commitlint.Config, rules convention.Rules) int {
	if limit, ok := cfg.MaxLength(commitlint.BodyMaxLineLength); ok && limit > 0 {
		return min(rules.Width(), limit)
	}
	return rules.Width()
}
