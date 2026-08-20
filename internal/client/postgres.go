package client

import (
	"context"
	"net/url"
)

type Postgres struct {
	PostgresID        string  `json:"postgresId"`
	Name              string  `json:"name"`
	AppName           string  `json:"appName"`
	DatabaseName      string  `json:"databaseName"`
	DatabaseUser      string  `json:"databaseUser"`
	DatabasePassword  string  `json:"databasePassword"`
	Description       *string `json:"description"`
	DockerImage       string  `json:"dockerImage"`
	ExternalPort      *int64  `json:"externalPort"`
	Env               *string `json:"env"`
	ApplicationStatus string  `json:"applicationStatus"`
	EnvironmentID     string  `json:"environmentId"`

	// v0.30.0 network attachment (probed 2026-08-19, see doc.go). networkIds
	// reads back as [] on a fresh record. After an explicit clear, it reads
	// back as a literal null. Both shapes decode to a nil or empty Go
	// slice. The resource layer collapses both to a null set.
	NetworkIDs           []string `json:"networkIds"`
	DetachDokployNetwork bool     `json:"detachDokployNetwork"`

	ServerID  *string `json:"serverId"`
	CreatedAt string  `json:"createdAt"`
}

type CreatePostgresRequest struct {
	Name             string  `json:"name"`
	AppName          string  `json:"appName,omitempty"`
	DatabaseName     string  `json:"databaseName"`
	DatabaseUser     string  `json:"databaseUser"`
	DatabasePassword string  `json:"databasePassword"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	Description      *string `json:"description,omitempty"`
	EnvironmentID    string  `json:"environmentId"`
	ServerID         *string `json:"serverId,omitempty"`
}

// UpdatePostgresRequest. Description is deliberately NOT omitempty, for the
// same reason as UpdateApplicationRequest.Description: verified empirically
// against a live Dokploy instance (v0.29.13, 2026-07-25) that postgres.update
// treats an absent `description` key as "leave the stored value alone"
// (returns true, postgres.one still reports the old text), while an explicit
// JSON null clears it (returns true, postgres.one then reports null). With
// omitempty a nil pointer vanished from the body, so removing `description`
// from config could never converge: state recorded null, the next Read
// flattened the server's stale value back in, and every plan showed the same
// diff forever (spec §5.6: optional attributes must be clearable back to
// null).
type UpdatePostgresRequest struct {
	PostgresID       string  `json:"postgresId"`
	Name             string  `json:"name,omitempty"`
	Description      *string `json:"description"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	DatabasePassword string  `json:"databasePassword,omitempty"`

	// v0.30.0 network attachment. NetworkIDs is nullable on the wire; a null
	// value clears it. DetachDokployNetwork is a bare boolean. The server
	// null-coerces it to false (doc.go, v0.30.0 section), so the client
	// always sends a concrete value - the Replicas pattern.
	NetworkIDs           *[]string `json:"networkIds"`
	DetachDokployNetwork bool      `json:"detachDokployNetwork"`
}

func (c *Client) CreatePostgres(ctx context.Context, req CreatePostgresRequest) (*Postgres, error) {
	var pg Postgres
	if err := c.Post(ctx, "/postgres.create", req, &pg); err != nil {
		return nil, err
	}
	return &pg, nil
}

func (c *Client) GetPostgres(ctx context.Context, id string) (*Postgres, error) {
	var pg Postgres
	if err := c.Get(ctx, "/postgres.one", url.Values{"postgresId": {id}}, &pg); err != nil {
		return nil, err
	}
	return &pg, nil
}

func (c *Client) UpdatePostgres(ctx context.Context, req UpdatePostgresRequest) error {
	return c.Post(ctx, "/postgres.update", req, nil)
}

func (c *Client) DeletePostgres(ctx context.Context, id string) error {
	return c.Post(ctx, "/postgres.remove", map[string]string{"postgresId": id}, nil)
}

// SavePostgresEnvironment sets or clears the extra environment variables. env
// is a *string, not a string, so that a null config value reaches the server
// as an explicit JSON null instead of an empty string: verified empirically
// against a live Dokploy instance (v0.29.13, 2026-07-25) that
// postgres.saveEnvironment declares `env` nullable-but-required in its zod
// schema — an entirely absent key 400s with "Input validation failed" /
// "expected nonoptional, received undefined", while an explicit JSON null is
// accepted and clears the stored value (postgres.one then reports env: null).
// The previous map[string]string signature could not express null at all, so
// clearing `env` from config stored "" server-side, Read flattened that back
// as types.StringValue(""), and every subsequent plan showed a permanent
// `"" -> null` diff (spec §5.6). Mirrors SavePostgresExternalPort, which
// already used *int64/map[string]any for exactly this reason.
func (c *Client) SavePostgresEnvironment(ctx context.Context, id string, env *string) error {
	return c.Post(ctx, "/postgres.saveEnvironment", map[string]any{"postgresId": id, "env": env}, nil)
}

// SavePostgresExternalPort sets or clears the external port. A nil port
// marshals to JSON null, which the API accepts to clear a previously set
// port (verified against a live instance 2026-07-23: postgres.one reports
// externalPort: null after a saveExternalPort call with externalPort: null).
func (c *Client) SavePostgresExternalPort(ctx context.Context, id string, port *int64) error {
	return c.Post(ctx, "/postgres.saveExternalPort", map[string]any{"postgresId": id, "externalPort": port}, nil)
}

func (c *Client) DeployPostgres(ctx context.Context, id string) error {
	return c.Post(ctx, "/postgres.deploy", map[string]string{"postgresId": id}, nil)
}
