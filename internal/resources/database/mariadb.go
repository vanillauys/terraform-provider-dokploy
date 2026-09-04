package database

import (
	"context"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// MariadbKind builds the mariadb database.Kind, adapting *client.Client's
// CreateMariadb/GetMariadb/UpdateMariadb/SaveMariadbEnvironment/
// SaveMariadbExternalPort/DeployMariadb/DeleteMariadb methods to KindClient's
// engine-neutral shape. See PostgresKind's doc comment for why c may be nil
// and why provider.go must re-evaluate this constructor on every
// registration call rather than hoisting it to a computed-once value.
//
// mariadb diverges from postgres in exactly the ways internal/client/doc.go's
// "Database engines" section records, and matches mysql field-for-field:
//
//   - database_root_password is a THIRD credential attribute postgres has
//     no equivalent of, modelled as Computed (server-generated when left
//     unset) with the same dialect-C exception on update as mysql - see
//     CreateMariadbRequest/UpdateMariadbRequest's doc comments in
//     internal/client/mariadb.go for the live evidence.
//   - The required create fields (databaseName, databaseUser,
//     databasePassword) and the uniform set are identical to postgres/mysql.
//
// mariadb's bare .create default image is mariadb:6, which does not exist on
// Docker Hub (doc.go) - unlike mysql:8/postgres's own working defaults.
// docker_image must be pinned to a real tag (e.g. mariadb:11.4) for anything
// that triggers a deploy (saveExternalPort, deploy) to succeed.
func MariadbKind(c *client.Client) Kind {
	return Kind{
		Name:      "mariadb",
		HumanName: "MariaDB",
		// "MariaDB" is the product's actual capitalization (not "Mariadb", a
		// mechanical re-casing of the lowercase resource-type string) - the
		// same casing trap flagged for MySQL/Redis's ShortName.
		ShortName:          "MariaDB",
		ExampleDockerImage: "mariadb:11.4",
		// doc.go: mariadb's server-side default image is mariadb:6, which
		// does not exist on Docker Hub. deploy_on_change defaults to true,
		// so a first apply that leaves docker_image unset creates the
		// record and then fails the deploy with a manifest-unknown error.
		DockerImageCaveat: " The server default `mariadb:6` does not exist on Docker Hub. A first apply without this attribute creates the record and then fails the deploy, because `deploy_on_change` defaults to `true`. Set an explicit tag that exists, for example `mariadb:11.4`.",
		CredentialAttrs: []CredentialAttr{
			{
				TFName:          "database_name",
				Description:     "Name of the MariaDB database.",
				Required:        true,
				RequiresReplace: true,
			},
			{
				TFName:          "database_user",
				Description:     "MariaDB user.",
				Required:        true,
				RequiresReplace: true,
			},
			{
				TFName:      "database_root_password",
				Description: "MariaDB root password. If unset, the server generates one. A change starts a redeploy.",
				Sensitive:   true,
				Computed:    true,
				// Same live-verified behavior as mysql's database_root_password
				// (kind.go's DeployTrigger doc comment): mariadb.update alone
				// changes only the stored record, not the running container's
				// env, until the next Deploy.
				DeployTrigger: true,
			},
		},
		Client: KindClient{
			Create: func(ctx context.Context, s CreateSpec) (*Object, error) {
				md, err := c.CreateMariadb(ctx, client.CreateMariadbRequest{
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
				return mariadbObject(md), nil
			},
			Get: func(ctx context.Context, id string) (*Object, error) {
				md, err := c.GetMariadb(ctx, id)
				if err != nil {
					return nil, err
				}
				return mariadbObject(md), nil
			},
			Update: func(ctx context.Context, s UpdateSpec) error {
				return c.UpdateMariadb(ctx, client.UpdateMariadbRequest{
					MariadbID:            s.ID,
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
				return c.SaveMariadbEnvironment(ctx, id, env)
			},
			SaveExternalPort: func(ctx context.Context, id string, port *int64) error {
				return c.SaveMariadbExternalPort(ctx, id, port)
			},
			Deploy: func(ctx context.Context, id string) error {
				return c.DeployMariadb(ctx, id)
			},
			Delete: func(ctx context.Context, id string) error {
				return c.DeleteMariadb(ctx, id)
			},
			// ListByEnvironment mirrors PostgresKind's/MysqlKind's: one
			// client.EnvironmentServices call (itself one environment.one
			// request), mapping its Mariadb []client.ServiceRef into partial
			// Objects carrying only ID and Name - the data source's Read
			// always finishes with a Get(ctx, id) for the rest.
			ListByEnvironment: func(ctx context.Context, environmentID string) ([]Object, error) {
				services, err := c.EnvironmentServices(ctx, environmentID)
				if err != nil {
					return nil, err
				}
				objs := make([]Object, len(services.Mariadb))
				for i, ref := range services.Mariadb {
					objs[i] = Object{ID: ref.ID, Name: ref.Name}
				}
				return objs, nil
			},
		},
	}
}

// mariadbObject maps a client.Mariadb into the engine-neutral Object.
func mariadbObject(md *client.Mariadb) *Object {
	return &Object{
		ID:                   md.MariadbID,
		Name:                 md.Name,
		AppName:              md.AppName,
		EnvironmentID:        md.EnvironmentID,
		DockerImage:          md.DockerImage,
		ApplicationStatus:    md.ApplicationStatus,
		CreatedAt:            md.CreatedAt,
		Description:          md.Description,
		Env:                  md.Env,
		ServerID:             md.ServerID,
		ExternalPort:         md.ExternalPort,
		DatabasePassword:     md.DatabasePassword,
		NetworkIDs:           md.NetworkIDs,
		DetachDokployNetwork: md.DetachDokployNetwork,
		Credentials: map[string]string{
			"database_name":          md.DatabaseName,
			"database_user":          md.DatabaseUser,
			"database_root_password": md.DatabaseRootPassword,
		},
	}
}
