package client

import (
	"context"
	"net/url"
)

// Project. Env is the project-level environment variables (#54, v1.5.0):
// a dialect C column like Environment.Env, so a plain string that reads ""
// when unset and after a clear. An environment does NOT inherit it:
// environment.one reports its own env only (probed live, v0.30.6,
// 2026-09-17), the shared value is a separate scope that a service reads
// through the ${{project.KEY}} syntax.
type Project struct {
	ProjectID    string        `json:"projectId"`
	Name         string        `json:"name"`
	Description  *string       `json:"description"`
	Env          string        `json:"env"`
	CreatedAt    string        `json:"createdAt"`
	Environments []Environment `json:"environments"`
}

// CreateProjectRequest. Unlike environment.create, project.create accepts
// env AND stores it (probed live, v0.30.6, 2026-09-17), so no follow-up
// update is needed. Env is dialect C: a plain string, never null.
type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Env         string  `json:"env"`
}

// UpdateProjectRequest. Description is deliberately NOT omitempty, for the
// same reason as UpdateApplicationRequest.Description: verified empirically
// against a live Dokploy instance (v0.29.13, 2026-07-25) that project.update
// treats an absent `description` key as "leave the stored value alone"
// (project.one still reports the old text afterwards), while an explicit
// JSON null clears it (project.one then reports null). With omitempty a nil
// pointer vanished from the body, so removing `description` from config
// could never converge: state recorded null, the next Read flattened the
// server's stale value back in, and every plan showed the same diff forever
// (spec §5.6: optional attributes must be clearable back to null).
//
// Env is dialect C on this endpoint (probed live, v0.30.6, 2026-09-17): an
// absent key keeps the stored value, an explicit null is an HTTP 400
// "expected string, received null", and "" clears it. A plain string with
// no omitempty sends "" for a null attribute.
type UpdateProjectRequest struct {
	ProjectID   string  `json:"projectId"`
	Name        string  `json:"name,omitempty"`
	Description *string `json:"description"`
	Env         string  `json:"env"`
}

// createProjectResponse matches the real /project.create response shape:
// unlike every other project.* endpoint, it wraps its result as
// {"project": {...}, "environment": {...}} instead of returning a flat
// Project object (discovered against the live acceptance rig; the plain
// Project shape decodes to all zero values, silently breaking the
// follow-up project.one read with an empty projectId).
type createProjectResponse struct {
	Project     Project     `json:"project"`
	Environment Environment `json:"environment"`
}

func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
	var resp createProjectResponse
	if err := c.Post(ctx, "/project.create", req, &resp); err != nil {
		return nil, err
	}
	p := resp.Project
	p.Environments = []Environment{resp.Environment}
	return &p, nil
}

func (c *Client) GetProject(ctx context.Context, projectID string) (*Project, error) {
	var p Project
	if err := c.Get(ctx, "/project.one", url.Values{"projectId": {projectID}}, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) UpdateProject(ctx context.Context, req UpdateProjectRequest) error {
	return c.Post(ctx, "/project.update", req, nil)
}

func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	return c.Post(ctx, "/project.remove", map[string]string{"projectId": projectID}, nil)
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var ps []Project
	if err := c.Get(ctx, "/project.all", nil, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}
