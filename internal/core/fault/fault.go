// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package fault defines the typed errors returned by core and app.
//
// A Fault carries a stable code (SHIP-E####) and a message key with
// parameters. It never carries a translated sentence: surfaces render the key
// through internal/i18n. Codes are never reused (docs/reference/logs.md).
package fault

import (
	"fmt"
	"sort"
	"strings"
)

// Code is a stable, documented error code such as "SHIP-E0101".
type Code string

// Codes of titles, branch names and sign-offs (01xx range).
const (
	EmptyHeader           Code = "SHIP-E0101"
	MalformedHeader       Code = "SHIP-E0102"
	UnknownCommitType     Code = "SHIP-E0103"
	SubjectTooLong        Code = "SHIP-E0104"
	HeaderTooLong         Code = "SHIP-E0105"
	SubjectNotLowercase   Code = "SHIP-E0106"
	SubjectEndsWithPeriod Code = "SHIP-E0107"
	EmptySubject          Code = "SHIP-E0108"
	InvalidScope          Code = "SHIP-E0109"
	InvalidBranch         Code = "SHIP-E0110"
	UnknownBranchPrefix   Code = "SHIP-E0111"
	InvalidVersion        Code = "SHIP-E0112"
	BreakingNotAllowed    Code = "SHIP-E0113"
	SignoffMissing        Code = "SHIP-E0114"
	SignoffMismatch       Code = "SHIP-E0115"
	SignoffNoBase         Code = "SHIP-E0116"
	// ScopeNotAllowed: a scope outside ship.commits.scopes.
	ScopeNotAllowed Code = "SHIP-E0117"
	// ScopeRequired: no scope while ship.commits.scopeRequired is set.
	ScopeRequired Code = "SHIP-E0118"
	// SigningKeyMissing: ship.commits.sign is "always" and git has no
	// signing key.
	SigningKeyMissing Code = "SHIP-E0119"
)

// Unexpected wraps an error that has no dedicated code yet.
const Unexpected Code = "SHIP-E0001"

// Codes of delivery (05xx range).
const (
	DoctorBlocking Code = "SHIP-E0501"
)

// Codes of checkpoints and undo (04xx range).
const (
	UnknownCheckpoint             Code = "SHIP-E0401"
	AmbiguousCheckpoint           Code = "SHIP-E0402"
	NothingToUndo                 Code = "SHIP-E0403"
	RepositoryMovedSinceOperation Code = "SHIP-E0404"
	UndoNeedsCleanTree            Code = "SHIP-E0405"
	UndoOfPublishedWork           Code = "SHIP-E0406"
	UndoNeedsConsent              Code = "SHIP-E0407"
	UndoOfMergedWork              Code = "SHIP-E0408"
	RemoteBranchMoved             Code = "SHIP-E0409"
	// ConfigFileChanged: a configuration file changed since ship-it read it
	// (ship config set, ship undo of an edit); nothing was written.
	ConfigFileChanged Code = "SHIP-E0410"
	// ProjectStatusChanged: undoing a publication found the linked issue's
	// status in the project changed since; ship undo keeps that change.
	ProjectStatusChanged Code = "SHIP-W0411"
)

// Codes of configuration files (07xx range).
const (
	ConfigInvalidJSON  Code = "SHIP-E0701"
	ConfigUnknownKey   Code = "SHIP-E0702"
	ConfigWrongType    Code = "SHIP-E0703"
	ConfigInvalidValue Code = "SHIP-E0704"
	ConfigTooNew       Code = "SHIP-E0705"
	ConfigAlreadyThere Code = "SHIP-E0706"
	SyncReadOnly       Code = "SHIP-E0707"
	// ConfigPresetUnavailable: a preset named in "extends" cannot be read
	// (missing file, GitHub unreachable and not cached).
	ConfigPresetUnavailable Code = "SHIP-E0708"
	// ConfigPresetCycle: presets extend each other in a loop, or too deeply.
	ConfigPresetCycle Code = "SHIP-E0709"
	// RoadmapMissing: ship roadmap sync found no roadmap file, or one with
	// no milestone (a "## " heading holding a checklist).
	RoadmapMissing Code = "SHIP-E0710"
	// ProjectNotFound: the project of github.project is not visible to the
	// token.
	ProjectNotFound Code = "SHIP-E0711"
	// ProjectAccessDenied: the token may not read or change the project (the
	// project scope, or the "Projects" permission of a fine-grained token).
	ProjectAccessDenied Code = "SHIP-E0712"
	// ProjectOtherHost: the project is on another GitHub host than origin.
	ProjectOtherHost Code = "SHIP-E0713"
	// ProjectIncomplete: a configured field or option is not in the project;
	// ship-it reports it and never creates it.
	ProjectIncomplete Code = "SHIP-W0714"
	// MCPManualEdit: ship mcp install cannot add ship-it to a client's file
	// without risking the rest of it; the user adds the snippet by hand.
	MCPManualEdit Code = "SHIP-E0715"
)

// Codes of safety gates (09xx range): they exit with code 3.
const (
	SecretsFound    Code = "SHIP-E0901"
	GateFailed      Code = "SHIP-E0902"
	GateToolMissing Code = "SHIP-E0903"
	// GateSkipRefused: --skip-gates while ship.gates.allowSkip is false.
	GateSkipRefused Code = "SHIP-E0904"
	// CommitlintRefused: the project's commitlint rejects the message.
	CommitlintRefused Code = "SHIP-E0905"
	// CommitlintUnsatisfiable: no title ship-it can write satisfies the
	// project's commitlint rules (a type it does not allow…).
	CommitlintUnsatisfiable Code = "SHIP-E0906"
)

// Codes of the organisation policy (08xx range): they exit with code 5.
const (
	PolicyConflict Code = "SHIP-E0801"
)

// Codes of publishing to GitHub (06xx range).
const (
	PublishFromTrunk         Code = "SHIP-E0601"
	NoGitHubRemote           Code = "SHIP-E0602"
	NotAuthenticated         Code = "SHIP-E0603"
	TokenRejected            Code = "SHIP-E0604"
	GitHubRepositoryNotFound Code = "SHIP-E0605"
	RateLimited              Code = "SHIP-E0606"
	NothingToPublish         Code = "SHIP-E0607"
	PushRejected             Code = "SHIP-E0608"
	FetchFailed              Code = "SHIP-W0609"
	RebaseConflict           Code = "SHIP-W0610"
	RemoteAhead              Code = "SHIP-W0611"
	// BranchNameRefused: a ruleset's branch name pattern rejects the branch
	// and ship-it may not or cannot rename it.
	BranchNameRefused Code = "SHIP-E0612"
	// CommitMessageRefused: a commit of the branch breaks a ruleset's commit
	// message pattern.
	CommitMessageRefused Code = "SHIP-E0613"
	// CommitEmailRefused: a commit's author or committer e-mail breaks a
	// ruleset's e-mail pattern.
	CommitEmailRefused Code = "SHIP-E0614"
	// UnsignedCommit: a ruleset requires signed commits and one is not.
	UnsignedCommit Code = "SHIP-E0615"
	// BranchRenamed: ship-it renamed its own branch for a ruleset.
	BranchRenamed Code = "SHIP-W0616"
	// MergeConflict: merging the base into the branch (ship.publish.update
	// merge) conflicts; the branch is published as it is.
	MergeConflict Code = "SHIP-W0617"
	// OverlappingPullRequest: an open pull request changes the same or
	// adjacent lines as the branch, a likely conflict for whoever merges
	// second; the publication goes on.
	OverlappingPullRequest Code = "SHIP-W0619"
	// GitHubNoAnswer: a request to GitHub got no answer (network, proxy,
	// outage).
	GitHubNoAnswer Code = "SHIP-E0618"
)

// Codes of planning a delivery (02xx range).
const (
	NothingToShip Code = "SHIP-E0201"
)

// Codes of the environment, checked by ship doctor (03xx range). Healthy
// marks a passing check; it is not an error.
const (
	Healthy                Code = "SHIP-I0000"
	GitMissing             Code = "SHIP-E0301"
	GitTooOld              Code = "SHIP-E0302"
	GitIdentityMissing     Code = "SHIP-E0303"
	LongPathsDisabled      Code = "SHIP-W0304"
	NotARepository         Code = "SHIP-W0305"
	OperationInProgress    Code = "SHIP-E0306"
	DetachedHead           Code = "SHIP-W0307"
	NoOriginRemote         Code = "SHIP-W0308"
	MirrorRemote           Code = "SHIP-W0309"
	GitHubNoToken          Code = "SHIP-W0310"
	GitHubTokenRejected    Code = "SHIP-W0311"
	GitHubRepositoryHidden Code = "SHIP-W0312"
	GitHubUnreachable      Code = "SHIP-W0313"
	TrackedJunk            Code = "SHIP-W0314"
	// RemoteNotGitHub: the remote is not a GitHub repository; local work only.
	RemoteNotGitHub Code = "SHIP-W0315"
	// NotInRepository: a command that works on a repository ran outside one.
	NotInRepository Code = "SHIP-E0316"
)

// Codes of stacks of pull requests (10xx range).
const (
	// StackNeedsLayers: the work cannot make a stack of at least two layers
	// (a single area, an empty layer).
	StackNeedsLayers Code = "SHIP-E1001"
	// NotInStack: the current branch is not a layer of a stack ship-it built.
	NotInStack Code = "SHIP-E1002"
	// StackRefused: GitHub refused to create or change the stack (422: the
	// pull requests cannot form one; 409: another request is changing it).
	StackRefused Code = "SHIP-E1003"
	// StackNotOnTop: the stack is published from its top layer only, where
	// every layer's commits are reachable.
	StackNotOnTop Code = "SHIP-E1004"
	// StackParentUnpublished: a new layer sits on a layer that is not on
	// GitHub yet; its pull request would have no base.
	StackParentUnpublished Code = "SHIP-E1005"
	// MergeNeedsConfirmation: ship stack merge needs a human's confirmation
	// in a terminal, or --yes; never an AI agent.
	MergeNeedsConfirmation Code = "SHIP-E1006"
	// MergeFailed: GitHub ran the merge and it failed (a rule, a check, a
	// conflict); nothing was merged.
	MergeFailed Code = "SHIP-E1007"
	// MergeStillRunning: the merge did not finish while ship-it waited; it
	// goes on on GitHub.
	MergeStillRunning Code = "SHIP-W1008"
	// MergeQueued: the stack joined the merge queue, which merges it later.
	MergeQueued Code = "SHIP-W1009"
	// StackLayerUnpushed: with ship.stack.rebase = github, a layer holds
	// commits that were never pushed; its branch is left alone.
	StackLayerUnpushed Code = "SHIP-W1010"
	// StackRebaseConflict: rebasing a layer on the one below conflicted; the
	// rebase was aborted and that layer and those above are left as they were.
	StackRebaseConflict Code = "SHIP-W1011"
	// NothingToSync: no layer of the stack was merged.
	NothingToSync Code = "SHIP-W1012"
	// StackNotFromTrunk: ship stack starts from the default branch, where
	// the bottom layer is based.
	StackNotFromTrunk Code = "SHIP-E1013"
	// StackNeedsCleanTree: a sync rebases and switches branches; the work in
	// progress must be committed (or put aside) first.
	StackNeedsCleanTree Code = "SHIP-E1014"
	// StackChangedSince: undoing a stack sync found a remote branch, the base
	// of a pull request or its chain line changed by someone since; theirs
	// is kept, the rest of the undo goes on.
	StackChangedSince Code = "SHIP-W1015"
)

// Fault is an error with a stable code, a message key and parameters.
type Fault struct {
	Code   Code
	Key    string
	Params map[string]string
}

// New builds a Fault. Params are given as key/value pairs.
func New(code Code, key string, kv ...string) *Fault {
	f := &Fault{Code: code, Key: key}
	if len(kv) > 0 {
		f.Params = make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			f.Params[kv[i]] = kv[i+1]
		}
	}
	return f
}

// Error returns a stable, untranslated representation meant for logs.
func (f *Fault) Error() string {
	if len(f.Params) == 0 {
		return fmt.Sprintf("%s %s", f.Code, f.Key)
	}
	keys := make([]string, 0, len(f.Params))
	for k := range f.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+f.Params[k])
	}
	return fmt.Sprintf("%s %s {%s}", f.Code, f.Key, strings.Join(parts, " "))
}

// Is reports whether target is a Fault with the same code, so that
// errors.Is(err, &Fault{Code: X}) works.
func (f *Fault) Is(target error) bool {
	t, ok := target.(*Fault)
	return ok && t.Code == f.Code
}
