// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GregoireF/ship-it/tools/internal/pins"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// registry serves GitHub "latest release" and PyPI JSON answers.
func registry(t *testing.T, github map[string]string, pypi map[string]string, wantToken string) *client {
	t.Helper()
	mux := http.NewServeMux()
	for repo, body := range github {
		mux.HandleFunc("GET /gh/repos/"+repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != wantToken {
				t.Errorf("Authorization = %q, want %q", got, wantToken)
			}
			w.Write([]byte(body))
		})
	}
	for name, body := range pypi {
		mux.HandleFunc("GET /pypi/pypi/"+name+"/json", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Error("the GitHub token was sent to PyPI")
			}
			w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	token := strings.TrimPrefix(wantToken, "Bearer ")
	return &client{http: srv.Client(), github: srv.URL + "/gh", pypi: srv.URL + "/pypi", token: token}
}

func ghRelease(tag string, published time.Time) string {
	return `{"tag_name":"` + tag + `","published_at":"` + published.Format(time.RFC3339) + `"}`
}

func TestCheck(t *testing.T) {
	old := now.Add(-30 * 24 * time.Hour)
	c := registry(t, map[string]string{
		"rhysd/actionlint":      ghRelease("v1.7.12", old),                // same
		"goreleaser/goreleaser": ghRelease("v2.20.0", old),                // behind
		"sigstore/cosign":       ghRelease("v3.2.0", now.Add(-time.Hour)), // in cooldown
		"docker/buildx":         ghRelease("v0.38.0", old),
	}, map[string]string{
		"zizmor": `{"info":{"version":"1.32.0"},"urls":[{"upload_time_iso_8601":"2026-10-01T10:00:00Z"}]}`,
	}, "Bearer secret")

	var out bytes.Buffer
	code, err := run(context.Background(), nil, &out, c, now)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	for _, want := range []string{
		"zizmor      1.31.1   1.32.0   behind",
		"actionlint  v1.7.12  v1.7.12  up to date",
		"goreleaser  v2.19.0  v2.20.0  behind",
		"cosign      v3.1.3   v3.2.0   newer, in cooldown until 2026-10-30",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestUpToDateAndMarkdown(t *testing.T) {
	old := now.Add(-30 * 24 * time.Hour)
	github := map[string]string{}
	pypi := map[string]string{}
	for _, tool := range pins.Tools {
		if tool.Source == pins.GitHub {
			github[tool.Repo] = ghRelease(tool.Version, old)
		} else {
			pypi[tool.Repo] = `{"info":{"version":"` + tool.Version + `"},"urls":[]}`
		}
	}
	c := registry(t, github, pypi, "") // no GH_TOKEN: no Authorization header
	var out bytes.Buffer
	code, err := run(context.Background(), []string{"-format=markdown"}, &out, c, now)
	if err != nil || code != 0 {
		t.Fatalf("code = %d, err = %v", code, err)
	}
	if !strings.Contains(out.String(), "| zizmor | 1.31.1 | 1.31.1 | up to date |") {
		t.Errorf("markdown:\n%s", out.String())
	}
}

func TestRegistryError(t *testing.T) {
	c := registry(t, nil, nil, "") // every request answers 404
	code, err := run(context.Background(), nil, &bytes.Buffer{}, c, now)
	if code != 2 || err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("code = %d, err = %v; want 2 and a 404", code, err)
	}
}

func TestIsNewer(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"v1.2.4", "v1.2.3", true},
		{"1.10.0", "1.9.9", true},
		{"v2.0.0", "v2.0.0", false},
		{"v1.9.0", "v2.0.0", false},
		{"v3.0.0-rc.1", "v2.0.0", false}, // pre-releases never count
	} {
		if got, err := isNewer(tc.a, tc.b); err != nil || got != tc.want {
			t.Errorf("isNewer(%s, %s) = %v, %v; want %v", tc.a, tc.b, got, err, tc.want)
		}
	}
	if _, err := isNewer("latest", "v1.0.0"); err == nil {
		t.Error("isNewer accepted a version that is not X.Y.Z")
	}
}
