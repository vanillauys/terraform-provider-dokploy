package database

import (
	"context"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// PostgresKind builds the postgres database.Kind, adapting *client.Client's
// existing CreatePostgres/GetPostgres/UpdatePostgres/SavePostgresEnvironment/
// SavePostgresExternalPort/DeployPostgres/DeletePostgres methods to
// KindClient's engine-neutral shape.
//
// c may be nil: PostgresKind is called once, eagerly, from
// provider.Resources() before the provider itself is configured (the
// framework caches Resources()'s returned factory funcs starting from its
// very first GetProviderSchema call, which always precedes Configure —
// verified directly against terraform-plugin-framework v1.19.0's
// internal/fwserver/server.go, Server.Resource). At that point only
// Metadata/Schema run on the resulting resource.Resource, and neither
// touches Kind.Client, so a nil c is harmless there.
//
// provider.go's registration re-evaluates `database.PostgresKind(p.client)`
// on every call (wrapped in its own closure, not computed once into a
// static value), and terraform-plugin-framework calls that factory closure
// fresh for every actual Create/Read/Update/Delete/ImportState operation
// (each is its own call to Server.Resource, which re-invokes the cached
// factory func) — by which time the provider's own Configure has already
// run, so p.client is the real, configured client. See the registration
// comment in provider.go. Future engines (Tasks 5-7) should mirror this same
// registration shape for their own <Engine>Kind constructors.
func PostgresKind(c *client.Client) Kind {
	return Kind{
		Name:               "postgres",
		HumanName:          "PostgreSQL",
		ShortName:          "Postgres",
		ExampleDockerImage: "postgres:16-alpine",
		CredentialAttrs: []CredentialAttr{
			{
				TFName:          "database_name",
				Description:     "Name of the PostgreSQL database.",
				Required:        true,
				RequiresReplace: true,
			},
			{
				TFName:          "database_user",
				Description:     "PostgreSQL user.",
				Required:        true,
				RequiresReplace: true,
			},
		},
		Client: KindClient{
			Create: func(ctx context.Context, s CreateSpec) (*Object, error) {
				pg, err := c.CreatePostgres(ctx, client.CreatePostgresRequest{
					Name:             s.Name,
					AppName:          s.AppName,
					DatabaseName:     s.Credentials["database_name"],
					DatabaseUser:     s.Credentials["database_user"],
					DatabasePassword: s.DatabasePassword,
					DockerImage:      s.DockerImage,
					Description:      s.Description,
					EnvironmentID:    s.EnvironmentID,
					ServerID:         s.ServerID,
				})
				if err != nil {
					return nil, err
				}
				return postgresObject(pg), nil
			},
			Get: func(ctx context.Context, id string) (*Object, error) {
				pg, err := c.GetPostgres(ctx, id)
				if err != nil {
					return nil, err
				}
				return postgresObject(pg), nil
			},
			Update: func(ctx context.Context, s UpdateSpec) error {
				return c.UpdatePostgres(ctx, client.UpdatePostgresRequest{
					PostgresID:       s.ID,
					Name:             s.Name,
					Description:      s.Description,
					DockerImage:      s.DockerImage,
					DatabasePassword: s.DatabasePassword,
				})
			},
			SaveEnvironment: func(ctx context.Context, id string, env *string) error {
				return c.SavePostgresEnvironment(ctx, id, env)
			},
			SaveExternalPort: func(ctx context.Context, id string, port *int64) error {
				return c.SavePostgresExternalPort(ctx, id, port)
			},
			Deploy: func(ctx context.Context, id string) error {
				return c.DeployPostgres(ctx, id)
			},
			Delete: func(ctx context.Context, id string) error {
				return c.DeletePostgres(ctx, id)
			},
			// ListByEnvironment keeps the exact mechanism the postgres data
			// source used before Task 4 folded it into the generic engine:
			// one client.EnvironmentServices call (which itself is one
			// environment.one request — no per-service fetch), then map its
			// Postgres []client.ServiceRef into partial Objects carrying
			// only ID and Name. The generic data-source engine's Read
			// always finishes with a Get(ctx, id) for the full object (see
			// internal/datasources/database), so nothing else needs to be
			// populated here.
			ListByEnvironment: func(ctx context.Context, environmentID string) ([]Object, error) {
				services, err := c.EnvironmentServices(ctx, environmentID)
				if err != nil {
					return nil, err
				}
				objs := make([]Object, len(services.Postgres))
				for i, ref := range services.Postgres {
					objs[i] = Object{ID: ref.ID, Name: ref.Name}
				}
				return objs, nil
			},
		},
	}
}

// postgresObject maps a client.Postgres into the engine-neutral Object.
func postgresObject(pg *client.Postgres) *Object {
	return &Object{
		ID:                pg.PostgresID,
		Name:              pg.Name,
		AppName:           pg.AppName,
		EnvironmentID:     pg.EnvironmentID,
		DockerImage:       pg.DockerImage,
		ApplicationStatus: pg.ApplicationStatus,
		CreatedAt:         pg.CreatedAt,
		Description:       pg.Description,
		Env:               pg.Env,
		ServerID:          pg.ServerID,
		ExternalPort:      pg.ExternalPort,
		DatabasePassword:  pg.DatabasePassword,
		Credentials: map[string]string{
			"database_name": pg.DatabaseName,
			"database_user": pg.DatabaseUser,
		},
	}
}
