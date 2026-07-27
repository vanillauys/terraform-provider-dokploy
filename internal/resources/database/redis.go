package database

import (
	"context"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// RedisKind builds the redis database.Kind, adapting *client.Client's
// CreateRedis/GetRedis/UpdateRedis/SaveRedisEnvironment/
// SaveRedisExternalPort/DeployRedis/DeleteRedis methods to KindClient's
// engine-neutral shape. See PostgresKind's doc comment for why c may be nil
// and why provider.go must re-evaluate this constructor on every
// registration call rather than hoisting it to a computed-once value.
//
// redis is the field-sparse extreme of the five database engines (doc.go's
// "Database engines" table): CredentialAttrs is empty (nil) here, not a
// smaller non-empty list. Live evidence backing that, re-verified against
// v0.29.13 on this task's own probes (2026-07-27), on top of doc.go's
// existing record:
//
//   - redis.create accepts only name/environmentId/databasePassword — a
//     scratch create sent with exactly those three fields returned HTTP 200
//     with a fully formed record. There is no databaseName, no
//     databaseUser, and no databaseRootPassword anywhere in the response.
//   - redis.update silently STRIPS an unrecognized databaseRootPassword key
//     rather than storing or rejecting it: a probe request carrying ONLY
//     redisId and databaseRootPassword (no other settable field) returned
//     HTTP 500 "No values to set" — proof the key never reached the update
//     logic at all, confirming there is no such field to model, not merely
//     one this task chooses not to expose.
//   - redis.one's response has no databaseName/databaseUser/
//     databaseRootPassword key at all (not even null), and separately omits
//     the `backups` array the other four engines all return.
//
// This is why RedisKind has no per-engine credential attributes beyond the
// uniform set's database_password — CredentialAttrs is nil, which kind.go's
// schemaAttributes and model.go's flatten/setComputed/resolveCredentials all
// already handle (proven ahead of this task by
// TestSchemaAttributes_ZeroCredentialAttrs in model_test.go, added
// anticipating exactly this shape). No generic-engine change was needed to
// express it: a Kind IS the escape hatch spec §4.2 asks for when a field set
// doesn't fit an assumed shape, and an empty CredentialAttrs slice is a
// valid, already-supported point in that space, not a gap requiring one.
//
// # The SaveExternalPort question: did NOT need to flex
//
// This task's brief raised the possibility that redis might lack a working
// saveExternalPort endpoint (framed as: a Kind with a nil SaveExternalPort
// must either be expressible, or the record must prove redis has the
// endpoint). It does not need to be nil here. doc.go already listed
// `redis.saveEnvironment, redis.saveExternalPort` explicitly in the
// dialect-A endpoint table ("All five expose the same six-endpoint shape
// ... and split across the same two dialects"), and this task re-verified
// that live rather than trusting the record blind:
//
//   - redis.saveExternalPort: absent `externalPort` key -> HTTP 400
//     "Invalid input: expected nonoptional, received undefined" (identical
//     to postgres/mysql). An explicit port value -> HTTP 200, and
//     redis.one's applicationStatus moved idle -> done in the same probe
//     (saveExternalPort synchronously redeploys here too, same as the
//     mariadb/mongo broken-default-image finding doc.go already recorded —
//     redis:8 is a real, pullable tag though, so this never fails).
//     Explicit `externalPort: null` -> HTTP 200, redis.one then reports
//     externalPort: null.
//   - redis.saveEnvironment: absent `env` key -> HTTP 400 "expected
//     nonoptional, received undefined"; explicit value -> 200, stored;
//     explicit null -> 200, cleared.
//
// So both save-endpoints exist and behave exactly like every other engine's
// (dialect A, full round trip proven). Client/redis.go's
// SaveRedisEnvironment/SaveRedisExternalPort are populated below exactly
// like PostgresKind/MysqlKind's — never nil, no branch, no escape hatch
// needed for this part of the design question either.
func RedisKind(c *client.Client) Kind {
	return Kind{
		Name:      "redis",
		HumanName: "Redis",
		// Redis's product name, like MySQL's, has no shorter idiomatic form
		// to abbreviate to - "Redis" already is the short form. ShortName
		// intentionally equals HumanName, giving "Redis service id." /
		// "Redis docker image, e.g. `redis:8`. ..." rather than a
		// re-cased or invented alternative. See MysqlKind's doc comment
		// (task 5's report) for the ShortName-casing trap this avoids.
		ShortName: "Redis",
		// redis:8 is a real, pullable tag (doc.go: "redis redis:8 real tag,
		// pulls fine") - unlike mariadb:6/mongo:15, which doc.go already
		// flagged as broken. No pin workaround is needed for this engine's
		// acceptance tests the way Task 7's mariadb/mongo will need one.
		ExampleDockerImage: "redis:8",
		// CredentialAttrs is deliberately nil: redis has no per-engine
		// credential field beyond the uniform set's database_password (see
		// this Kind's doc comment above for the live evidence). This is the
		// zero-CredentialAttrs case kind.go/model.go were already built to
		// support.
		CredentialAttrs: nil,
		Client: KindClient{
			Create: func(ctx context.Context, s CreateSpec) (*Object, error) {
				rd, err := c.CreateRedis(ctx, client.CreateRedisRequest{
					Name:             s.Name,
					AppName:          s.AppName,
					DatabasePassword: s.DatabasePassword,
					DockerImage:      s.DockerImage,
					Description:      s.Description,
					EnvironmentID:    s.EnvironmentID,
					ServerID:         s.ServerID,
				})
				if err != nil {
					return nil, err
				}
				return redisObject(rd), nil
			},
			Get: func(ctx context.Context, id string) (*Object, error) {
				rd, err := c.GetRedis(ctx, id)
				if err != nil {
					return nil, err
				}
				return redisObject(rd), nil
			},
			Update: func(ctx context.Context, s UpdateSpec) error {
				return c.UpdateRedis(ctx, client.UpdateRedisRequest{
					RedisID:          s.ID,
					Name:             s.Name,
					Description:      s.Description,
					DockerImage:      s.DockerImage,
					DatabasePassword: s.DatabasePassword,
				})
			},
			SaveEnvironment: func(ctx context.Context, id string, env *string) error {
				return c.SaveRedisEnvironment(ctx, id, env)
			},
			SaveExternalPort: func(ctx context.Context, id string, port *int64) error {
				return c.SaveRedisExternalPort(ctx, id, port)
			},
			Deploy: func(ctx context.Context, id string) error {
				return c.DeployRedis(ctx, id)
			},
			Delete: func(ctx context.Context, id string) error {
				return c.DeleteRedis(ctx, id)
			},
			// ListByEnvironment mirrors PostgresKind's/MysqlKind's: one
			// client.EnvironmentServices call (itself one environment.one
			// request), mapping its Redis []client.ServiceRef into partial
			// Objects carrying only ID and Name - the data source's Read
			// always finishes with a Get(ctx, id) for the rest.
			ListByEnvironment: func(ctx context.Context, environmentID string) ([]Object, error) {
				services, err := c.EnvironmentServices(ctx, environmentID)
				if err != nil {
					return nil, err
				}
				objs := make([]Object, len(services.Redis))
				for i, ref := range services.Redis {
					objs[i] = Object{ID: ref.ID, Name: ref.Name}
				}
				return objs, nil
			},
		},
	}
}

// redisObject maps a client.Redis into the engine-neutral Object.
// Credentials is an empty map, not nil: RedisKind has no CredentialAttrs
// to populate it with, mirroring the zero-CredentialAttrs handling already
// exercised by TestFlatten_MissingCredentialGoesNull / flatten's own nil-map
// initialization in model.go for a Kind whose adapter returns a nil map
// here — either shape works, but an explicit empty map documents that this
// is deliberate, not an oversight.
func redisObject(rd *client.Redis) *Object {
	return &Object{
		ID:                rd.RedisID,
		Name:              rd.Name,
		AppName:           rd.AppName,
		EnvironmentID:     rd.EnvironmentID,
		DockerImage:       rd.DockerImage,
		ApplicationStatus: rd.ApplicationStatus,
		CreatedAt:         rd.CreatedAt,
		Description:       rd.Description,
		Env:               rd.Env,
		ServerID:          rd.ServerID,
		ExternalPort:      rd.ExternalPort,
		DatabasePassword:  rd.DatabasePassword,
		Credentials:       map[string]string{},
	}
}
