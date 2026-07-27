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
// redis is the field-sparse extreme of the five database engines
// (internal/client/doc.go's "Database engines" table): CredentialAttrs is
// empty (nil), not a smaller non-empty list. redis.create accepts only
// name/environmentId/databasePassword — there is no databaseName, no
// databaseUser, and no databaseRootPassword anywhere in redis.create's
// response, redis.one's response, or redis.update's accepted fields (an
// unrecognized databaseRootPassword key is silently stripped by the
// server's zod schema before the update logic runs, confirmed by a probe
// sending only redisId + databaseRootPassword: HTTP 500 "No values to
// set"). redis.one's response also omits the `backups` array the other
// four engines all return, even when empty; not modelled here, since this
// struct only declares the fields it uses.
//
// kind.go's schemaAttributes and model.go's flatten/setComputed/
// resolveCredentials/credentialsNeedServerValue all already handle a
// zero-CredentialAttrs Kind (see TestSchemaAttributes_ZeroCredentialAttrs
// in model_test.go). No generic-engine change was needed for this shape.
//
// redis.saveEnvironment and redis.saveExternalPort both exist and are both
// dialect A, identical to postgres/mysql: absent key -> HTTP 400 "expected
// nonoptional, received undefined"; explicit value -> HTTP 200
// (saveExternalPort synchronously redeploys — applicationStatus moves
// idle -> done in the same call); explicit null -> HTTP 200, cleared.
// redis:8 is a real, pullable default image (unlike mariadb:6/mongo:15).
func RedisKind(c *client.Client) Kind {
	return Kind{
		Name:      "redis",
		HumanName: "Redis",
		// Redis's product name, like MySQL's, has no shorter idiomatic form
		// to abbreviate to - "Redis" already is the short form. ShortName
		// intentionally equals HumanName, giving "Redis service id." /
		// "Redis docker image, e.g. `redis:8`. ..." rather than a
		// re-cased or invented alternative.
		ShortName: "Redis",
		// redis:8 is a real, pullable tag, unlike mariadb:6/mongo:15 (both
		// broken on Docker Hub — see internal/client/doc.go).
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
