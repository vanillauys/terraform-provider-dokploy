package client

import (
	"context"
	"net/url"
)

// Environment is modeled from Dokploy's Drizzle schema; only the fields
// wave 0 needs. Dokploy auto-creates a "production" environment per project.
type Environment struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	ProjectID     string `json:"projectId"`
}

type Project struct {
	ProjectID    string        `json:"projectId"`
	Name         string        `json:"name"`
	Description  *string       `json:"description"`
	CreatedAt    string        `json:"createdAt"`
	Environments []Environment `json:"environments"`
}

type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
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
type UpdateProjectRequest struct {
	ProjectID   string  `json:"projectId"`
	Name        string  `json:"name,omitempty"`
	Description *string `json:"description"`
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
