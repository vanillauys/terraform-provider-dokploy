package client

import (
	"context"
	"net/url"
)

// Mariadb is the read shape for a MariaDB database service.
//
// Field set verified live against v0.29.13 (2026-07-27): mariadb.one returns
// the same shape as postgres.one/mysql.one PLUS DatabaseRootPassword, with
// the identical never-null-on-read behavior as mysql's equivalent field (see
// UpdateMariadbRequest's doc comment) - a plain string here, not a *string.
type Mariadb struct {
	MariadbID            string  `json:"mariadbId"`
	Name                 string  `json:"name"`
	AppName              string  `json:"appName"`
	DatabaseName         string  `json:"databaseName"`
	DatabaseUser         string  `json:"databaseUser"`
	DatabasePassword     string  `json:"databasePassword"`
	DatabaseRootPassword string  `json:"databaseRootPassword"`
	Description          *string `json:"description"`
	DockerImage          string  `json:"dockerImage"`
	ExternalPort         *int64  `json:"externalPort"`
	Env                  *string `json:"env"`
	ApplicationStatus    string  `json:"applicationStatus"`
	EnvironmentID        string  `json:"environmentId"`

	// v0.30.0 network attachment (probed 2026-08-19, see doc.go). networkIds
	// reads back as [] on a fresh record. After an explicit clear, it reads
	// back as a literal null. Both shapes decode to a nil or empty Go
	// slice. The resource layer collapses both to a null set.
	NetworkIDs           []string `json:"networkIds"`
	DetachDokployNetwork bool     `json:"detachDokployNetwork"`

	ServerID  *string `json:"serverId"`
	CreatedAt string  `json:"createdAt"`
}

// CreateMariadbRequest. DatabaseRootPassword is deliberately `omitempty`,
// mirroring CreateMysqlRequest exactly: a scratch mariadb.create sent with
// only name/environmentId/databaseName/databaseUser/databasePassword returns
// HTTP 200 with a server-generated, non-empty databaseRootPassword (verified
// live, 2026-07-27); an explicit empty string collapses to the same result
// as an absent key at create time (unlike on update - see
// UpdateMariadbRequest), so `omitempty` on a plain string is exactly what
// turns the generic engine's Unknown-credential-collapses-to-"" behavior
// into an entirely absent key, matching the server's own create-time
// semantics.
type CreateMariadbRequest struct {
	Name                 string  `json:"name"`
	AppName              string  `json:"appName,omitempty"`
	DatabaseName         string  `json:"databaseName"`
	DatabaseUser         string  `json:"databaseUser"`
	DatabasePassword     string  `json:"databasePassword"`
	DatabaseRootPassword string  `json:"databaseRootPassword,omitempty"`
	DockerImage          string  `json:"dockerImage,omitempty"`
	Description          *string `json:"description,omitempty"`
	EnvironmentID        string  `json:"environmentId"`
	ServerID             *string `json:"serverId,omitempty"`
}

// UpdateMariadbRequest. Description follows UpdatePostgresRequest's pattern
// exactly (pointer, no omitempty - dialect B).
//
// DatabaseRootPassword is a dialect-C exception inside this otherwise
// dialect-B endpoint, identical to UpdateMysqlRequest's field of the same
// name: verified live (v0.29.13, 2026-07-27) with isolated single-field
// mariadb.update calls against a record with an existing stored root
// password -
//
//   - key absent:    HTTP 200, mariadb.one still reports the OLD password
//     (dialect B's normal "keep" behavior).
//   - explicit null: HTTP 400, "Invalid input: expected string, received
//     null".
//   - explicit "":    HTTP 200, mariadb.one then reports databaseRootPassword
//     "" - the only way to clear it.
//
// So this field is a plain string with NO omitempty, exactly like
// UpdateMysqlRequest.DatabaseRootPassword, for the same reason (see that
// type's doc comment for the full rationale) - see
// TestRequestStructsNeverOmitMustSendFields's guard row for this type in
// dialect_test.go.
type UpdateMariadbRequest struct {
	MariadbID            string  `json:"mariadbId"`
	Name                 string  `json:"name,omitempty"`
	Description          *string `json:"description"`
	DockerImage          string  `json:"dockerImage,omitempty"`
	DatabasePassword     string  `json:"databasePassword,omitempty"`
	DatabaseRootPassword string  `json:"databaseRootPassword"`

	// v0.30.0 network attachment. NetworkIDs is nullable on the wire; a null
	// value clears it. DetachDokployNetwork is a bare boolean. The server
	// null-coerces it to false (doc.go, v0.30.0 section), so the client
	// always sends a concrete value - the Replicas pattern.
	NetworkIDs           *[]string `json:"networkIds"`
	DetachDokployNetwork bool      `json:"detachDokployNetwork"`
}

func (c *Client) CreateMariadb(ctx context.Context, req CreateMariadbRequest) (*Mariadb, error) {
	var m Mariadb
	if err := c.Post(ctx, "/mariadb.create", req, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) GetMariadb(ctx context.Context, id string) (*Mariadb, error) {
	var m Mariadb
	if err := c.Get(ctx, "/mariadb.one", url.Values{"mariadbId": {id}}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) UpdateMariadb(ctx context.Context, req UpdateMariadbRequest) error {
	return c.Post(ctx, "/mariadb.update", req, nil)
}

func (c *Client) DeleteMariadb(ctx context.Context, id string) error {
	return c.Post(ctx, "/mariadb.remove", map[string]string{"mariadbId": id}, nil)
}

// SaveMariadbEnvironment sets or clears the extra environment variables.
// Mirrors SaveMysqlEnvironment: env is a *string so a null config value
// reaches the server as an explicit JSON null (mariadb.saveEnvironment is
// dialect A, verified live 2026-07-27: an absent `env` key 400s with
// "expected nonoptional, received undefined").
func (c *Client) SaveMariadbEnvironment(ctx context.Context, id string, env *string) error {
	return c.Post(ctx, "/mariadb.saveEnvironment", map[string]any{"mariadbId": id, "env": env}, nil)
}

// SaveMariadbExternalPort sets or clears the external port. Mirrors
// SaveMysqlExternalPort: mariadb.saveExternalPort is dialect A, verified live
// 2026-07-27 (absent `externalPort` key 400s "expected nonoptional, received
// undefined"; explicit null clears a previously set port).
//
// mariadb:6, the server's bare .create default, does not exist on Docker
// Hub - calling this against a record still on that default image returns
// HTTP 500 "Error on deploy ... manifest for mariadb:6 not found" (verified
// live, 2026-07-27). Callers must create with an explicit working
// dockerImage (e.g. mariadb:11.4) before calling this.
func (c *Client) SaveMariadbExternalPort(ctx context.Context, id string, port *int64) error {
	return c.Post(ctx, "/mariadb.saveExternalPort", map[string]any{"mariadbId": id, "externalPort": port}, nil)
}

func (c *Client) DeployMariadb(ctx context.Context, id string) error {
	return c.PostDeploy(ctx, "/mariadb.deploy", map[string]string{"mariadbId": id})
}
