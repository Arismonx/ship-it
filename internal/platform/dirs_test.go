// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package platform

import (
	"path/filepath"
	"testing"
)

func env(goos string, vars map[string]string) Environment {
	return Environment{GOOS: goos, Home: filepath.FromSlash("/home/dev"), Getenv: func(k string) string { return vars[k] }}
}

func TestLogDir(t *testing.T) {
	tests := []struct {
		name string
		e    Environment
		want string
	}{
		{"linux default", env("linux", nil), "/home/dev/.local/state/ship-it/logs"},
		{"linux xdg", env("linux", map[string]string{"XDG_STATE_HOME": filepath.FromSlash("/var/state")}), "/var/state/ship-it/logs"},
		{"linux relative xdg ignored", env("linux", map[string]string{"XDG_STATE_HOME": "rel"}), "/home/dev/.local/state/ship-it/logs"},
		{"darwin", env("darwin", nil), "/home/dev/Library/Logs/ship-it"},
		{"windows", env("windows", map[string]string{"LocalAppData": filepath.FromSlash("/AppData/Local")}), "/AppData/Local/ship-it/logs"},
	}
	for _, tt := range tests {
		got, err := tt.e.LogDir()
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got != filepath.FromSlash(tt.want) {
			t.Errorf("%s: LogDir() = %q, want %q", tt.name, got, filepath.FromSlash(tt.want))
		}
	}
}

func TestCacheDir(t *testing.T) {
	for _, tt := range []struct {
		name string
		e    Environment
		want string
	}{
		{"linux default", env("linux", nil), "/home/dev/.cache/ship-it"},
		{"linux xdg", env("linux", map[string]string{"XDG_CACHE_HOME": filepath.FromSlash("/var/cache")}), "/var/cache/ship-it"},
		{"darwin", env("darwin", nil), "/home/dev/Library/Caches/ship-it"},
		{"windows", env("windows", map[string]string{"LocalAppData": filepath.FromSlash("/AppData/Local")}), "/AppData/Local/ship-it/cache"},
	} {
		if got, err := tt.e.CacheDir(); err != nil || got != filepath.FromSlash(tt.want) {
			t.Errorf("%s: CacheDir() = %q, %v; want %q", tt.name, got, err, filepath.FromSlash(tt.want))
		}
	}
}

func TestConfigDir(t *testing.T) {
	got, err := env("linux", nil).ConfigDir()
	if err != nil || got != filepath.FromSlash("/home/dev/.config/ship-it") {
		t.Fatalf("ConfigDir() = %q, %v", got, err)
	}
	if _, err := env("windows", nil).ConfigDir(); err == nil {
		t.Fatal("ConfigDir() on Windows without %AppData% must fail")
	}
}

func TestPolicyFile(t *testing.T) {
	for _, tt := range []struct {
		e    Environment
		want string
	}{
		{env("linux", nil), "/etc/ship-it/policy.json"},
		{env("darwin", nil), "/Library/Application Support/ship-it/policy.json"},
		{env("windows", map[string]string{"ProgramData": filepath.FromSlash("/ProgramData")}), filepath.FromSlash("/ProgramData/ship-it/policy.json")},
	} {
		if got, err := tt.e.PolicyFile(); err != nil || got != tt.want {
			t.Errorf("%s: PolicyFile() = %q, %v", tt.e.GOOS, got, err)
		}
	}
}

func TestSaltFile(t *testing.T) {
	for _, tt := range []struct {
		e    Environment
		want string
	}{
		{env("linux", nil), "/home/dev/.config/ship-it/install-salt"},
		{env("linux", map[string]string{"XDG_CONFIG_HOME": filepath.FromSlash("/cfg")}), "/cfg/ship-it/install-salt"},
		{env("darwin", nil), "/home/dev/Library/Application Support/ship-it/install-salt"},
		{env("windows", map[string]string{"AppData": filepath.FromSlash("/AppData/Roaming")}), "/AppData/Roaming/ship-it/install-salt"},
	} {
		if got, err := tt.e.SaltFile(); err != nil || got != filepath.FromSlash(tt.want) {
			t.Errorf("%s: SaltFile() = %q, %v; want %q", tt.e.GOOS, got, err, filepath.FromSlash(tt.want))
		}
	}
	if _, err := env("windows", nil).SaltFile(); err == nil {
		t.Error("SaltFile() on Windows without %AppData% must fail")
	}
}

func TestRunLogDir(t *testing.T) {
	for _, tt := range []struct {
		e      Environment
		gitDir string
		want   string
	}{
		{env("linux", nil), "/repo/.git", "/repo/.git/ship/logs"},
		{env("linux", nil), "", "/home/dev/.local/state/ship-it/logs"},
		{env("darwin", nil), "", "/home/dev/Library/Logs/ship-it"},
		{env("windows", map[string]string{"LocalAppData": filepath.FromSlash("/AppData/Local")}), "", "/AppData/Local/ship-it/logs"},
		{env("windows", nil), filepath.FromSlash("/repo/.git"), "/repo/.git/ship/logs"},
	} {
		got, err := tt.e.RunLogDir(filepath.FromSlash(tt.gitDir))
		if err != nil || got != filepath.FromSlash(tt.want) {
			t.Errorf("%s %q: RunLogDir() = %q, %v; want %q", tt.e.GOOS, tt.gitDir, got, err, tt.want)
		}
	}
	if _, err := env("windows", nil).RunLogDir(""); err == nil {
		t.Error("RunLogDir() on Windows without %LocalAppData% must fail")
	}
}

func TestTempDir(t *testing.T) {
	for _, tt := range []struct {
		e    Environment
		want string
	}{
		{env("linux", nil), "/tmp"},
		{env("darwin", map[string]string{"TMPDIR": "/var/folders/x/T/"}), "/var/folders/x/T/"},
		{env("windows", map[string]string{"TEMP": `C:\T`}), `C:\T`},
		{env("windows", map[string]string{"TMP": `C:\A`, "TEMP": `C:\B`}), `C:\A`},
	} {
		if got := tt.e.TempDir(); got != tt.want {
			t.Errorf("%s: TempDir() = %q, want %q", tt.e.GOOS, got, tt.want)
		}
	}
}

func TestRepositoryLogDir(t *testing.T) {
	if got := RepositoryLogDir(filepath.FromSlash("/repo/.git")); got != filepath.FromSlash("/repo/.git/ship/logs") {
		t.Fatalf("RepositoryLogDir() = %q", got)
	}
}
