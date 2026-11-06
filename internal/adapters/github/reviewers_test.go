// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestRequestReviewers(t *testing.T) {
	calls := 0
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/repos/acme/shop/pulls/7/requested_reviewers" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var in map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(in, map[string][]string{"reviewers": {"ada"}, "team_reviewers": {}}) {
			t.Errorf("body = %v", in)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number": 7}`))
	})
	if err := c.RequestReviewers(context.Background(), repo, 7, []string{"ada"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.RequestReviewers(context.Background(), repo, 7, nil, nil); err != nil || calls != 1 {
		t.Errorf("an empty request must not reach GitHub: %v, %d calls", err, calls)
	}
}

func TestRequestReviewersRefused(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Reviews may only be requested from collaborators."}`))
	})
	if err := c.RequestReviewers(context.Background(), repo, 7, []string{"stranger"}, nil); err == nil {
		t.Fatal("a refusal must be an error")
	}
}
