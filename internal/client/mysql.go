package client

import (
	"context"
	"net/url"
)

// Mysql is the read shape for a MySQL database service.
//
// Field set verified live against v0.29.13 (wave-2 task 2, re-verified task
// 5): mysql.one returns the same shape as postgres.one PLUS
// DatabaseRootPassword, which postgres has no equivalent of at all. Unlike
// Description/Env/ServerID/ExternalPort, DatabaseRootPassword is never JSON
// null on read — mysql.one always reports a string, either the
// server-generated value, a user-supplied one, or "" after it has been
// explicitly cleared via UpdateMysql (see UpdateMysqlRequest's doc comment).
// So it is a plain string here, not a *string.
type Mysql struct {
	MysqlID              string  `json:"mysqlId"`
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
	ServerID             *string `json:"serverId"`
	CreatedAt            string  `json:"createdAt"`
}

// CreateMysqlRequest. DatabaseRootPassword is deliberately `omitempty`,
// mirroring AppName/DockerImage/Description below rather than
// UpdateMysqlRequest's DatabaseRootPassword (no omitempty, see that type's
// doc comment): this is a CREATE endpoint, and doc.go records that an
// absent databaseRootPassword key at create means "server generates a
// random value" — the same "absent = schema default" shape AppName and
// DockerImage already use here, not dialect A/B/C (those only describe
// absent-key semantics for a record that already exists to keep or clear).
//
// Verified live (v0.29.13, 2026-07-27, wave-2 task 5) that this field can
// safely stay a plain string with omitempty even though the generic
// resource engine's Create (internal/resources/database/resource.go)
// always inserts every CredentialAttr into CreateSpec.Credentials via
// plan.Credentials[name].ValueString() — which collapses BOTH an Unknown
// planned value (a Computed attribute left unset in config, the normal
// case) and a Terraform-config-set empty string to the same Go "": sending
// an explicit `"databaseRootPassword":""` to mysql.create produces the
// exact same result as omitting the key entirely — the server generates a
// random password either way (single-field probe against a scratch
// record: both got a non-empty, different generated value back). This is
// NOT the same field's behavior on update (see UpdateMysqlRequest): only
// at create does the server treat empty identically to absent, because
// there is no existing stored value for empty-string to legitimately
// "clear". So the naive collapse-to-"" is harmless here, and `omitempty`
// on a plain (non-pointer) string is what turns that harmless "" into an
// entirely absent key — matching the create-time semantics exactly, no
// generic-engine change required.
type CreateMysqlRequest struct {
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

// UpdateMysqlRequest. Description follows UpdatePostgresRequest's pattern
// exactly (pointer, no omitempty, see that type's doc comment for the
// live-verified dialect-B rationale).
//
// DatabaseRootPassword is the field doc.go flags as a dialect-C EXCEPTION
// living inside this otherwise dialect-B endpoint, and it is why this
// field is a plain string with NO omitempty (unlike CreateMysqlRequest's
// DatabaseRootPassword above): verified live (v0.29.13, 2026-07-27) with
// isolated single-field mysql.update calls against a record that already
// had a stored root password —
//
//   - key absent:    HTTP 200, mysql.one still reports the OLD password
//     (dialect B's normal "keep" behavior).
//   - explicit null: HTTP 400, "Invalid input: expected string, received
//     null" — dialect A/B would accept null and clear the field; this
//     endpoint's zod schema for this one field has no nullable variant.
//   - explicit "":    HTTP 200, mysql.one then reports databaseRootPassword
//     "" — this is the ONLY way to clear it.
//
// A plain string with omitempty would make a Terraform config setting
// database_root_password = "" indistinguishable from an unset one (both
// marshal to an absent key), which the "keep" behavior above would then
// silently preserve the old password forever — the exact non-convergent
// diff pattern spec §5.6 exists to prevent. A *string with no omitempty
// would marshal Go's nil as JSON null, which this field's zod schema
// rejects outright (400), unlike every other dialect-B/A field in this
// package. So this field needs its own plain-string-no-omitempty shape,
// matching UpdateEnvironmentRequest's Description/Env (dialect C) rather
// than UpdatePostgresRequest's Description (dialect A/B) — see
// TestRequestStructsNeverOmitMustSendFields's guard row for this type in
// dialect_test.go.
//
// The generic resource engine (internal/resources/database/resource.go)
// always resends the CURRENT planned value of every Computed
// CredentialAttr on every Update call (UpdateSpec.Credentials; see its
// doc comment), so this field's key is always populated from
// s.Credentials["database_root_password"] — never left at Go's zero
// value by accident the way a merely-absent map entry would be.
type UpdateMysqlRequest struct {
	MysqlID              string  `json:"mysqlId"`
	Name                 string  `json:"name,omitempty"`
	Description          *string `json:"description"`
	DockerImage          string  `json:"dockerImage,omitempty"`
	DatabasePassword     string  `json:"databasePassword,omitempty"`
	DatabaseRootPassword string  `json:"databaseRootPassword"`
}

func (c *Client) CreateMysql(ctx context.Context, req CreateMysqlRequest) (*Mysql, error) {
	var m Mysql
	if err := c.Post(ctx, "/mysql.create", req, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) GetMysql(ctx context.Context, id string) (*Mysql, error) {
	var m Mysql
	if err := c.Get(ctx, "/mysql.one", url.Values{"mysqlId": {id}}, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) UpdateMysql(ctx context.Context, req UpdateMysqlRequest) error {
	return c.Post(ctx, "/mysql.update", req, nil)
}

func (c *Client) DeleteMysql(ctx context.Context, id string) error {
	return c.Post(ctx, "/mysql.remove", map[string]string{"mysqlId": id}, nil)
}

// SaveMysqlEnvironment sets or clears the extra environment variables.
// Mirrors SavePostgresEnvironment: env is a *string so a null config value
// reaches the server as an explicit JSON null (mysql.saveEnvironment is
// dialect A, verified live 2026-07-27: an absent `env` key 400s with
// "expected nonoptional, received undefined").
func (c *Client) SaveMysqlEnvironment(ctx context.Context, id string, env *string) error {
	return c.Post(ctx, "/mysql.saveEnvironment", map[string]any{"mysqlId": id, "env": env}, nil)
}

// SaveMysqlExternalPort sets or clears the external port. Mirrors
// SavePostgresExternalPort: mysql.saveExternalPort is dialect A, verified
// live 2026-07-27 (absent `externalPort` key 400s "expected nonoptional,
// received undefined"; explicit null clears a previously set port).
func (c *Client) SaveMysqlExternalPort(ctx context.Context, id string, port *int64) error {
	return c.Post(ctx, "/mysql.saveExternalPort", map[string]any{"mysqlId": id, "externalPort": port}, nil)
}

func (c *Client) DeployMysql(ctx context.Context, id string) error {
	return c.Post(ctx, "/mysql.deploy", map[string]string{"mysqlId": id}, nil)
}
