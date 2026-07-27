package client

import (
	"context"
	"fmt"
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

// ServiceRef is the minimum needed to resolve a service name to an id.
type ServiceRef struct {
	ID   string
	Name string
}

// EnvironmentServices lists the services in an environment, for name-based
// data-source lookup.
//
// environment.one embeds every service collection, so one call resolves a
// name for any service kind. Only the two kinds this provider ships are
// decoded; the rest (mysql, mariadb, mongo, redis, libsql, compose) are
// ignored until they have resources.
type EnvironmentServices struct {
	Applications []ServiceRef
	Postgres     []ServiceRef
}

func (c *Client) EnvironmentServices(ctx context.Context, environmentID string) (*EnvironmentServices, error) {
	var raw struct {
		Applications []struct {
			ApplicationID string `json:"applicationId"`
			Name          string `json:"name"`
		} `json:"applications"`
		Postgres []struct {
			PostgresID string `json:"postgresId"`
			Name       string `json:"name"`
		} `json:"postgres"`
	}
	if err := c.Get(ctx, "/environment.one", url.Values{"environmentId": {environmentID}}, &raw); err != nil {
		return nil, err
	}
	out := &EnvironmentServices{}
	for _, a := range raw.Applications {
		out.Applications = append(out.Applications, ServiceRef{ID: a.ApplicationID, Name: a.Name})
	}
	for _, p := range raw.Postgres {
		out.Postgres = append(out.Postgres, ServiceRef{ID: p.PostgresID, Name: p.Name})
	}
	return out, nil
}

// FindServiceByName resolves an exact service name to its id. It errors on
// multiple matches rather than picking one: Dokploy does not enforce unique
// service names within an environment.
//
// The "have I found one yet" sentinel is a *string, not a string compared
// against "": that comparison can't tell "nothing matched yet" apart from
// "matched a ref whose ID happens to be empty". The old found != "" check
// conflated the two, so a same-named ref with an empty ID would not be
// counted as a first match, and a second same-named ref could silently win
// instead of the lookup erroring. Nothing in the introspection for this
// provider observed Dokploy actually returning an empty service ID — this
// is a defensive correction for what the sentinel's own logic could not
// distinguish, not a report of live server behavior. Mirrors
// datasources/environment.FindByName, which uses the same nil-pointer
// sentinel for the same reason.
func FindServiceByName(refs []ServiceRef, name, kind string) (string, error) {
	var found *string
	for i := range refs {
		if refs[i].Name != name {
			continue
		}
		if found != nil {
			return "", fmt.Errorf("multiple %s services named %q in this environment; look it up by id instead", kind, name)
		}
		found = &refs[i].ID
	}
	if found == nil {
		return "", fmt.Errorf("no %s service named %q in this environment", kind, name)
	}
	return *found, nil
}
