package client

import (
	"context"
	"net/url"

	"github.com/vanillauys/terraform-provider-dokploy/internal/lookup"
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
// name for any service kind. Only the kinds this provider ships resources
// for are decoded; the rest (libsql, compose) are ignored until they have
// resources too.
//
// Each engine's own Kind.Client.ListByEnvironment (internal/resources/
// database/<engine>.go) decodes the matching field here: there is no other
// way to resolve a service by name, since environment.one is the only
// endpoint that returns every service kind in one call.
type EnvironmentServices struct {
	Applications []ServiceRef
	Postgres     []ServiceRef
	Mysql        []ServiceRef
	Redis        []ServiceRef
	Mariadb      []ServiceRef
	Mongo        []ServiceRef
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
		Mysql []struct {
			MysqlID string `json:"mysqlId"`
			Name    string `json:"name"`
		} `json:"mysql"`
		Redis []struct {
			RedisID string `json:"redisId"`
			Name    string `json:"name"`
		} `json:"redis"`
		Mariadb []struct {
			MariadbID string `json:"mariadbId"`
			Name      string `json:"name"`
		} `json:"mariadb"`
		Mongo []struct {
			MongoID string `json:"mongoId"`
			Name    string `json:"name"`
		} `json:"mongo"`
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
	for _, m := range raw.Mysql {
		out.Mysql = append(out.Mysql, ServiceRef{ID: m.MysqlID, Name: m.Name})
	}
	for _, r := range raw.Redis {
		out.Redis = append(out.Redis, ServiceRef{ID: r.RedisID, Name: r.Name})
	}
	for _, m := range raw.Mariadb {
		out.Mariadb = append(out.Mariadb, ServiceRef{ID: m.MariadbID, Name: m.Name})
	}
	for _, m := range raw.Mongo {
		out.Mongo = append(out.Mongo, ServiceRef{ID: m.MongoID, Name: m.Name})
	}
	return out, nil
}

// FindServiceByName resolves an exact service name to its id. It errors on
// multiple matches rather than picking one: Dokploy does not enforce unique
// service names within an environment.
//
// The zero/multiple-match error behavior and its nil-pointer found-sentinel
// (a *string, not a string compared against "", so "nothing matched yet"
// can't be confused with "matched a ref whose ID happens to be empty") now
// live in lookup.ByName — this and
// internal/datasources/database's findByName were, before Task 4's review,
// character-for-character duplicates of that same loop differing only in
// element type ([]ServiceRef vs []resourcedb.Object). See lookup.ByName's
// doc comment for the full rationale. Mirrors
// datasources/environment.FindByName, which still carries its own copy of
// the same sentinel (a pre-existing, out-of-scope third instance — see the
// task-4 report).
func FindServiceByName(refs []ServiceRef, name, kind string) (string, error) {
	return lookup.ByName(refs, name, kind,
		func(r ServiceRef) string { return r.ID },
		func(r ServiceRef) string { return r.Name },
	)
}
