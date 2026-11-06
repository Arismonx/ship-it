// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/adapters/checkpointstore"
	"github.com/GregoireF/ship-it/internal/adapters/oplogstore"
	"github.com/GregoireF/ship-it/internal/app"
)

// TestADeliveryAfterAReviewReadsTheFilesAgain: the plan's reads spare the
// delivery that follows at once a second `git status` and diff, but a
// review takes time: a secret written meanwhile must still stop the
// delivery that reads the repository again.
func TestADeliveryAfterAReviewReadsTheFilesAgain(t *testing.T) {
	s := newScenario(t, nil)
	s.write("README.md", "# shop\n")
	s.sh("add", "--all")
	s.sh("commit", "--quiet", "-m", "🎉 chore: init")
	s.sh("switch", "--quiet", "--create", "feat/login")
	ctx := context.Background()
	ws := app.Workspace{Git: s.git, Checkpoints: checkpointstore.New(s.git), Operations: oplogstore.New(s.git), OperatingSystem: "linux"}

	s.write("login.go", "package shop\n")
	view, err := app.ProposePlan(ctx, ws, app.PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// During the review, the developer (or their editor) writes a key.
	key := strings.Join([]string{"AK", "IA", "IOSFODNN7ABCDEFG"}, "")
	s.write("config.txt", "aws_access_key_id = "+key+"\n")
	_, err = app.DeliverLocally(ctx, ws, app.DeliveryRequest{Title: view.Title, Branch: view.Branch})
	var found *app.SecretsFoundError
	if !errors.As(err, &found) || found.Findings[0].Path != "config.txt" {
		t.Fatalf("delivery after the review: %v, want the secret of config.txt", err)
	}
	if s.sh("log", "--format=%s") != "🎉 chore: init" {
		t.Fatal("nothing may be committed when a secret is found")
	}

	// Right after a plan, the delivery commits what the plan read.
	s.write("config.txt", "region = eu-west-3\n")
	view, err = app.ProposePlan(ctx, ws, app.PlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.DeliverLocally(ctx, ws, app.DeliveryRequest{Title: view.Title, Branch: view.Branch, Reads: view.Reads()})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedBranch || result.Branch != "feat/login" || s.sh("status", "--porcelain") != "" {
		t.Fatalf("result = %+v, status %q", result, s.sh("status", "--porcelain"))
	}
	if files := s.sh("show", "--name-only", "--format=", "HEAD"); files != "config.txt\nlogin.go" {
		t.Errorf("committed %q", files)
	}
}
