// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AppName is the directory name ship-it uses under standard locations.
const AppName = "ship-it"

// errNoHome is returned when no home directory can be determined.
var errNoHome = errors.New("platform: cannot determine the user home directory")

// Environment abstracts the process environment so paths are testable.
type Environment struct {
	GOOS   string
	Getenv func(string) string
	Home   string
	// PolicyPath replaces the system policy location; tests only.
	PolicyPath string
}

// CurrentEnvironment describes the running process.
func CurrentEnvironment() Environment {
	home, _ := os.UserHomeDir()
	return Environment{GOOS: runtime.GOOS, Getenv: os.Getenv, Home: home}
}

// ConfigDir returns the user configuration directory of ship-it:
// %AppData%\ship-it, ~/Library/Application Support/ship-it, or
// $XDG_CONFIG_HOME/ship-it (default ~/.config/ship-it). It matches
// os.UserConfigDir.
func (e Environment) ConfigDir() (string, error) {
	switch e.GOOS {
	case "windows":
		return e.windowsDir("AppData")
	case "darwin":
		return e.homeJoin("Library", "Application Support", AppName)
	default:
		return e.xdgDir("XDG_CONFIG_HOME", ".config")
	}
}

// CacheDir returns the user cache directory: %LocalAppData%\ship-it\cache,
// ~/Library/Caches/ship-it, or $XDG_CACHE_HOME/ship-it (default
// ~/.cache/ship-it, the XDG location for non-essential data). What is there
// can be deleted at any time and is fetched again.
func (e Environment) CacheDir() (string, error) {
	switch e.GOOS {
	case "windows":
		dir, err := e.windowsDir("LocalAppData")
		return filepath.Join(dir, "cache"), err
	case "darwin":
		return e.homeJoin("Library", "Caches", AppName)
	default:
		return e.xdgDir("XDG_CACHE_HOME", ".cache")
	}
}

// LogDir returns the user log directory used outside a repository:
// %LocalAppData%\ship-it\logs, ~/Library/Logs/ship-it, or
// $XDG_STATE_HOME/ship-it/logs (default ~/.local/state/ship-it/logs), the
// XDG location for "actions history (logs, history…)".
// Inside a repository, logs live in .git/ship/logs (RepositoryLogDir).
func (e Environment) LogDir() (string, error) {
	switch e.GOOS {
	case "windows":
		dir, err := e.windowsDir("LocalAppData")
		return filepath.Join(dir, "logs"), err
	case "darwin":
		return e.homeJoin("Library", "Logs", AppName)
	default:
		dir, err := e.xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state"))
		return filepath.Join(dir, "logs"), err
	}
}

// PolicyFile returns the organisation policy deployed by the IT department,
// in a system location only administrators can write:
// %ProgramData%\ship-it\policy.json,
// /Library/Application Support/ship-it/policy.json (Apple: resources shared
// by all users), or /etc/ship-it/policy.json.
// No environment variable can move it: that would let anyone bypass it.
func (e Environment) PolicyFile() (string, error) {
	if e.PolicyPath != "" {
		return e.PolicyPath, nil
	}
	switch e.GOOS {
	case "windows":
		dir, err := e.windowsDir("ProgramData")
		return filepath.Join(dir, "policy.json"), err
	case "darwin":
		return "/Library/Application Support/" + AppName + "/policy.json", nil
	default:
		return "/etc/" + AppName + "/policy.json", nil
	}
}

// RepositoryLogDir returns the per-repository log directory, inside the git
// directory so that logs never mix across clients (decision N2).
func RepositoryLogDir(gitDir string) string {
	return filepath.Join(gitDir, "ship", "logs")
}

// SaltFile returns the file holding the installation's random salt, which
// keys the repo_hash of the journal: <ConfigDir>/install-salt.
func (e Environment) SaltFile() (string, error) {
	dir, err := e.ConfigDir()
	return filepath.Join(dir, "install-salt"), err
}

// RunLogDir returns where the journal of a run goes: the repository's log
// directory when gitDir is known, the user log directory otherwise.
func (e Environment) RunLogDir(gitDir string) (string, error) {
	if gitDir != "" {
		return RepositoryLogDir(gitDir), nil
	}
	return e.LogDir()
}

// TempDir returns the temporary directory of the target operating system,
// as os.TempDir does: %TMP%/%TEMP% on Windows, $TMPDIR elsewhere.
func (e Environment) TempDir() string {
	if e.GOOS == "windows" {
		for _, v := range []string{"TMP", "TEMP"} {
			if dir := e.Getenv(v); dir != "" {
				return dir
			}
		}
		return ""
	}
	if dir := e.Getenv("TMPDIR"); dir != "" {
		return dir
	}
	return "/tmp"
}

func (e Environment) windowsDir(variable string) (string, error) {
	if base := e.Getenv(variable); base != "" {
		return filepath.Join(base, AppName), nil
	}
	return "", errors.New("platform: %" + variable + "% is not set")
}

// xdgDir applies the XDG rule: use the variable when it is an absolute path,
// otherwise fall back to $HOME/<fallback>.
func (e Environment) xdgDir(variable, fallback string) (string, error) {
	if base := e.Getenv(variable); base != "" && e.isAbs(base) {
		return filepath.Join(base, AppName), nil
	}
	return e.homeJoin(fallback, AppName)
}

// isAbs applies the target operating system's rule rather than the host's,
// so that the environment can be simulated in tests.
func (e Environment) isAbs(p string) bool {
	if e.GOOS == "windows" {
		return filepath.IsAbs(p)
	}
	return strings.HasPrefix(filepath.ToSlash(p), "/")
}

func (e Environment) homeJoin(elem ...string) (string, error) {
	if e.Home == "" {
		return "", errNoHome
	}
	return filepath.Join(append([]string{e.Home}, elem...)...), nil
}
