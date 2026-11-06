// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package checkpointstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A user who signs every commit (commit.gpgSign in the global
// configuration) still gets checkpoints: they are never signed, so no
// signer, key or passphrase is involved.
func TestCheckpointsAreNeverSigned(t *testing.T) {
	c := newRepo(t)
	global := filepath.Join(t.TempDir(), "gitconfig")
	config := "[user]\n\tname = ship-it test\n\temail = test@example.invalid\n" +
		"[commit]\n\tgpgsign = true\n[gpg]\n\tformat = openpgp\n\tprogram = ship-it-no-such-gpg\n"
	if err := os.WriteFile(global, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	if _, err := New(c).Create(context.Background(), "deliver"); err != nil {
		t.Fatalf("Create with a signer that fails: %v", err)
	}
}
