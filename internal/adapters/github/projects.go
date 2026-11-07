// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GregoireF/ship-it/internal/core/forge"
	"github.com/GregoireF/ship-it/internal/core/roadmap"
	"github.com/GregoireF/ship-it/internal/ports"
)

var _ ports.Projects = (*Client)(nil)

// maxProjectPages bounds the listings of a project (100 per page).
const maxProjectPages = 50

// projectPath is the route of a project: /orgs/{org}/projectsV2/{number} or
// /users/{username}/projectsV2/{number} (REST API "Projects", 2026-03-10).
func projectPath(p forge.Project) string {
	return "/" + p.Kind + "/" + url.PathEscape(p.Owner) + "/projectsV2/" + strconv.Itoa(p.Number)
}

// projectCall sends one request on a project. A refusal (403) or a hidden
// project (404) from a classic token whose scopes lack the one needed —
// "project" to write, "project" or "read:project" to read — is reported as
// ports.ErrProjectScope: GitHub says which scopes a token has
// (X-OAuth-Scopes), fine-grained tokens excepted.
func (c *Client) projectCall(ctx context.Context, repo forge.Repository, method, path string, in, out any) (http.Header, error) {
	header, err := c.doWithHeader(ctx, repo, method, path, in, out)
	if err == nil || !(errors.Is(err, ports.ErrForbidden) || errors.Is(err, ports.ErrNotFound)) {
		return header, err
	}
	scopes, classic := header["X-Oauth-Scopes"]
	if !classic {
		return header, err
	}
	granted := map[string]bool{}
	for _, s := range strings.Split(strings.Join(scopes, ","), ",") {
		granted[strings.TrimSpace(s)] = true
	}
	if !granted["project"] && (method != http.MethodGet || !granted["read:project"]) {
		return header, errors.Join(ports.ErrProjectScope, err)
	}
	return header, err
}

type projectFieldJSON struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Options  []struct {
		ID   string `json:"id"`
		Name struct {
			Raw string `json:"raw"`
		} `json:"name"`
	} `json:"options"`
}

// ProjectFields implements ports.Projects (GET /{kind}/{owner}/projectsV2/{n},
// then its fields page after page).
func (c *Client) ProjectFields(ctx context.Context, repo forge.Repository, project forge.Project) (forge.Project, []roadmap.ProjectField, error) {
	kinds := []string{project.Kind}
	if project.Kind == "" {
		kinds = []string{forge.ProjectOfOrganization, forge.ProjectOfUser}
	}
	var err error
	for _, kind := range kinds {
		project.Kind = kind
		if _, err = c.projectCall(ctx, repo, http.MethodGet, projectPath(project), nil, nil); err == nil || !errors.Is(err, ports.ErrNotFound) {
			break
		}
	}
	if err != nil {
		return project, nil, err
	}
	var fields []roadmap.ProjectField
	next := projectPath(project) + "/fields?per_page=100"
	for page := 0; next != "" && page < maxProjectPages; page++ {
		var out []projectFieldJSON
		header, err := c.projectCall(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return project, nil, err
		}
		for _, f := range out {
			field := roadmap.ProjectField{ID: f.ID, Name: f.Name, Type: f.DataType}
			for _, o := range f.Options {
				field.Options = append(field.Options, roadmap.FieldOption{ID: o.ID, Name: o.Name.Raw})
			}
			fields = append(fields, field)
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return project, fields, nil
}

type projectItemJSON struct {
	ID          int64  `json:"id"`
	ContentType string `json:"content_type"`
	Content     *struct {
		ID            int64  `json:"id"`
		Number        int    `json:"number"`
		RepositoryURL string `json:"repository_url"`
	} `json:"content"`
	Fields []struct {
		ID       int64           `json:"id"`
		DataType string          `json:"data_type"`
		Value    json.RawMessage `json:"value"`
	} `json:"fields"`
}

// fieldValue reads a field value as the API takes it back: the id of an
// option or an iteration ({"id": …}), the day of a date (returned as a
// timestamp, "2025-06-13T02:00:00+02:00", checked 2026-11-01), a text.
func fieldValue(dataType string, raw json.RawMessage) string {
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil && object.ID != "" {
		return object.ID
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return "" // null, a number, a list: not a value ship-it sets
	}
	if dataType == roadmap.DateField && len(text) >= 10 {
		return text[:10]
	}
	return text
}

// ProjectItems implements ports.Projects (GET …/items?fields=…, page after
// page: the next pages are linked under /organizations/{id}/…).
func (c *Client) ProjectItems(ctx context.Context, repo forge.Repository, project forge.Project, fields []int64) ([]roadmap.ProjectItem, error) {
	query := url.Values{"per_page": {"100"}}
	if len(fields) > 0 {
		ids := make([]string, len(fields))
		for i, f := range fields {
			ids[i] = strconv.FormatInt(f, 10)
		}
		query.Set("fields", strings.Join(ids, ","))
	}
	ours := "/repos/" + repo.Owner + "/" + repo.Name
	var items []roadmap.ProjectItem
	next := projectPath(project) + "/items?" + query.Encode()
	for page := 0; next != "" && page < maxProjectPages; page++ {
		var out []projectItemJSON
		header, err := c.projectCall(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return nil, err
		}
		for _, it := range out {
			if it.ContentType != "Issue" || it.Content == nil ||
				!strings.HasSuffix(strings.ToLower(it.Content.RepositoryURL), strings.ToLower(ours)) {
				continue
			}
			item := roadmap.ProjectItem{ID: it.ID, Issue: it.Content.Number, Values: map[int64]string{}}
			for _, f := range it.Fields {
				if v := fieldValue(f.DataType, f.Value); v != "" {
					item.Values[f.ID] = v
				}
			}
			items = append(items, item)
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return items, nil
}

// AddProjectItem implements ports.Projects (POST …/items with the issue's id).
// An issue already in the project is not an error: GitHub's built-in
// workflow "Auto-add sub-issues to project", on by default, adds a sub-issue
// as soon as its parent is there, and then answers 422 "Content already
// exists in this project". The item it added is the one returned. The
// workflow runs in the background: the item can be missing from the listing
// for a moment after the 422, so the lookup is tried a few times.
func (c *Client) AddProjectItem(ctx context.Context, repo forge.Repository, project forge.Project, issueID int64) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	_, err := c.projectCall(ctx, repo, http.MethodPost, projectPath(project)+"/items", map[string]any{"type": "Issue", "id": issueID}, &out)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity || !strings.Contains(strings.ToLower(apiErr.Error()), "already exists") {
		return out.ID, err
	}
	for attempt := 1; attempt <= projectItemLookups; attempt++ {
		id, found, ferr := c.projectItemOf(ctx, repo, project, issueID)
		if ferr != nil {
			return 0, errors.Join(err, ferr)
		}
		if found {
			return id, nil
		}
		if attempt < projectItemLookups {
			select {
			case <-ctx.Done():
				return 0, errors.Join(err, ctx.Err())
			case <-time.After(time.Duration(attempt) * projectItemLookupDelay):
			}
		}
	}
	return 0, err
}

// projectItemLookups and projectItemLookupDelay bound the search for an item
// GitHub added itself: 1 s, then 2 s, 3 s and 4 s between the tries.
var (
	projectItemLookups     = 5
	projectItemLookupDelay = time.Second
)

// projectItemOf finds the item of an issue in a project.
func (c *Client) projectItemOf(ctx context.Context, repo forge.Repository, project forge.Project, issueID int64) (int64, bool, error) {
	next := projectPath(project) + "/items?per_page=100"
	for page := 0; next != "" && page < maxProjectPages; page++ {
		var out []projectItemJSON
		header, err := c.projectCall(ctx, repo, http.MethodGet, next, nil, &out)
		if err != nil {
			return 0, false, err
		}
		for _, it := range out {
			if it.Content != nil && it.Content.ID == issueID {
				return it.ID, true, nil
			}
		}
		next = nextPage(header.Get("Link"), c.baseURL(repo))
	}
	return 0, false, nil
}

// SetProjectField implements ports.Projects (PATCH …/items/{id} with
// {"fields": [{"id", "value"}]}, null emptying the field).
func (c *Client) SetProjectField(ctx context.Context, repo forge.Repository, project forge.Project, item, field int64, value string) error {
	var v any
	if value != "" {
		v = value
	}
	body := map[string]any{"fields": []map[string]any{{"id": field, "value": v}}}
	_, err := c.projectCall(ctx, repo, http.MethodPatch, fmt.Sprintf("%s/items/%d", projectPath(project), item), body, nil)
	return err
}

// RemoveProjectItem implements ports.Projects (DELETE …/items/{id}).
func (c *Client) RemoveProjectItem(ctx context.Context, repo forge.Repository, project forge.Project, item int64) error {
	_, err := c.projectCall(ctx, repo, http.MethodDelete, fmt.Sprintf("%s/items/%d", projectPath(project), item), nil, nil)
	if errors.Is(err, ports.ErrNotFound) && !errors.Is(err, ports.ErrProjectScope) {
		return nil // already gone
	}
	return err
}
