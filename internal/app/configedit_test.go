// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/core/config"
	"github.com/GregoireF/ship-it/internal/core/fault"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/ports"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const commentedRepositoryFile = "{\n  // Our team publishes by hand.\n  \"version\": 1,\n  \"ship\": {\n    \"publish\": {\"mode\": \"never\"} // see ADR-3\n  }\n}\n"

func TestSetSettingEditsTheFileAndUndoPutsItBack(t *testing.T) {
	ctx := context.Background()
	ws, _, log := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{Repository: commentedRepositoryFile}
	ws.Config = files

	result, err := SetSetting(ctx, ws, ConfigEditRequest{Key: "ship.pullRequest.maxLines", Value: "600"})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  // Our team publishes by hand.\n  \"version\": 1,\n  \"ship\": {\n    \"publish\": {\"mode\": \"never\"}, // see ADR-3\n    \"pullRequest\": {\n      \"maxLines\": 600\n    }\n  }\n}\n"
	if files.Repository != want {
		t.Fatalf("file =\n%s\nwant\n%s", files.Repository, want)
	}
	if !result.Changed || result.Scope != "repository" || result.Path != testkit.FakeRepositoryConfigPath || string(result.Value) != "600" || result.Operation == "" {
		t.Fatalf("result = %+v", result)
	}
	op := log.Operations[0]
	if op.Kind != oplog.Configure || op.ConfigFile.Before != commentedRepositoryFile || op.ConfigFile.After != want || !op.ConfigFile.BeforeExisted {
		t.Fatalf("operation = %+v", op)
	}

	// The same value again: nothing written, nothing to undo.
	writes := files.Writes
	if again, err := SetSetting(ctx, ws, ConfigEditRequest{Key: "ship.pullRequest.maxLines", Value: "600"}); err != nil || again.Changed || files.Writes != writes {
		t.Fatalf("same value: %+v, %v", again, err)
	}

	undo, err := Undo(ctx, ws, UndoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if files.Repository != commentedRepositoryFile || undo.ConfigFile != testkit.FakeRepositoryConfigPath || undo.Kind != "config" {
		t.Fatalf("after undo: %q, %+v", files.Repository, undo)
	}
}

func TestUnsetSettingRemovesTheKeyAndItsEmptyParents(t *testing.T) {
	ctx := context.Background()
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{Repository: commentedRepositoryFile}
	ws.Config = files

	result, err := UnsetSetting(ctx, ws, ConfigEditRequest{Key: "ship.publish.mode"})
	if err != nil || !result.Changed {
		t.Fatalf("UnsetSetting = %+v, %v", result, err)
	}
	if want := "{\n  // Our team publishes by hand.\n  \"version\": 1\n}\n"; files.Repository != want {
		t.Fatalf("file = %q, want %q", files.Repository, want)
	}
	if again, err := UnsetSetting(ctx, ws, ConfigEditRequest{Key: "ship.publish.mode"}); err != nil || again.Changed {
		t.Fatalf("second unset = %+v, %v", again, err)
	}
	if _, err := Undo(ctx, ws, UndoRequest{}); err != nil || files.Repository != commentedRepositoryFile {
		t.Fatalf("undo: %v, %q", err, files.Repository)
	}
}

func TestSetSettingCreatesTheFileAndUndoRemovesIt(t *testing.T) {
	ctx := context.Background()
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{}
	ws.Config = files

	result, err := SetSetting(ctx, ws, ConfigEditRequest{Key: "github.labels.areas", Value: "cli, core", Scope: ports.ScopeUser})
	if err != nil || !result.Created || result.Scope != "user" {
		t.Fatalf("SetSetting = %+v, %v", result, err)
	}
	want := "{\n  \"$schema\": \"" + config.SchemaURL + "\",\n  \"version\": 1,\n  \"github\": {\n    \"labels\": {\n      \"areas\": [\"cli\",\"core\"]\n    }\n  }\n}\n"
	if files.User != want || files.Repository != "" {
		t.Fatalf("user file =\n%s\nwant\n%s", files.User, want)
	}
	if _, err := Undo(ctx, ws, UndoRequest{}); err != nil || files.User != "" {
		t.Fatalf("undo: %v, %q", err, files.User)
	}
}

func TestSetSettingRefusals(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		files testkit.FakeConfigFiles
		req   ConfigEditRequest
		code  fault.Code
	}{
		{"unknown key", testkit.FakeConfigFiles{}, ConfigEditRequest{Key: "ship.publsh", Value: "never"}, fault.ConfigUnknownKey},
		{"a section is no key", testkit.FakeConfigFiles{}, ConfigEditRequest{Key: "ship", Value: "{}"}, fault.ConfigUnknownKey},
		{"wrong type", testkit.FakeConfigFiles{}, ConfigEditRequest{Key: "ship.gates.enabled", Value: "maybe"}, fault.ConfigWrongType},
		{"not allowed", testkit.FakeConfigFiles{}, ConfigEditRequest{Key: "ship.publish.mode", Value: "sometimes"}, fault.ConfigInvalidValue},
		{"below the minimum", testkit.FakeConfigFiles{}, ConfigEditRequest{Key: "ship.pullRequest.maxLines", Value: "0"}, fault.ConfigInvalidValue},
		{"broken file", testkit.FakeConfigFiles{Repository: `{"ship": `}, ConfigEditRequest{Key: "ship.publish.mode", Value: "never"}, fault.ConfigInvalidJSON},
		{"imposed by the policy", testkit.FakeConfigFiles{Policy: `{"organization":"Acme","enforce":{"ship":{"publish":{"mode":"auto"}}}}`},
			ConfigEditRequest{Key: "ship.publish.mode", Value: "never"}, fault.PolicyConflict},
		{"no repository", testkit.FakeConfigFiles{NoRepository: true}, ConfigEditRequest{Key: "ship.publish.mode", Value: "never", Scope: ports.ScopeRepository}, fault.NotARepository},
		{"personal setting in the repository file", testkit.FakeConfigFiles{},
			ConfigEditRequest{Key: "ui.color", Value: "never", Scope: ports.ScopeRepository}, fault.ConfigUnknownKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws, _, log := workspaceOf(testkit.HealthyRepository("main"))
			files := tt.files
			ws.Config = &files
			_, err := SetSetting(ctx, ws, tt.req)
			if !errors.Is(err, &fault.Fault{Code: tt.code}) {
				t.Fatalf("error = %v, want %s", err, tt.code)
			}
			if files.Writes != 0 || len(log.Operations) != 0 {
				t.Fatalf("something was written: %d files, %v", files.Writes, log.Operations)
			}
		})
	}
}

// A personal setting goes to the user file by default, even in a repository.
func TestSetSettingOfTheInterfaceGoesToTheUserFile(t *testing.T) {
	ctx := context.Background()
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{Repository: commentedRepositoryFile}
	ws.Config = files
	result, err := SetSetting(ctx, ws, ConfigEditRequest{Key: "ui.color", Value: "never"})
	if err != nil || result.Scope != "user" || !strings.Contains(files.User, `"color": "never"`) || files.Repository != commentedRepositoryFile {
		t.Fatalf("SetSetting = %+v, %v; user file %q", result, err, files.User)
	}
}

func TestSetSettingOutsideARepositoryIsNotUndoable(t *testing.T) {
	ws, _, log := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{NoRepository: true}
	ws.Config = files
	result, err := SetSetting(context.Background(), ws, ConfigEditRequest{Key: "ship.publish.mode", Value: "never"})
	if err != nil || result.Scope != "user" || result.Operation != "" || len(log.Operations) != 0 || !strings.Contains(files.User, `"mode": "never"`) {
		t.Fatalf("SetSetting = %+v, %v; user file %q", result, err, files.User)
	}
}

func TestUndoOfAnEditKeepsALaterChange(t *testing.T) {
	ctx := context.Background()
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	files := &testkit.FakeConfigFiles{Repository: commentedRepositoryFile}
	ws.Config = files
	if _, err := SetSetting(ctx, ws, ConfigEditRequest{Key: "ship.publish.mode", Value: "auto"}); err != nil {
		t.Fatal(err)
	}
	files.Repository = `{"ship": {"publish": {"mode": "never"}}}` // edited by hand since
	_, err := Undo(ctx, ws, UndoRequest{})
	if !errors.Is(err, &fault.Fault{Code: fault.ConfigFileChanged}) || !NeedsHuman(err) {
		t.Fatalf("error = %v", err)
	}
	if files.Repository != `{"ship": {"publish": {"mode": "never"}}}` {
		t.Fatalf("the later change was overwritten: %q", files.Repository)
	}
}

func TestGetAndExplainSetting(t *testing.T) {
	ctx := context.Background()
	ws, _, _ := workspaceOf(testkit.HealthyRepository("main"))
	ws.Config = &testkit.FakeConfigFiles{User: `{"ship": {"publish": {"mode": "never"}}}`, Repository: `{"ship": {"publish": {"mode": "auto"}}}`}

	value, err := GetSetting(ctx, ws, "ship.publish.mode")
	if err != nil || string(value.Value) != `"auto"` || value.Source != "repository" || value.Path != testkit.FakeRepositoryConfigPath {
		t.Fatalf("GetSetting = %+v, %v", value, err)
	}
	x, err := ExplainSetting(ctx, ws, "ship.publish.mode")
	if err != nil || len(x.Layers) != 2 || x.Layers[0].Source != "user" || x.Layers[0].Value != `"never"` || x.Env != "SHIP_PUBLISH_MODE" {
		t.Fatalf("ExplainSetting = %+v, %v", x, err)
	}
	var f *fault.Fault
	if _, err := GetSetting(ctx, ws, "ship.publish.mod"); !errors.As(err, &f) || f.Params["suggestion"] != "ship.publish.mode" {
		t.Fatalf("unknown key: %v", err)
	}
}
