// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package browser

import (
	"context"
	"strings"
	"testing"
)

func TestOpen(t *testing.T) {
	const url = "https://github.com/GregoireF/ship-it/issues/new?template=1-bug-report.yml&title=x%20y"
	for goos, want := range map[string]string{
		"windows": "rundll32 url.dll,FileProtocolHandler " + url,
		"darwin":  "open " + url,
		"linux":   "xdg-open " + url,
	} {
		var got string
		o := &Opener{GOOS: goos, command: func(_ context.Context, name string, args ...string) error {
			got = strings.Join(append([]string{name}, args...), " ")
			return nil
		}}
		if err := o.Open(context.Background(), url); err != nil || got != want {
			t.Errorf("%s: %q, %v", goos, got, err)
		}
	}
	o := &Opener{GOOS: "linux", command: func(context.Context, string, ...string) error {
		t.Error("opened a page that is not GitHub's")
		return nil
	}}
	for _, bad := range []string{"file:///etc/passwd", "https://example.com/", "https://github.com/a b"} {
		if err := o.Open(context.Background(), bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}
