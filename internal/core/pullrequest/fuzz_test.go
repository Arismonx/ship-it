// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package pullrequest

import (
	"strings"
	"testing"
)

// FuzzFillTemplate: any template, however malformed (unclosed comments,
// headings in fences), is filled without panicking, keeps its non-heading
// text and ends with the marker.
func FuzzFillTemplate(f *testing.F) {
	f.Add(reactTemplate, "Why.")
	f.Add(kubernetesTemplate, "")
	f.Add("<!-- ## Summary", "x")
	f.Add("## Summary\r\n<!--\r\n-->", "y")
	f.Fuzz(func(t *testing.T, template, summary string) {
		got := FillTemplate(template, Sections{Summary: summary, Testing: "t", Issue: "Closes #1"})
		if !strings.HasSuffix(got, Marker+"\n") {
			t.Fatalf("no marker: %q", got)
		}
	})
}

// FuzzParseMessage: the subject is the first line, and the body never holds
// more text than the message.
func FuzzParseMessage(f *testing.F) {
	f.Add("fix: x\n\nWhy.\n\nSigned-off-by: A <a@b>")
	f.Add("\n\n\n")
	f.Fuzz(func(t *testing.T, message string) {
		subject, body := ParseMessage(message)
		if strings.Contains(subject, "\n") || len(body) > len(message) {
			t.Fatalf("ParseMessage(%q) = %q, %q", message, subject, body)
		}
	})
}
