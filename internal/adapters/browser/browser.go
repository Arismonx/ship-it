// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

// Package browser implements ports.Browser with the opener of each system,
// run with explicit arguments, never through a shell: rundll32
// url.dll,FileProtocolHandler on Windows, open on macOS, xdg-open elsewhere.
// Only https://github.com/ pages are opened.
package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Opener implements ports.Browser.
type Opener struct {
	GOOS string
	// command runs the opener; tests replace it.
	command func(ctx context.Context, name string, args ...string) error
}

// New returns an Opener for the operating system goos.
func New(goos string) *Opener {
	return &Opener{GOOS: goos, command: func(ctx context.Context, name string, args ...string) error {
		return exec.CommandContext(ctx, name, args...).Start()
	}}
}

// Open implements ports.Browser.
func (o *Opener) Open(ctx context.Context, url string) error {
	if !strings.HasPrefix(url, "https://github.com/") || strings.ContainsAny(url, " \"'\n\r\t") {
		return errors.New("browser: only https://github.com/ pages are opened")
	}
	name, args := o.argv(url)
	return o.command(ctx, name, args...)
}

func (o *Opener) argv(url string) (string, []string) {
	switch o.GOOS {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
