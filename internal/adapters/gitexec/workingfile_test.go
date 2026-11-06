// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package gitexec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkingFilesCreateAndRemoveTheirDirectories(t *testing.T) {
	c := newRepo(t)
	ctx := context.Background()
	if err := c.WriteWorkingFile(ctx, ".vscode/mcp.json", []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if data, err := c.ReadWorkingFile(ctx, ".vscode/mcp.json", 100); err != nil || string(data) != "{}\n" {
		t.Fatalf("read back %q, %v", data, err)
	}
	if err := c.RemoveWorkingFile(ctx, ".vscode/mcp.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, ".vscode")); !os.IsNotExist(err) {
		t.Errorf("the emptied .vscode must go: %v", err)
	}
	if _, err := os.Stat(c.Dir); err != nil {
		t.Errorf("the top level must stay: %v", err)
	}

	// A directory that still holds the user's files stays.
	write(t, c.Dir, ".cursor/rules.md", "mine\n")
	if err := c.WriteWorkingFile(ctx, ".cursor/mcp.json", []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveWorkingFile(ctx, ".cursor/mcp.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, ".cursor", "rules.md")); err != nil {
		t.Errorf("the user's file must stay: %v", err)
	}
}
