package database

import (
	"context"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// MysqlKind builds the mysql database.Kind, adapting *client.Client's
// CreateMysql/GetMysql/UpdateMysql/SaveMysqlEnvironment/
// SaveMysqlExternalPort/DeployMysql/DeleteMysql methods to KindClient's
// engine-neutral shape. See PostgresKind's doc comment for why c may be
// nil and why provider.go must re-evaluate this constructor on every
// registration call rather than hoisting it to a computed-once value.
//
// mysql diverges from postgres in exactly the ways internal/client/doc.go's
// "Database engines" section records:
//
//   - database_root_password is a THIRD credential attribute postgres has
//     no equivalent of, modelled as Computed (server-generated when left
//     unset - see CreateMysqlRequest/UpdateMysqlRequest's doc comments in
//     internal/client/mysql.go for the full omit-vs-empty resolution and
//     the live evidence backing it).
//   - Otherwise the required create fields (databaseName, databaseUser,
//     databasePassword) and the uniform set are identical to postgres.
func MysqlKind(c *client.Client) Kind {
	return Kind{
		Name:      "mysql",
		HumanName: "MySQL",
		// Unlike postgres (HumanName "PostgreSQL", ShortName the shorter,
		// more commonly used "Postgres"), MySQL's product name has no
		// shorter idiomatic form to abbreviate to - "MySQL" already is the
		// short form. So ShortName intentionally equals HumanName here,
		// giving "MySQL service id." / "MySQL docker image, e.g. `mysql:8`.
		// ..." rather than a re-cased "Mysql" that would read as a typo of
		// the product's actual branding.
		ShortName:          "MySQL",
		ExampleDockerImage: "mysql:8",
		CredentialAttrs: []CredentialAttr{
			{
				TFName:          "database_name",
				Description:     "Name of the MySQL database.",
				Required:        true,
				RequiresReplace: true,
			},
			{
				TFName:          "database_user",
				Description:     "MySQL user.",
				Required:        true,
				RequiresReplace: true,
			},
			{
				TFName:      "database_root_password",
				Description: "MySQL root password. If unset, the server generates one. A change starts a redeploy.",
				Sensitive:   true,
				Computed:    true,
				// Verified live (2026-07-27): mysql.update alone changes
				// only the stored record; the running container's
				// MYSQL_ROOT_PASSWORD keeps its old value until the
				// following Deploy. See DeployTrigger's doc comment in
				// kind.go for the full evidence.
				DeployTrigger: true,
			},
		},
		Client: KindClient{
			Create: func(ctx context.Context, s CreateSpec) (*Object, error) {
				my, err := c.CreateMysql(ctx, client.CreateMysqlRequest{
					Name:                 s.Name,
					AppName:              s.AppName,
					DatabaseName:         s.Credentials["database_name"],
					DatabaseUser:         s.Credentials["database_user"],
					DatabasePassword:     s.DatabasePassword,
					DatabaseRootPassword: s.Credentials["database_root_password"],
					DockerImage:          s.DockerImage,
					Description:          s.Description,
					EnvironmentID:        s.EnvironmentID,
					ServerID:             s.ServerID,
				})
				if err != nil {
					return nil, err
				}
				return mysqlObject(my), nil
			},
			Get: func(ctx context.Context, id string) (*Object, error) {
				my, err := c.GetMysql(ctx, id)
				if err != nil {
					return nil, err
				}
				return mysqlObject(my), nil
			},
			Update: func(ctx context.Context, s UpdateSpec) error {
				return c.UpdateMysql(ctx, client.UpdateMysqlRequest{
					MysqlID:              s.ID,
					Name:                 s.Name,
					Description:          s.Description,
					DockerImage:          s.DockerImage,
					DatabasePassword:     s.DatabasePassword,
					DatabaseRootPassword: s.Credentials["database_root_password"],
					NetworkIDs:           s.NetworkIDs,
					DetachDokployNetwork: s.DetachDokployNetwork,
				})
			},
			SaveEnvironment: func(ctx context.Context, id string, env *string) error {
				return c.SaveMysqlEnvironment(ctx, id, env)
			},
			SaveExternalPort: func(ctx context.Context, id string, port *int64) error {
				return c.SaveMysqlExternalPort(ctx, id, port)
			},
			Deploy: func(ctx context.Context, id string) error {
				return c.DeployMysql(ctx, id)
			},
			Delete: func(ctx context.Context, id string) error {
				return c.DeleteMysql(ctx, id)
			},
			// ListByEnvironment mirrors PostgresKind's: one
			// client.EnvironmentServices call (itself one environment.one
			// request), mapping its Mysql []client.ServiceRef into partial
			// Objects carrying only ID and Name - the data source's Read
			// always finishes with a Get(ctx, id) for the rest.
			ListByEnvironment: func(ctx context.Context, environmentID string) ([]Object, error) {
				services, err := c.EnvironmentServices(ctx, environmentID)
				if err != nil {
					return nil, err
				}
				objs := make([]Object, len(services.Mysql))
				for i, ref := range services.Mysql {
					objs[i] = Object{ID: ref.ID, Name: ref.Name}
				}
				return objs, nil
			},
		},
	}
}

// mysqlObject maps a client.Mysql into the engine-neutral Object.
func mysqlObject(my *client.Mysql) *Object {
	return &Object{
		ID:                   my.MysqlID,
		Name:                 my.Name,
		AppName:              my.AppName,
		EnvironmentID:        my.EnvironmentID,
		DockerImage:          my.DockerImage,
		ApplicationStatus:    my.ApplicationStatus,
		CreatedAt:            my.CreatedAt,
		Description:          my.Description,
		Env:                  my.Env,
		ServerID:             my.ServerID,
		ExternalPort:         my.ExternalPort,
		DatabasePassword:     my.DatabasePassword,
		NetworkIDs:           my.NetworkIDs,
		DetachDokployNetwork: my.DetachDokployNetwork,
		Credentials: map[string]string{
			"database_name":          my.DatabaseName,
			"database_user":          my.DatabaseUser,
			"database_root_password": my.DatabaseRootPassword,
		},
	}
}
