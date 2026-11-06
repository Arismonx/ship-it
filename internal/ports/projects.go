// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package ports

import (
	"context"
	"errors"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
)

// Projects is a GitHub Project (v2), for ship roadmap sync and ship
// publish. repo names the host and the credentials; the project belongs to
// an organisation or a user of that host.
type Projects interface {
	// ProjectFields returns the project's fields, and the project with its
	// owner's kind resolved: an owner given as "acme/7" is asked for as an
	// organisation, then as a user.
	ProjectFields(ctx context.Context, repo forge.Repository, project forge.Project) (forge.Project, []roadmap.ProjectField, error)
	// ProjectItems returns the items that are issues of repo, with the
	// values of the fields named (the API returns only the title otherwise).
	ProjectItems(ctx context.Context, repo forge.Repository, project forge.Project, fields []int64) ([]roadmap.ProjectItem, error)
	// AddProjectItem adds an issue, by its global id, and returns the item.
	AddProjectItem(ctx context.Context, repo forge.Repository, project forge.Project, issueID int64) (int64, error)
	// SetProjectField sets a field of an item to value (an option or
	// iteration id, a date, a text); "" empties it.
	SetProjectField(ctx context.Context, repo forge.Repository, project forge.Project, item, field int64, value string) error
	// RemoveProjectItem removes an item; already gone is fine.
	RemoveProjectItem(ctx context.Context, repo forge.Repository, project forge.Project, item int64) error
}

// ErrProjectScope: GitHub refused (or hid) a project and the token's scopes
// lack "project" (writes) or "read:project" (reads).
var ErrProjectScope = errors.New("ports: the token lacks the project scope")
