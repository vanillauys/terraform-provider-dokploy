package client

import (
	"context"
	"net/url"
)

// Redis is the read shape for a Redis database service.
//
// Field set verified live against v0.29.13 (wave-2 task 2's doc.go record,
// re-verified live for this task, 2026-07-27): redis is the field-sparse
// extreme of the five database engines. Unlike Postgres/Mysql, there is NO
// DatabaseName and NO DatabaseUser field at all - redis.create accepts only
// databasePassword besides name/environmentId, and redis.one's response
// carries no databaseName/databaseUser/databaseRootPassword key whatsoever
// (a single-field probe against a scratch record confirmed this: the raw
// JSON response has no such keys, not even as null). This is a genuine
// absence, not an oversight - unlike Description/Env/ServerID/ExternalPort
// (server-mutable, so *string/*int64), there is nothing here to be null OR
// non-null, because the field does not exist on this engine's data model.
//
// redis.one's response also omits the `backups` array the other four
// engines (postgres, mysql, mariadb, mongo) all return, even when empty
// (doc.go). Not modelled here: this struct, like Postgres/Mysql, only
// declares the fields it actually uses and silently ignores everything else
// in the response - there is no `Backups` field on ANY of these structs, so
// its absence on redis specifically changes nothing about how this type
// decodes.
type Redis struct {
	RedisID           string  `json:"redisId"`
	Name              string  `json:"name"`
	AppName           string  `json:"appName"`
	DatabasePassword  string  `json:"databasePassword"`
	Description       *string `json:"description"`
	DockerImage       string  `json:"dockerImage"`
	ExternalPort      *int64  `json:"externalPort"`
	Env               *string `json:"env"`
	ApplicationStatus string  `json:"applicationStatus"`
	EnvironmentID     string  `json:"environmentId"`
	ServerID          *string `json:"serverId"`
	CreatedAt         string  `json:"createdAt"`
}

// CreateRedisRequest. There is deliberately no DatabaseName, DatabaseUser or
// DatabaseRootPassword field: redis.create's zod schema has no such
// parameters at all (doc.go; re-verified live 2026-07-27 against v0.29.13 -
// a scratch redis.create sent with only name/environmentId/databasePassword
// returned HTTP 200 with a fully formed record, and sending
// databaseRootPassword as an extra key on redis.update was silently
// stripped by the server rather than accepted, see UpdateRedisRequest's doc
// comment). This is the postgres/mysql shape minus two-to-three fields, not
// a smaller version of the same shape with blanks - there is nothing for a
// Terraform schema to expose for these, and CredentialAttrs is correspondingly
// empty on RedisKind (kind.go already supports this: see
// TestSchemaAttributes_ZeroCredentialAttrs in
// internal/resources/database/model_test.go, added ahead of this task as
// the anticipated redis shape).
type CreateRedisRequest struct {
	Name             string  `json:"name"`
	AppName          string  `json:"appName,omitempty"`
	DatabasePassword string  `json:"databasePassword"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	Description      *string `json:"description,omitempty"`
	EnvironmentID    string  `json:"environmentId"`
	ServerID         *string `json:"serverId,omitempty"`
}

// UpdateRedisRequest. Description follows UpdatePostgresRequest's pattern
// exactly (pointer, no omitempty - dialect B, verified live 2026-07-27:
// redis.update with description absent keeps the old value, HTTP 200;
// explicit null clears it).
//
// There is no DatabaseRootPassword field here, unlike UpdateMysqlRequest:
// redis has no such credential at all. Verified live (v0.29.13, 2026-07-27)
// that sending an extra `"databaseRootPassword": "..."` key on redis.update
// is silently stripped by the server's zod schema rather than accepted or
// rejected - a probe request carrying ONLY redisId and databaseRootPassword
// (no other settable field) returned HTTP 500 "No values to set", proving
// the key was dropped before reaching the update logic, not merely ignored
// after being stored. So there is no dialect-C exception to model for
// redis, unlike mysql/mariadb's databaseRootPassword (doc.go: "redis: ...
// NO databaseRootPassword").
type UpdateRedisRequest struct {
	RedisID          string  `json:"redisId"`
	Name             string  `json:"name,omitempty"`
	Description      *string `json:"description"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	DatabasePassword string  `json:"databasePassword,omitempty"`
}

func (c *Client) CreateRedis(ctx context.Context, req CreateRedisRequest) (*Redis, error) {
	var rd Redis
	if err := c.Post(ctx, "/redis.create", req, &rd); err != nil {
		return nil, err
	}
	return &rd, nil
}

func (c *Client) GetRedis(ctx context.Context, id string) (*Redis, error) {
	var rd Redis
	if err := c.Get(ctx, "/redis.one", url.Values{"redisId": {id}}, &rd); err != nil {
		return nil, err
	}
	return &rd, nil
}

func (c *Client) UpdateRedis(ctx context.Context, req UpdateRedisRequest) error {
	return c.Post(ctx, "/redis.update", req, nil)
}

func (c *Client) DeleteRedis(ctx context.Context, id string) error {
	return c.Post(ctx, "/redis.remove", map[string]string{"redisId": id}, nil)
}

// SaveRedisEnvironment sets or clears the extra environment variables.
// Mirrors SavePostgresEnvironment/SaveMysqlEnvironment: env is a *string so
// a null config value reaches the server as an explicit JSON null
// (redis.saveEnvironment is dialect A, verified live 2026-07-27: an absent
// `env` key 400s with "expected nonoptional, received undefined").
func (c *Client) SaveRedisEnvironment(ctx context.Context, id string, env *string) error {
	return c.Post(ctx, "/redis.saveEnvironment", map[string]any{"redisId": id, "env": env}, nil)
}

// SaveRedisExternalPort sets or clears the external port. Mirrors
// SavePostgresExternalPort/SaveMysqlExternalPort: redis.saveExternalPort is
// dialect A, verified live 2026-07-27 (absent `externalPort` key 400s
// "expected nonoptional, received undefined"; explicit value sets it and
// synchronously redeploys - applicationStatus moved idle -> done in the
// same probe that set it; explicit null clears a previously set port).
//
// This endpoint's existence is the answer to this task's central design
// question: doc.go already lists `redis.saveExternalPort` alongside every
// other engine's in its dialect-A endpoint table ("All five expose the same
// six-endpoint shape ... redis.saveEnvironment, redis.saveExternalPort"),
// and this task re-verified it live rather than trusting that record blind -
// so RedisKind's SaveExternalPort adapter below is populated exactly like
// every other engine's, never nil. See RedisKind's doc comment in
// resources/database/redis.go for the full evidence and why the Kind
// descriptor did not need to flex for this engine after all.
func (c *Client) SaveRedisExternalPort(ctx context.Context, id string, port *int64) error {
	return c.Post(ctx, "/redis.saveExternalPort", map[string]any{"redisId": id, "externalPort": port}, nil)
}

func (c *Client) DeployRedis(ctx context.Context, id string) error {
	return c.Post(ctx, "/redis.deploy", map[string]string{"redisId": id}, nil)
}
