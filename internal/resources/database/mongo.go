package database

import (
	"context"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// MongoKind builds the mongo database.Kind, adapting *client.Client's
// CreateMongo/GetMongo/UpdateMongo/SaveMongoEnvironment/
// SaveMongoExternalPort/DeployMongo/DeleteMongo methods to KindClient's
// engine-neutral shape. See PostgresKind's doc comment for why c may be nil
// and why provider.go must re-evaluate this constructor on every
// registration call rather than hoisting it to a computed-once value.
//
// mongo diverges from every other engine in internal/client/doc.go's
// "Database engines" table: it has NO database_name credential attribute at
// all - mongo.create's zod schema has no such parameter, and mongo.one's
// response carries no databaseName key whatsoever, not even null. mongo
// also has no database_root_password (mongo's one admin credential is
// database_user/database_password; there is nothing else to be
// server-generated or cleared). So CredentialAttrs here is exactly one
// entry - database_user, Required + RequiresReplace - unlike postgres/
// mysql/mariadb's two-or-three and unlike redis's zero.
//
// mongo's bare .create default image is mongo:15, which does not exist on
// Docker Hub (doc.go) - unlike mysql:8/postgres's own working defaults.
// docker_image must be pinned to a real tag (e.g. mongo:7) for anything
// that triggers a deploy (saveExternalPort, deploy) to succeed.
//
// # replicaSets is a known, deliberate gap
//
// mongo.create's zod schema also accepts a `replicaSets` bool (defaults
// false when omitted; verified live, 2026-07-27, also independently
// settable via mongo.update). This provider does NOT expose it as a
// Terraform attribute: CredentialAttr's fixed interface is string-only by
// design (kind.go: "every CredentialAttr is a string... matching Task 2's
// evidence that all per-engine credential fields ... are strings"), and
// replicaSets is neither a credential nor string-shaped - it is a
// deployment-topology toggle. Exposing it would need a new, non-string Kind
// attribute mechanism (e.g. a second, typed attribute list alongside
// CredentialAttrs, threaded through CreateSpec/UpdateSpec/Object/
// schemaAttributes/genericModel), which is a materially larger, generic-
// engine change than the field-map configuration this task is scoped to,
// and one that would need to be re-verified against every existing engine's
// tests. Every dokploy_mongo instance this provider creates therefore gets
// the server's standalone (non-replica-set) default. Deferring it is safe:
// mongo.update is dialect B (doc.go), and UpdateMongoRequest declares no
// replicaSets field, so a server-side replicaSets: true (set out-of-band,
// e.g. via the Dokploy UI) survives every provider Update call untouched -
// there is no clobber risk in leaving this gap open. See this wave's task-7
// report for the full rationale and live evidence; a follow-up task should
// pick this up if replica-set mongo instances are ever needed. Whenever that
// happens, the attribute MUST be Optional+Computed, not plain Optional: Read
// (flatten) will populate it from the server's actual value on every
// refresh, and a plain Optional attribute left unset in config would fight
// that server-reported value as a permanent diff forever (spec §5.6) rather
// than adopting it, exactly the trap UseStateForUnknown/Computed exists to
// avoid for docker_image/app_name above.
func MongoKind(c *client.Client) Kind {
	return Kind{
		Name:      "mongo",
		HumanName: "MongoDB",
		// "MongoDB" is the product's actual capitalization (not "Mongo", a
		// mechanical re-casing of the lowercase resource-type string) - the
		// same casing trap flagged for MySQL/MariaDB/Redis's ShortName.
		ShortName:          "MongoDB",
		ExampleDockerImage: "mongo:7",
		// doc.go: mongo's server-side default image is mongo:15, which
		// does not exist on Docker Hub. deploy_on_change defaults to true,
		// so a first apply that leaves docker_image unset creates the
		// record and then fails the deploy with a manifest-unknown error.
		DockerImageCaveat: " The server's own default (`mongo:15`) does not exist on Docker Hub; a first apply that leaves this unset creates the record and then fails the deploy (`deploy_on_change` defaults to `true`). Set an explicit, real tag such as `mongo:7`.",
		CredentialAttrs: []CredentialAttr{
			{
				TFName:          "database_user",
				Description:     "MongoDB user.",
				Required:        true,
				RequiresReplace: true,
			},
		},
		Client: KindClient{
			Create: func(ctx context.Context, s CreateSpec) (*Object, error) {
				mo, err := c.CreateMongo(ctx, client.CreateMongoRequest{
					Name:             s.Name,
					AppName:          s.AppName,
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
				return mongoObject(mo), nil
			},
			Get: func(ctx context.Context, id string) (*Object, error) {
				mo, err := c.GetMongo(ctx, id)
				if err != nil {
					return nil, err
				}
				return mongoObject(mo), nil
			},
			Update: func(ctx context.Context, s UpdateSpec) error {
				return c.UpdateMongo(ctx, client.UpdateMongoRequest{
					MongoID:          s.ID,
					Name:             s.Name,
					Description:      s.Description,
					DockerImage:      s.DockerImage,
					DatabasePassword: s.DatabasePassword,
				})
			},
			SaveEnvironment: func(ctx context.Context, id string, env *string) error {
				return c.SaveMongoEnvironment(ctx, id, env)
			},
			SaveExternalPort: func(ctx context.Context, id string, port *int64) error {
				return c.SaveMongoExternalPort(ctx, id, port)
			},
			Deploy: func(ctx context.Context, id string) error {
				return c.DeployMongo(ctx, id)
			},
			Delete: func(ctx context.Context, id string) error {
				return c.DeleteMongo(ctx, id)
			},
			// ListByEnvironment mirrors every other engine's: one
			// client.EnvironmentServices call (itself one environment.one
			// request), mapping its Mongo []client.ServiceRef into partial
			// Objects carrying only ID and Name - the data source's Read
			// always finishes with a Get(ctx, id) for the rest.
			ListByEnvironment: func(ctx context.Context, environmentID string) ([]Object, error) {
				services, err := c.EnvironmentServices(ctx, environmentID)
				if err != nil {
					return nil, err
				}
				objs := make([]Object, len(services.Mongo))
				for i, ref := range services.Mongo {
					objs[i] = Object{ID: ref.ID, Name: ref.Name}
				}
				return objs, nil
			},
		},
	}
}

// mongoObject maps a client.Mongo into the engine-neutral Object.
func mongoObject(mo *client.Mongo) *Object {
	return &Object{
		ID:                mo.MongoID,
		Name:              mo.Name,
		AppName:           mo.AppName,
		EnvironmentID:     mo.EnvironmentID,
		DockerImage:       mo.DockerImage,
		ApplicationStatus: mo.ApplicationStatus,
		CreatedAt:         mo.CreatedAt,
		Description:       mo.Description,
		Env:               mo.Env,
		ServerID:          mo.ServerID,
		ExternalPort:      mo.ExternalPort,
		DatabasePassword:  mo.DatabasePassword,
		Credentials: map[string]string{
			"database_user": mo.DatabaseUser,
		},
	}
}
