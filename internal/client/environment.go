package client

import (
	"context"
	"net/url"
)

// Environment is an environment inside a Dokploy project. Every project
// auto-creates one named "production" with IsDefault true; environment.remove
// refuses to delete it ("You cannot delete the default environment").
//
// There is deliberately no CreatedAt field. environment.create and
// environment.update both return createdAt, but environment.one does NOT
// (verified live, v0.29.13, 2026-07-26). Modelling it would give a field that
// is populated after a create and empty after every read — an attribute whose
// value tells you how the record entered state rather than anything about the
// environment.
//
// Description and Env are plain strings, not pointers, because this is a
// dialect C record (see doc.go): the server represents "unset" as null on a
// never-set field and as "" on a cleared one. JSON null decodes into a Go
// string as the zero value, so both arrive here as "" and callers only have
// one case to handle.
type Environment struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	ProjectID     string `json:"projectId"`
	Description   string `json:"description"`
	Env           string `json:"env"`
	IsDefault     bool   `json:"isDefault"`
}

// CreateEnvironmentRequest is dialect C (doc.go): Description is a plain
// string with no omitempty. An explicit JSON null is rejected with
// "expected string, received null"; "" is accepted and stored as "".
//
// Env is deliberately absent from this struct. environment.create ACCEPTS an
// `env` key and then silently discards it — a create sending env:"A=1"
// produces a record with env:"" — so callers that need it must follow the
// create with an UpdateEnvironment.
type CreateEnvironmentRequest struct {
	Name        string `json:"name"`
	ProjectID   string `json:"projectId"`
	Description string `json:"description"`
}

// UpdateEnvironmentRequest is dialect C (doc.go). Every field is a plain
// string with no omitempty: an absent key silently keeps the stored value and
// an explicit null is a 400, so "" is the only way to clear Description or
// Env. Name is sent on every call for the same reason.
type UpdateEnvironmentRequest struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Env           string `json:"env"`
}

func (c *Client) CreateEnvironment(ctx context.Context, req CreateEnvironmentRequest) (*Environment, error) {
	var e Environment
	if err := c.Post(ctx, "/environment.create", req, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (c *Client) GetEnvironment(ctx context.Context, id string) (*Environment, error) {
	var e Environment
	if err := c.Get(ctx, "/environment.one", url.Values{"environmentId": {id}}, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (c *Client) UpdateEnvironment(ctx context.Context, req UpdateEnvironmentRequest) error {
	return c.Post(ctx, "/environment.update", req, nil)
}

func (c *Client) DeleteEnvironment(ctx context.Context, id string) error {
	return c.Post(ctx, "/environment.remove", map[string]string{"environmentId": id}, nil)
}

// ListEnvironments returns every environment in a project.
//
// environment.byProjectId omits both `projectId` and `env` from each row
// (verified live), unlike environment.one which returns them. ProjectID is
// backfilled from the argument so callers get a consistent record; Env stays
// empty, so a caller needing it must GetEnvironment the id.
func (c *Client) ListEnvironments(ctx context.Context, projectID string) ([]Environment, error) {
	var es []Environment
	if err := c.Get(ctx, "/environment.byProjectId", url.Values{"projectId": {projectID}}, &es); err != nil {
		return nil, err
	}
	for i := range es {
		es[i].ProjectID = projectID
	}
	return es, nil
}
