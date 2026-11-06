// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInstallSaltIsCreatedOnceAndKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ship-it", "install-salt")
	first, err := InstallSalt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != SaltSize || bytes.Equal(first, make([]byte, SaltSize)) {
		t.Fatalf("salt = %x", first)
	}
	again, err := InstallSalt(path)
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("second read = %x, %v; want the stored salt", again, err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestConcurrentRunsAgreeOnTheSalt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-salt")
	const runs = 16
	salts := make([][]byte, runs)
	errs := make([]error, runs)
	var wg sync.WaitGroup
	for i := range runs {
		wg.Go(func() { salts[i], errs[i] = InstallSalt(path) })
	}
	wg.Wait()
	for i := range runs {
		if errs[i] != nil || !bytes.Equal(salts[i], salts[0]) {
			t.Fatalf("run %d: salt %x, %v; want %x", i, salts[i], errs[i], salts[0])
		}
	}
}

func TestStaleSaltIsReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install-salt")
	if err := os.WriteFile(path, []byte("half"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	salt, err := InstallSalt(path)
	if err != nil || len(salt) != SaltSize {
		t.Fatalf("salt = %x, %v", salt, err)
	}
}

func TestRepoHash(t *testing.T) {
	a, b := bytes.Repeat([]byte{1}, SaltSize), bytes.Repeat([]byte{2}, SaltSize)
	path := filepath.FromSlash("/home/dev/src/acme-shop")
	h := RepoHash(a, path)
	if h != RepoHash(a, path) {
		t.Error("the hash must be stable for one salt")
	}
	if h == RepoHash(b, path) {
		t.Error("two installations must not share hashes")
	}
	if h == RepoHash(a, path+"2") {
		t.Error("two repositories must not share a hash")
	}
	if !strings.HasPrefix(h, "hmac-sha256:") || len(h) != len("hmac-sha256:")+16 || strings.Contains(h, "acme") || strings.Contains(h, "dev") {
		t.Errorf("RepoHash = %q", h)
	}
	if RepoHash(nil, path) != "" || RepoHash(a, "") != "" {
		t.Error("no salt or no path: no hash, never the raw path")
	}
}
