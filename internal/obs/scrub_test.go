// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// fakeToken is assembled at run time so that ship-it's secret scan, run on
// its own code, does not flag it.
var fakeToken = strings.Join([]string{"gh", "p_0123456789abcdefghijklmnopqrstuvwxyz"}, "")

func TestScrubString(t *testing.T) {
	home := filepath.FromSlash("/home/dev")
	repo := filepath.Join(home, "src", "shop")
	s := NewScrubber(map[string]string{repo: ".", home: "~"})
	tests := []struct{ in, want string }{
		{"fatal: cannot open " + filepath.Join(repo, "main.go"), "fatal: cannot open ." + string(filepath.Separator) + "main.go"},
		{"config at " + filepath.Join(home, ".gitconfig"), "config at ~" + string(filepath.Separator) + ".gitconfig"},
		{"author dev@example.com", "author " + RedactedEmail},
		{"https://bob:s3cret@github.com/acme/shop.git", "https://" + Redacted + "@github.com/acme/shop.git"},
		{"https://github.com/acme/shop.git", "https://github.com/acme/shop.git"},
		{"Authorization: Bearer abcdefgh12345678", "Authorization: Bearer " + Redacted},
		{"token " + fakeToken, "token " + Redacted},
		{"'C:\\Users\\dev\\AppData\\x' failed", "'" + RedactedPath + "' failed"},
		{"cd /Users/dev/work", "cd " + RedactedPath},
		{"exit status 1", "exit status 1"},
		{"refs/heads/feat/42-login", "refs/heads/feat/42-login"},
	}
	for _, tt := range tests {
		if got := s.String(tt.in); got != tt.want {
			t.Errorf("String(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestJournalScrubsEveryValue(t *testing.T) {
	var file bytes.Buffer
	home := filepath.FromSlash("/home/dev")
	j := NewJournal(JournalOptions{File: &file, Scrubber: NewScrubber(map[string]string{home: "~"})})
	logger := j.Logger.With("remote", "https://u:pw123456@example.com/r.git")
	logger.Info("github.request",
		"error", errors.New("open "+filepath.Join(home, "x")+": denied"),
		"args", []string{"config", "user.email", "dev@example.com"},
		slog.Group("http", "authorization", "Bearer "+fakeToken),
		"token", Secret(fakeToken),
		"value", struct{ Email string }{"dev@example.com"},
	)
	out := file.String()
	for _, leak := range []string{"pw123456", "dev@example.com", fakeToken, home} {
		if strings.Contains(out, leak) {
			t.Errorf("%q leaked into the journal: %s", leak, out)
		}
	}
}

// FuzzScrub checks that private values never survive scrubbing, whatever
// surrounds them.
func FuzzScrub(f *testing.F) {
	f.Add("fatal: ", " not found")
	f.Add("remote=", "")
	f.Add("\"", "\"")
	f.Add("https://x", "@y")
	home := filepath.FromSlash("/home/dev")
	s := NewScrubber(map[string]string{home: "~"})
	secrets := []string{
		"dev@example.com",
		fakeToken,
		"https://bob:hunter2hunter2@github.com/acme/shop.git",
		filepath.Join(home, "notes.txt"),
	}
	hidden := []string{"dev@example.com", fakeToken, "hunter2hunter2", home}
	f.Fuzz(func(t *testing.T, before, after string) {
		for i, secret := range secrets {
			out := s.String(before + " " + secret + " " + after)
			if strings.Contains(out, hidden[i]) && !strings.Contains(before+after, hidden[i]) {
				t.Fatalf("String(%q) = %q leaks %q", before+" "+secret+" "+after, out, hidden[i])
			}
		}
	})
}
