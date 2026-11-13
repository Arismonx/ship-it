// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/GregoireF/ship-it/internal/testkit"
)

// TestInstallSkillThenUndo: #50. Both folders by default, nothing the
// second time, an older SKILL.md of ship-it's updated, and one ship undo
// removes what was created.
func TestInstallSkillThenUndo(t *testing.T) {
	var calls []string
	git := testkit.HealthyRepository("main")
	git.Calls = &calls
	git.WorkingFiles = map[string]string{}
	ws, _, log := workspaceOf(git)

	done, err := InstallSkill(context.Background(), ws, "", true)
	if err != nil || !done.Applied || len(done.Files) != 2 || !done.Files[0].Created || !done.Files[1].Created || len(log.Operations) != 1 {
		t.Fatalf("install = %+v, %v", done, err)
	}
	if again, _ := InstallSkill(context.Background(), ws, "", true); again.Changed || len(log.Operations) != 1 {
		t.Errorf("a second install must change nothing: %+v", again)
	}
	calls = nil
	if _, err := Undo(context.Background(), ws, UndoRequest{}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(calls)
	if want := []string{"remove .agents/skills/ship-it/SKILL.md", "remove .claude/skills/ship-it/SKILL.md"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("undo calls = %q, want %q", calls, want)
	}

	// An older skill of ship-it's is updated; the user's own is kept.
	git.WorkingFiles[".claude/skills/ship-it/SKILL.md"] = "---\nname: ship-it\n---\n<!-- ship-it skill, written by `ship skill install`; v0 -->\n"
	git.WorkingFiles[".agents/skills/ship-it/SKILL.md"] = "---\nname: ship-it\ndescription: my own\n---\n"
	plan, err := InstallSkill(context.Background(), ws, "", false)
	if err != nil || !plan.Changed || plan.Files[0].Kept != true || plan.Files[1].Updated != true {
		t.Errorf("plan = %+v, %v", plan, err)
	}
}

func TestInstallMCPWithTheSkillIsOneOperation(t *testing.T) {
	git := testkit.HealthyRepository("main")
	git.WorkingFiles = map[string]string{}
	ws, _, log := workspaceOf(git)
	done, err := InstallMCPWith(context.Background(), ws, "claude", true, true)
	if err != nil || !done.Applied || done.Skill == nil || !done.Skill.Applied || len(log.Operations) != 1 ||
		len(log.Operations[0].FilesBefore) != 2 {
		t.Fatalf("install = %+v, %v, operations %+v", done, err, log.Operations)
	}
}
