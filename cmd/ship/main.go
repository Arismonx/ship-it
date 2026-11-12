// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Command ship is the composition root: it builds the adapters, injects them
// into the app use cases and starts the requested surface (cli, rpc, mcp, daemon).
//
// Nothing else may import this package.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/GregoireF/ship-it/internal/adapters/browser"
	"github.com/GregoireF/ship-it/internal/adapters/checkpointstore"
	"github.com/GregoireF/ship-it/internal/adapters/footprint"
	"github.com/GregoireF/ship-it/internal/adapters/fsconfig"
	"github.com/GregoireF/ship-it/internal/adapters/ghauth"
	"github.com/GregoireF/ship-it/internal/adapters/gitexec"
	"github.com/GregoireF/ship-it/internal/adapters/github"
	"github.com/GregoireF/ship-it/internal/adapters/oplogstore"
	"github.com/GregoireF/ship-it/internal/adapters/projectexec"
	"github.com/GregoireF/ship-it/internal/adapters/roadmapstore"
	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/i18n"
	"github.com/GregoireF/ship-it/internal/obs"
	"github.com/GregoireF/ship-it/internal/platform"
	"github.com/GregoireF/ship-it/internal/surface/cli"
	"github.com/GregoireF/ship-it/internal/surface/mcp"
)

// Set at link time: -ldflags "-X main.version=… -X main.commit=… -X main.date=…".
// Left empty (go install, go build), they come from the build information
// the go command records in the binary (see buildInfo).
var (
	version = ""
	commit  = ""
	date    = ""
)

func main() {
	version, commit, date = buildInfo(version, commit, date, debug.ReadBuildInfo)
	// Ctrl+C cancels the context so that running git commands stop cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	git := gitexec.New(".")
	journal := openJournal(ctx, git, platform.CurrentEnvironment())
	ws := workspace(git, journal)
	args := os.Args[1:]

	// `ship mcp` serves agents on stdin/stdout: nothing else may write there.
	// --attended is for the VS Code extension, where a human reviews.
	if attended, client, ok := mcpServeArgs(args); ok {
		limits := app.RunLogLimits(ctx, ws)
		journal.SetLimits(obs.Retention{Runs: limits.KeepRuns, Bytes: limits.MaxBytes}, limits.BlackBoxBytes)
		server := &mcp.Server{Workspace: ws, Version: version, Log: os.Stderr, Attended: attended, Client: client, Journal: journal}
		journal.Logger.Info("run.start", "command", "mcp", "version", version, "os", runtime.GOOS, "arch", runtime.GOARCH)
		err := server.Serve(ctx, os.Stdin, os.Stdout)
		stop()
		journal.Logger.Info("run.end", "command", "mcp", obs.KeyDurationMS, journal.Elapsed().Milliseconds())
		journal.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ship mcp:", err)
			os.Exit(cli.ExitError)
		}
		os.Exit(cli.ExitOK)
	}

	code := cli.Run(
		ctx,
		args,
		cli.Streams{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Interactive: isTerminal(os.Stdin), TerminalUI: isTerminal(os.Stderr), Journal: journal},
		uiLanguage(),
		cli.BuildInfo{Version: version, Commit: commit, Date: date},
		ws,
	)
	stop()
	journal.Close()
	os.Exit(code)
}

// openJournal starts the run's technical journal (docs/reference/logs.md):
// in .git/ship/logs inside a repository, in the user log directory
// otherwise. Paths of the repository, the home and the temporary directory
// are replaced by ".", "~" and "$TMP" in every record. Logging never stops
// the work: when the directory cannot be written, the journal stays in
// memory.
func openJournal(ctx context.Context, git *gitexec.Client, env platform.Environment) *obs.Journal {
	top, _ := git.TopLevel(ctx)
	gitDir, _ := git.GitDir(ctx)
	aliases := map[string]string{env.Home: "~", env.TempDir(): "$TMP"}
	if top != "" {
		aliases[top] = "."
	}
	if gitDir != "" {
		aliases[gitDir] = "$GIT_DIR" // a linked worktree's git directory is elsewhere
	}
	journal := obs.NewJournal(obs.JournalOptions{Scrubber: obs.NewScrubber(aliases)})
	// Without a salt there is no repo_hash: never an unsalted, guessable one.
	if saltFile, err := env.SaltFile(); err == nil {
		if salt, err := obs.InstallSalt(saltFile); err == nil {
			journal.SetRepository(obs.RepoHash(salt, top))
		}
	}
	dir, err := env.RunLogDir(gitDir)
	if err == nil {
		err = journal.Attach(dir)
	}
	if err != nil {
		journal.Discard()
	}
	git.Log = journal.Logger
	return journal
}

// workspace wires every adapter to its port, for every surface.
func workspace(git *gitexec.Client, journal *obs.Journal) app.Workspace {
	credentials := ghauth.New(git)
	forge := github.New(credentials, "ship-it/"+version)
	forge.Log = journal.Logger
	ws := app.Workspace{
		Git:             git,
		Checkpoints:     checkpointstore.New(git),
		Operations:      oplogstore.New(git),
		Forge:           forge,
		Stacks:          forge,
		Tracker:         forge,
		Projects:        forge,
		RoadmapMemory:   roadmapstore.New(git),
		Credentials:     credentials,
		Config:          fsconfig.New(git),
		Runner:          projectexec.New(git),
		OperatingSystem: runtime.GOOS,
		Log:             journal.Logger,
		Environ:         os.Environ(),
	}
	if home, err := os.UserHomeDir(); err == nil {
		ws.Home = home
	}
	if journal.Dir() != "" {
		ws.RunLogs = journal
	}
	ws.Footprint = footprint.New(git, platform.CurrentEnvironment())
	ws.Browser = browser.New(runtime.GOOS)
	return ws
}

// buildInfo completes what the link did not set: the module version (a
// tagged go install gives v0.1.0, a go build of a checkout "(devel)") and the
// vcs.revision and vcs.time the go command stamps from the checkout.
// Whatever stays unknown reads "dev", "none" and "unknown".
func buildInfo(version, commit, date string, read func() (*debug.BuildInfo, bool)) (string, string, string) {
	if info, ok := read(); ok && info != nil {
		if version == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && commit == "":
				commit = s.Value
			case s.Key == "vcs.time" && date == "":
				date = s.Value
			}
		}
	}
	if version == "" {
		version = "dev"
	}
	if commit == "" {
		commit = "none"
	}
	if date == "" {
		date = "unknown"
	}
	return version, commit, date
}

// isTerminal reports whether f is a character device (a terminal), so that
// ship never waits for an answer nobody can give.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// uiLanguage reads SHIP_LANG, then the POSIX locale variables: the language
// of the messages written before the settings are read. cli.Run then puts
// ui.language between the two (SHIP_LANG > ui.language > locale).
func uiLanguage() i18n.Language {
	for _, name := range []string{"SHIP_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" {
			return i18n.ParseLanguage(v)
		}
	}
	return i18n.DefaultLanguage
}

// mcpServeArgs reads `ship mcp [--attended] [--client <name>]`: the server
// on stdin/stdout. Anything else (ship mcp install…) is the CLI's.
func mcpServeArgs(args []string) (attended bool, client string, ok bool) {
	if len(args) == 0 || args[0] != "mcp" {
		return false, "", false
	}
	for i := 1; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--attended":
			attended = true
		case a == "--client" && i+1 < len(args):
			i++
			client = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(a, "--client="):
			client = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(a, "--client=")))
		default:
			return false, "", false
		}
	}
	return attended, client, true
}
