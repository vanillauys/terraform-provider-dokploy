package client

import (
	"context"
	"net/url"
)

// Mongo is the read shape for a MongoDB database service.
//
// Field set verified live against v0.29.13 (2026-07-27): unlike
// Postgres/Mysql/Mariadb, there is NO DatabaseName field at all -
// mongo.create accepts only databaseUser/databasePassword besides
// name/environmentId, and mongo.one's response carries no databaseName or
// databaseRootPassword key whatsoever (a scratch record's raw JSON response
// has no such keys, not even as null). mongo.one's response also carries a
// `replicaSets` bool (defaults false, settable on both create and update) -
// deliberately NOT modelled here: this provider does not expose replica-set
// configuration as a Terraform attribute (see CreateMongoRequest's doc
// comment for why), and this struct, like Postgres/Mysql/Redis, only
// declares the fields it actually uses.
type Mongo struct {
	MongoID           string  `json:"mongoId"`
	Name              string  `json:"name"`
	AppName           string  `json:"appName"`
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

// CreateMongoRequest. There is deliberately no DatabaseName or
// DatabaseRootPassword field: mongo.create's zod schema has no
// databaseName parameter at all, and mongo has no root-password concept
// (databaseUser/databasePassword is its one admin credential) - a scratch
// mongo.create sent with only name/environmentId/databaseUser/
// databasePassword returns HTTP 200 with a fully formed record and neither
// key present anywhere.
//
// mongo.create's zod schema also accepts a `replicaSets` bool (defaults
// false when omitted, and is independently settable via mongo.update too -
// verified live, 2026-07-27). This field is NOT modelled here: replica-set
// configuration is a deployment-topology choice, not a string-shaped
// credential attribute, and does not fit CredentialAttr's fixed
// Required/RequiresReplace/Computed/Sensitive/DeployTrigger string
// interface (kind.go). Exposing it would need a new, non-string Kind
// attribute mechanism - out of scope for this field-map-configuration task;
// every dokploy_mongo instance this provider creates gets the server's
// standalone (non-replica-set) default. See this task's report for the
// full rationale.
type CreateMongoRequest struct {
	Name             string  `json:"name"`
	AppName          string  `json:"appName,omitempty"`
	DatabaseUser     string  `json:"databaseUser"`
	DatabasePassword string  `json:"databasePassword"`
	DockerImage      string  `json:"dockerImage,omitempty"`
	Description      *string `json:"description,omitempty"`
	EnvironmentID    string  `json:"environmentId"`
	ServerID         *string `json:"serverId,omitempty"`
}

// UpdateMongoRequest. Description follows UpdatePostgresRequest's pattern
// exactly (pointer, no omitempty - dialect B, verified live 2026-07-27:
// mongo.update with description absent keeps the old value, HTTP 200;
// explicit null clears it).
//
// There is no DatabaseRootPassword field here, matching redis: mongo has no
// such credential at all (see Mongo's doc comment). Unlike redis, an
// unrecognized extra key on mongo.update (databaseRootPassword, or any
// other bogus key) does not 500 with "No values to set" even when it is
// the only field in the request body - mongo.update tolerates a body with
// zero recognized fields and returns HTTP 200 regardless (verified live,
// 2026-07-27, sending mongoId alone). So that particular probe technique
// doesn't distinguish "field exists but is a no-op" from "field doesn't
// exist" for mongo the way it did for redis; the direct evidence here is
// mongo.one's response never carrying a databaseRootPassword key under any
// circumstance.
type UpdateMongoRequest struct {
	MongoID          string  `json:"mongoId"`
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

func (c *Client) CreateMongo(ctx context.Context, req CreateMongoRequest) (*Mongo, error) {
	var mo Mongo
	if err := c.Post(ctx, "/mongo.create", req, &mo); err != nil {
		return nil, err
	}
	return &mo, nil
}

func (c *Client) GetMongo(ctx context.Context, id string) (*Mongo, error) {
	var mo Mongo
	if err := c.Get(ctx, "/mongo.one", url.Values{"mongoId": {id}}, &mo); err != nil {
		return nil, err
	}
	return &mo, nil
}

func (c *Client) UpdateMongo(ctx context.Context, req UpdateMongoRequest) error {
	return c.Post(ctx, "/mongo.update", req, nil)
}

func (c *Client) DeleteMongo(ctx context.Context, id string) error {
	return c.Post(ctx, "/mongo.remove", map[string]string{"mongoId": id}, nil)
}

// SaveMongoEnvironment sets or clears the extra environment variables.
// Mirrors SaveMysqlEnvironment/SaveRedisEnvironment: env is a *string so a
// null config value reaches the server as an explicit JSON null
// (mongo.saveEnvironment is dialect A, verified live 2026-07-27: an absent
// `env` key 400s with "expected nonoptional, received undefined").
func (c *Client) SaveMongoEnvironment(ctx context.Context, id string, env *string) error {
	return c.Post(ctx, "/mongo.saveEnvironment", map[string]any{"mongoId": id, "env": env}, nil)
}

// SaveMongoExternalPort sets or clears the external port. Mirrors
// SaveMysqlExternalPort/SaveRedisExternalPort: mongo.saveExternalPort is
// dialect A, verified live 2026-07-27 (absent `externalPort` key 400s
// "expected nonoptional, received undefined"; explicit null clears a
// previously set port).
//
// mongo:15, the server's bare .create default, does not exist on Docker
// Hub - calling this against a record still on that default image returns
// HTTP 500 "Error on deploy ... manifest for mongo:15 not found" (verified
// live, 2026-07-27). Callers must create with an explicit working
// dockerImage (e.g. mongo:7) before calling this.
func (c *Client) SaveMongoExternalPort(ctx context.Context, id string, port *int64) error {
	return c.Post(ctx, "/mongo.saveExternalPort", map[string]any{"mongoId": id, "externalPort": port}, nil)
}

func (c *Client) DeployMongo(ctx context.Context, id string) error {
	return c.Post(ctx, "/mongo.deploy", map[string]string{"mongoId": id}, nil)
}
