// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package convention

import (
	"strings"
	"testing"
)

func TestCommitMessage(t *testing.T) {
	for _, tt := range []struct{ why, tested, want string }{
		{"", "", "✨ feat: x\n"},
		{"The EU changed its VAT rules.", "", "✨ feat: x\n\nThe EU changed its VAT rules.\n"},
		{"", "  invoices\nby hand ", "✨ feat: x\n\nTested: invoices by hand\n"},
		{"First.\r\n\r\nSecond.", "npm test", "✨ feat: x\n\nFirst.\n\nSecond.\n\nTested: npm test\n"},
	} {
		if got := CommitMessage("✨ feat: x", tt.why, tt.tested); got != tt.want {
			t.Errorf("CommitMessage(%q, %q) = %q, want %q", tt.why, tt.tested, got, tt.want)
		}
	}
}

func TestWrap(t *testing.T) {
	long := strings.Repeat("word ", 30)
	for _, line := range strings.Split(Wrap(long, DefaultBodyWidth), "\n") {
		if len(line) > DefaultBodyWidth {
			t.Errorf("line of %d characters: %q", len(line), line)
		}
	}
	url := "https://example.com/" + strings.Repeat("x", 80)
	if got := Wrap("see "+url, DefaultBodyWidth); got != "see\n"+url {
		t.Errorf("a long word stays whole: %q", got)
	}
}
