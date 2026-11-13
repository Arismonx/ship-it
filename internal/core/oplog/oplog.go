// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package oplog

import (
	"time"

	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/settings"
)

// Kind names what an operation did.
type Kind string

// Operation kinds.
const (
	Deliver Kind = "deliver"
	// Publish pushed a branch and opened (or found) its pull request.
	Publish Kind = "publish"
	// Sync changed repository settings on GitHub (labels). It is recorded for
	// audit; `ship undo` reverts deliveries, not settings.
	Sync Kind = "sync"
	// Tidy left a merged branch for the up-to-date base and deleted local
	// branches whose pull request was merged.
	Tidy Kind = "tidy"
	// Fix applied remedies of ship fix: configuration values, untracked files.
	Fix Kind = "fix"
	// Rebase replayed a branch on its base before publishing; Title holds the
	// base. Recorded for ship log, not undone (the publication that follows
	// is what an undo reverts).
	Rebase Kind = "rebase"
	// Merge merged the base into a branch before publishing
	// (ship.publish.update merge); Title holds the base. Recorded for ship
	// log, not undone, like Rebase.
	Merge Kind = "merge"
	// Configure changed a configuration file (ship config set or unset);
	// ConfigFile holds the content before and after.
	Configure Kind = "config"
	// Rename gave a branch ship-it created, not pushed yet, the name a
	// GitHub ruleset requires, right before publishing it: Before.Branch is
	// the old name, After.Branch the new one. ship undo renames it back.
	Rename Kind = "rename"
	// Roadmap synchronised ROADMAP.md with GitHub (ship roadmap sync);
	// RoadmapSync holds what it created and changed, FilesBefore the file.
	Roadmap Kind = "roadmap"
	// Rewind wrote back the files of a checkpoint (ship rewind, #39);
	// Checkpoint holds the files as they were before, FilesBefore the files
	// the rewind recreated, which did not exist then.
	Rewind Kind = "rewind"
	Undo   Kind = "undo"
)

// RoadmapSync records what a roadmap sync did on GitHub, so that undo can
// compensate it: created issues are closed as not planned (never deleted),
// created milestones are deleted when nothing else uses them, issue states
// go back, sub-issue links are removed.
type RoadmapSync struct {
	CreatedMilestones []int          `json:"created_milestones,omitempty"`
	CreatedIssues     []CreatedIssue `json:"created_issues,omitempty"`
	States            []IssueState   `json:"states,omitempty"`
	Parents           []ParentLink   `json:"parents,omitempty"`
	// Project is what the sync changed in the GitHub Project, nil if nothing.
	Project *ProjectSync `json:"project,omitempty"`
}

// ProjectSync records the changes a roadmap sync made in a project: undo
// removes the items it added and puts the field values it changed back.
type ProjectSync struct {
	Project forge.Project `json:"project"`
	// AddedItems are the ids of the items the sync added.
	AddedItems []int64 `json:"added_items,omitempty"`
	// Values are the values it changed on items that were already there.
	Values []FieldValue `json:"values,omitempty"`
}

// FieldValue is the value a field of an item had before the sync: the raw
// value the API takes back (option id, date), "" for an empty field.
type FieldValue struct {
	Item   int64  `json:"item"`
	Field  int64  `json:"field"`
	Before string `json:"before,omitempty"`
}

// CreatedIssue is an issue the sync created, and its milestone.
type CreatedIssue struct {
	Number    int `json:"number"`
	Milestone int `json:"milestone,omitempty"`
}

// IssueState is the state an issue had before the sync changed it.
type IssueState struct {
	Number  int  `json:"number"`
	WasOpen bool `json:"was_open"`
}

// ParentLink is a sub-issue link the sync made.
type ParentLink struct {
	Parent int   `json:"parent"`
	Child  int64 `json:"child"`
}

// RepositoryView is where the user stood at a point in time.
type RepositoryView struct {
	// Branch is empty when HEAD is detached.
	Branch string `json:"branch,omitempty"`
	// Head is empty before the first commit.
	Head     string `json:"head,omitempty"`
	Detached bool   `json:"detached,omitempty"`
}

// Operation is one entry of the append-only operation log. Entries are never
// modified: undoing an operation appends an Undo entry that points to it.
type Operation struct {
	ID        string    `json:"id"`
	Kind      Kind      `json:"kind"`
	StartedAt time.Time `json:"started_at"`
	// Checkpoint holds the working tree as it was before the operation.
	Checkpoint string         `json:"checkpoint,omitempty"`
	Before     RepositoryView `json:"before"`
	After      RepositoryView `json:"after"`
	// CreatedBranch is set when the operation created After.Branch.
	CreatedBranch bool   `json:"created_branch,omitempty"`
	Commit        string `json:"commit,omitempty"`
	Title         string `json:"title,omitempty"`
	// Undoes is the id of the operation an Undo entry reverts.
	Undoes string `json:"undoes,omitempty"`
	// Only lists the paths of a partial delivery; the rest of the work stayed
	// uncommitted.
	Only []string `json:"only,omitempty"`
	// AIAssistant names who wrote the plan of a delivery when it was not the
	// deterministic planner ("host agent" over MCP); the pull request says so.
	AIAssistant string `json:"ai_assistant,omitempty"`
	// Agent names the AI agent that made the operation through MCP (the
	// client ship mcp --client names, or the one its handshake gives); empty
	// for a person at the terminal (#49).
	Agent string `json:"agent,omitempty"`

	// Remote effects of a Publish, recorded so that compensations can come
	// later (docs/design/undo-storage-optimizations.md: close the pull request, delete the branch if unchanged).
	Remote         string `json:"remote,omitempty"`
	PullRequest    int    `json:"pull_request,omitempty"`
	PullRequestURL string `json:"pull_request_url,omitempty"`
	// PullRequestCreated: the publication opened the pull request, rather
	// than updating one that was open. Only then can an undo close it.
	PullRequestCreated bool `json:"pull_request_created,omitempty"`
	// PullRequestRepository is the repository of the pull request, as
	// "owner/name", when it is not the remote's: the parent of a fork. Its
	// head is then "<owner of the remote>:<branch>".
	PullRequestRepository string `json:"pull_request_repository,omitempty"`
	// PullRequestMilestone is the milestone the publication put the pull
	// request in (ship.pullRequest.milestone): an undo takes it out, so that
	// the milestone does not count a closed pull request as done.
	PullRequestMilestone int `json:"pull_request_milestone,omitempty"`
	// LabelChanges are the label changes a Sync applied.
	LabelChanges []settings.LabelChange `json:"label_changes,omitempty"`
	// DeletedBranches are the local branches a Tidy deleted, with their
	// commits, so that an undo recreates them exactly.
	DeletedBranches []BranchRef `json:"deleted_branches,omitempty"`
	// ConfigChanges, Untracked and FilesBefore describe what a Fix changed.
	ConfigChanges []ConfigChange `json:"config_changes,omitempty"`
	Untracked     []string       `json:"untracked,omitempty"`
	FilesBefore   []FileBefore   `json:"files_before,omitempty"`
	// ConfigFile is the file a Configure edited.
	ConfigFile *ConfigFileEdit `json:"config_file,omitempty"`
	// RoadmapSync is what a Roadmap operation did on GitHub.
	RoadmapSync *RoadmapSync `json:"roadmap_sync,omitempty"`
	// LinkedIssue is what a Publish changed on the issue its pull request
	// closes, in the GitHub Project; nil when it changed nothing (and in the
	// operations logged before it was recorded).
	LinkedIssue *LinkedIssueChange `json:"linked_issue,omitempty"`
	// Stack is what a stack operation did (stack.go).
	Stack *StackRecord `json:"stack,omitempty"`
	// StackParent is set on a Deliver that added a layer on top of this
	// branch, the top of a stack: After.Branch is the new layer.
	StackParent string `json:"stack_parent,omitempty"`
}

// LinkedIssueChange records what a publication changed on the issue its pull
// request closes, so that an undo puts it back without guessing: the status
// of its item in the project, and the assignee it added.
type LinkedIssueChange struct {
	Issue int `json:"issue"`
	// Project, Item and Field locate the status the publication set; Field is
	// 0 when it did not change the status.
	Project forge.Project `json:"project"`
	Item    int64         `json:"item,omitempty"`
	Field   int64         `json:"field,omitempty"`
	// Before is the option the status had ("" for no status), After the one
	// the publication set: an undo restores Before only if the status still
	// holds After.
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	// Assigned is the login the publication assigned the issue to, when it
	// had no assignee.
	Assigned string `json:"assigned,omitempty"`
}

// ConfigFileEdit is a configuration file a Configure rewrote, whole, so that
// an undo puts it back byte for byte, comments included.
type ConfigFileEdit struct {
	// Scope is "user" or "repository"; Path is shown in messages.
	Scope string `json:"scope"`
	Path  string `json:"path"`
	// Key is the setting that was set or unset.
	Key string `json:"key"`
	// Before and BeforeExisted describe the file before the edit: an undo
	// removes a file the edit created. After is what the edit wrote: the undo
	// is refused when the file no longer holds it (compare-and-swap).
	Before        string `json:"before"`
	BeforeExisted bool   `json:"before_existed"`
	After         string `json:"after"`
}

// ConfigChange is a configuration value a Fix changed, with what was there
// before in that very scope.
type ConfigChange struct {
	Key    string `json:"key"`
	Global bool   `json:"global,omitempty"`
	// Old and HadOld describe the previous value; an undo restores it, or
	// removes the key when there was none.
	Old    string `json:"old,omitempty"`
	HadOld bool   `json:"had_old,omitempty"`
}

// FileBefore is a working tree file a Fix rewrote, as it was.
type FileBefore struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	// Existed is false when the Fix created the file.
	Existed bool `json:"existed"`
}

// BranchRef is a branch and the commit it pointed to.
type BranchRef struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// Undoable returns the deliveries and publications that have not been undone,
// in the order of operations (newest first).
func Undoable(operations []Operation) []Operation {
	undone := map[string]bool{}
	for _, op := range operations {
		if op.Kind == Undo {
			undone[op.Undoes] = true
		}
	}
	var undoable []Operation
	for _, op := range operations {
		// A tidy that deleted nothing only switched branch: nothing to undo.
		tidyWithDeletions := op.Kind == Tidy && len(op.DeletedBranches) > 0
		if (op.Kind == Deliver || op.Kind == Publish || op.Kind == Fix || op.Kind == Configure || op.Kind == Rename || op.Kind == Roadmap || op.Kind == Rewind || tidyWithDeletions ||
			op.Kind == Stack || op.Kind == StackPublish || op.Kind == StackSync) && !undone[op.ID] {
			undoable = append(undoable, op)
		}
	}
	return undoable
}

// LastUndoable returns the most recent operation that has not been undone.
// operations are ordered newest first.
func LastUndoable(operations []Operation) (Operation, bool) {
	if undoable := Undoable(operations); len(undoable) > 0 {
		return undoable[0], true
	}
	return Operation{}, false
}

// StepKind is one compensating action of an undo.
type StepKind string

// Undo steps. None of them rewrites the files of the working tree except
// RestoreCheckpoint, and every ref change is a compare-and-swap on Expected.
const (
	// AttachHead points HEAD at Branch again, files untouched.
	AttachHead StepKind = "attach_head"
	// DetachHead points HEAD at Commit, files untouched.
	DetachHead StepKind = "detach_head"
	// MoveBranch moves Branch from Expected back to Commit.
	MoveBranch StepKind = "move_branch"
	// DeleteBranch removes Branch if it still points at Expected.
	DeleteBranch StepKind = "delete_branch"
	// ResetIndex makes the index match Commit (empty before the first commit),
	// so the changes show as uncommitted again.
	ResetIndex StepKind = "reset_index"
	// RestoreCheckpoint writes back the files saved before the operation,
	// byte for byte, in case a hook changed them while committing.
	RestoreCheckpoint StepKind = "restore_checkpoint"
	// RecreateBranch creates Branch at Commit again; refused if it exists.
	RecreateBranch StepKind = "recreate_branch"
	// ClosePullRequest closes pull request Number on GitHub, with a comment.
	ClosePullRequest StepKind = "close_pull_request"
	// DeleteRemoteBranch deletes Branch on the remote only if it still points
	// at Expected (push --force-with-lease, which also guards deletions:
	// checked on git 2.52 and by an integration test).
	DeleteRemoteBranch StepKind = "delete_remote_branch"
	// RestoreConfig puts Config back (or removes the key).
	RestoreConfig StepKind = "restore_config"
	// TrackFiles adds Paths back to the index.
	TrackFiles StepKind = "track_files"
	// RestoreFile writes File back (or removes it when it did not exist).
	RestoreFile StepKind = "restore_file"
	// RestoreConfigFile puts ConfigFile.Before back (or removes the file)
	// only if the file still holds ConfigFile.After.
	RestoreConfigFile StepKind = "restore_config_file"
	// RenameBranch renames Branch back to Target if Branch still points at
	// Expected; refused if Target exists.
	RenameBranch StepKind = "rename_branch"
	// The compensations of a roadmap sync, on GitHub: CloseCreatedIssue
	// closes Number as not planned with a comment, RestoreIssueState puts its
	// WasOpen state back, RemoveSubIssue unlinks Child from Number, and
	// DeleteMilestone removes milestone Number if no issue uses it.
	CloseCreatedIssue StepKind = "close_created_issue"
	RestoreIssueState StepKind = "restore_issue_state"
	RemoveSubIssue    StepKind = "remove_sub_issue"
	DeleteMilestone   StepKind = "delete_milestone"
	// RestoreProjectField puts Value back in field Field of item Item of
	// Project ("" empties it); when Expected is set, only if the field still
	// holds it (a later change by hand is kept). RemoveProjectItem removes
	// item Item.
	RestoreProjectField StepKind = "restore_project_field"
	RemoveProjectItem   StepKind = "remove_project_item"
	// UnassignIssue removes Assignee from issue Number, if it is still
	// assigned (undo of a publication that assigned it).
	UnassignIssue StepKind = "unassign_issue"
)

// Remote reports whether the step changes something on GitHub: the human
// must agree to it explicitly.
func (s Step) Remote() bool {
	switch s.Kind {
	case ClosePullRequest, DeleteRemoteBranch, CloseCreatedIssue, RestoreIssueState, RemoveSubIssue, DeleteMilestone,
		RestoreProjectField, RemoveProjectItem, UnassignIssue, Unstack, RestoreRemoteBranch, RetargetPullRequest, RestoreChainNote:
		return true
	}
	return false
}

// NeedsCleanTree reports whether undoing op requires no uncommitted work:
// undoing a delivery restores its checkpoint over the files, recreating
// branches touches nothing. A partial delivery left work uncommitted on
// purpose, so its undo moves refs and index only.
func NeedsCleanTree(op Operation) bool {
	return (op.Kind == Deliver && len(op.Only) == 0) || op.Kind == Stack || op.Kind == StackSync
}

// Step is one compensating action. Only the fields its Kind documents are set.
type Step struct {
	Kind       StepKind `json:"kind"`
	Branch     string   `json:"branch,omitempty"`
	Commit     string   `json:"commit,omitempty"`
	Expected   string   `json:"expected,omitempty"`
	Checkpoint string   `json:"checkpoint,omitempty"`
	// Target is the name RenameBranch gives Branch back, the base
	// RetargetPullRequest gives pull request Number back.
	Target string `json:"target,omitempty"`
	// Number is the pull request of ClosePullRequest, the issue or the
	// milestone of the roadmap compensations, the linked issue of
	// UnassignIssue and of a RestoreProjectField that undoes a publication.
	Number int `json:"number,omitempty"`
	// Child and WasOpen are the arguments of RemoveSubIssue and
	// RestoreIssueState.
	Child   int64 `json:"child,omitempty"`
	WasOpen bool  `json:"was_open,omitempty"`
	// Issues is how many issues DeleteMilestone may find in the milestone:
	// those the sync created, now closed; any other means it is in use.
	Issues int `json:"issues,omitempty"`
	// Config, Paths and File are the arguments of the Fix compensations.
	Config *ConfigChange `json:"config,omitempty"`
	Paths  []string      `json:"paths,omitempty"`
	File   *FileBefore   `json:"file,omitempty"`
	// ConfigFile is the argument of RestoreConfigFile.
	ConfigFile *ConfigFileEdit `json:"config_file,omitempty"`
	// Project, Item, Field and Value are the arguments of the project
	// compensations; Value is also the chain line RestoreChainNote writes.
	Project *forge.Project `json:"project,omitempty"`
	Item    int64          `json:"item,omitempty"`
	Field   int64          `json:"field,omitempty"`
	Value   string         `json:"value,omitempty"`
	// Assignee is the login UnassignIssue removes.
	Assignee string `json:"assignee,omitempty"`
	// RemoteName is the remote of DeleteRemoteBranch ("" for the configured
	// one, in operations logged before it was recorded).
	RemoteName string `json:"remote,omitempty"`
	// Repository is the repository of ClosePullRequest as "owner/name" when
	// it is not the remote's (a fork's parent), and ClearMilestone takes the
	// pull request out of its milestone first.
	Repository     string `json:"repository,omitempty"`
	ClearMilestone bool   `json:"clear_milestone,omitempty"`
}

// PlanUndo decides how to revert a delivery, given where the user is now.
//
// Right after a delivery the working tree holds exactly the delivered commit,
// which is the work as it was before. Undoing therefore only moves refs and
// the index back; the files stay. It refuses when the repository moved since
// the operation, so that an undo never discards work made afterwards.
func PlanUndo(op Operation, current RepositoryView) ([]Step, error) {
	if op.Kind == Roadmap {
		return planRoadmapUndo(op), nil
	}
	if op.Kind == Stack || op.Kind == StackPublish || op.Kind == StackSync {
		return planStackUndo(op, current)
	}
	if op.Kind == Fix {
		// Reverse order: the file and the index first, the configuration last.
		var steps []Step
		for _, f := range op.FilesBefore {
			steps = append(steps, Step{Kind: RestoreFile, File: &f})
		}
		if len(op.Untracked) > 0 {
			steps = append(steps, Step{Kind: TrackFiles, Paths: op.Untracked})
		}
		for i := len(op.ConfigChanges) - 1; i >= 0; i-- {
			steps = append(steps, Step{Kind: RestoreConfig, Config: &op.ConfigChanges[i]})
		}
		return steps, nil
	}
	if op.Kind == Rewind {
		// The files as they were, then those the rewind recreated go away.
		if op.Checkpoint == "" {
			return nil, fault.New(fault.NothingToUndo, "undo.nothing")
		}
		steps := []Step{{Kind: RestoreCheckpoint, Checkpoint: op.Checkpoint}}
		for _, f := range op.FilesBefore {
			steps = append(steps, Step{Kind: RestoreFile, File: &f})
		}
		return steps, nil
	}
	if op.Kind == Configure {
		if op.ConfigFile == nil {
			return nil, fault.New(fault.NothingToUndo, "undo.nothing")
		}
		return []Step{{Kind: RestoreConfigFile, ConfigFile: op.ConfigFile}}, nil
	}
	if op.Kind == Tidy {
		// Branch deletions are the only destructive part of a tidy; the
		// switch to the base and the fast-forward lost nothing.
		var steps []Step
		for _, b := range op.DeletedBranches {
			steps = append(steps, Step{Kind: RecreateBranch, Branch: b.Name, Commit: b.Commit})
		}
		return steps, nil
	}
	if op.Kind == Publish {
		// Compensations (docs/design/undo-storage-optimizations.md): close the pull request this publication
		// opened, then delete the branch if nobody pushed to it since. A
		// pull request that was only updated is not ours to rewrite.
		if !op.PullRequestCreated || op.PullRequest == 0 {
			return nil, fault.New(fault.UndoOfPublishedWork, "undo.published", "url", op.PullRequestURL)
		}
		steps := []Step{
			{Kind: ClosePullRequest, Number: op.PullRequest, Repository: op.PullRequestRepository,
				ClearMilestone: op.PullRequestMilestone != 0},
			{Kind: DeleteRemoteBranch, Branch: op.After.Branch, Expected: op.Commit, RemoteName: op.Remote},
		}
		// Then what it changed on the linked issue, in reverse order.
		return append(steps, linkedIssueSteps(op.LinkedIssue)...), nil
	}
	if op.Kind == Rename {
		if current.Branch != op.After.Branch || current.Head != op.After.Head {
			return nil, fault.New(fault.RepositoryMovedSinceOperation, "undo.repository_moved",
				"branch", op.After.Branch, "current", current.Branch)
		}
		return []Step{{Kind: RenameBranch, Branch: op.After.Branch, Target: op.Before.Branch, Expected: op.After.Head}}, nil
	}
	if op.Kind != Deliver {
		return nil, fault.New(fault.NothingToUndo, "undo.nothing")
	}
	if current.Head != op.After.Head || current.Branch != op.After.Branch || current.Detached != op.After.Detached {
		return nil, fault.New(fault.RepositoryMovedSinceOperation, "undo.repository_moved",
			"branch", op.After.Branch, "current", current.Branch)
	}

	var steps []Step
	switch {
	case op.CreatedBranch && op.Before.Detached:
		steps = append(steps, Step{Kind: DetachHead, Commit: op.Before.Head})
	case op.CreatedBranch:
		steps = append(steps, Step{Kind: AttachHead, Branch: op.Before.Branch})
	case op.Before.Head == op.After.Head:
		// Nothing was committed on the branch: no ref to move.
	case op.Before.Head == "":
		steps = append(steps, Step{Kind: DeleteBranch, Branch: op.After.Branch, Expected: op.After.Head})
	default:
		steps = append(steps, Step{Kind: MoveBranch, Branch: op.After.Branch, Commit: op.Before.Head, Expected: op.After.Head})
	}
	steps = append(steps, Step{Kind: ResetIndex, Commit: op.Before.Head})
	// The checkpoint holds the whole work; restoring it after a partial
	// delivery could overwrite the files left out, edited since.
	if op.Checkpoint != "" && len(op.Only) == 0 {
		steps = append(steps, Step{Kind: RestoreCheckpoint, Checkpoint: op.Checkpoint})
	}
	if op.CreatedBranch {
		steps = append(steps, Step{Kind: DeleteBranch, Branch: op.After.Branch, Expected: op.After.Head})
	}
	return steps, nil
}

// linkedIssueSteps put back what a publication changed on its linked issue,
// in reverse order: the assignee it added, the status it set.
func linkedIssueSteps(li *LinkedIssueChange) []Step {
	if li == nil {
		return nil
	}
	var steps []Step
	if li.Assigned != "" {
		steps = append(steps, Step{Kind: UnassignIssue, Number: li.Issue, Assignee: li.Assigned})
	}
	if li.Field != 0 && li.After != "" {
		steps = append(steps, Step{Kind: RestoreProjectField, Project: &li.Project, Number: li.Issue,
			Item: li.Item, Field: li.Field, Value: li.Before, Expected: li.After})
	}
	return steps
}

// planRoadmapUndo reverses a roadmap sync: the file first (local), then the
// links, states and issues it made, and the milestones last, once the issues
// that were in them are closed.
func planRoadmapUndo(op Operation) []Step {
	var steps []Step
	for _, f := range op.FilesBefore {
		steps = append(steps, Step{Kind: RestoreFile, File: &f})
	}
	rs := op.RoadmapSync
	if rs == nil {
		return steps
	}
	// The project first: its items name issues the steps below close.
	if ps := rs.Project; ps != nil {
		for i := len(ps.Values) - 1; i >= 0; i-- {
			v := ps.Values[i]
			steps = append(steps, Step{Kind: RestoreProjectField, Project: &ps.Project, Item: v.Item, Field: v.Field, Value: v.Before})
		}
		for i := len(ps.AddedItems) - 1; i >= 0; i-- {
			steps = append(steps, Step{Kind: RemoveProjectItem, Project: &ps.Project, Item: ps.AddedItems[i]})
		}
	}
	for i := len(rs.Parents) - 1; i >= 0; i-- {
		steps = append(steps, Step{Kind: RemoveSubIssue, Number: rs.Parents[i].Parent, Child: rs.Parents[i].Child})
	}
	for i := len(rs.States) - 1; i >= 0; i-- {
		steps = append(steps, Step{Kind: RestoreIssueState, Number: rs.States[i].Number, WasOpen: rs.States[i].WasOpen})
	}
	inMilestone := map[int]int{}
	for _, is := range rs.CreatedIssues {
		steps = append(steps, Step{Kind: CloseCreatedIssue, Number: is.Number})
		inMilestone[is.Milestone]++
	}
	for _, n := range rs.CreatedMilestones {
		// Deleted only if it holds no more than the issues this sync created.
		steps = append(steps, Step{Kind: DeleteMilestone, Number: n, Issues: inMilestone[n]})
	}
	return steps
}
