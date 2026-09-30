// Package backup holds the dokploy_backup data source.
package backup

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type model struct {
	ID                   types.String `tfsdk:"id"`
	ServiceID            types.String `tfsdk:"service_id"`
	ServiceType          types.String `tfsdk:"service_type"`
	ComposeDatabaseType  types.String `tfsdk:"compose_database_type"`
	Database             types.String `tfsdk:"database"`
	Prefix               types.String `tfsdk:"prefix"`
	CronExpression       types.String `tfsdk:"cron_expression"`
	DestinationID        types.String `tfsdk:"destination_id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	KeepLatestCount      types.Int64  `tfsdk:"keep_latest_count"`
	IncludeEncryptionKey types.Bool   `tfsdk:"include_encryption_key"`
	ServiceName          types.String `tfsdk:"service_name"`
	AppName              types.String `tfsdk:"app_name"`
}

// flatten follows the resource: a compose parent reads back as service_type
// `compose`, and the engine moves to compose_database_type.
func flatten(b *client.Backup, out *model) {
	out.ID = types.StringValue(b.BackupID)
	if b.BackupType == "compose" {
		out.ServiceType = types.StringValue("compose")
		out.ComposeDatabaseType = types.StringValue(b.DatabaseType)
	} else {
		out.ServiceType = types.StringValue(b.DatabaseType)
		out.ComposeDatabaseType = types.StringNull()
	}
	serviceID := b.ParentRef().ID
	out.ServiceID = tfutil.StringOrNull(&serviceID)
	out.Database = types.StringValue(b.Database)
	out.Prefix = types.StringValue(b.Prefix)
	out.CronExpression = types.StringValue(b.Schedule)
	out.DestinationID = types.StringValue(b.DestinationID)
	out.Enabled = types.BoolValue(b.Enabled != nil && *b.Enabled)
	out.KeepLatestCount = types.Int64PointerValue(b.KeepLatestCount)
	out.IncludeEncryptionKey = types.BoolValue(b.IncludeEncryptionKey)
	out.ServiceName = tfutil.StringOrNull(b.ServiceName)
	out.AppName = types.StringValue(b.AppName)
}

// NewDataSource returns the dokploy_backup data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "backup",
		What: "a database backup schedule of a database or compose service that already exists in Dokploy",
		Example: "data \"dokploy_backup\" \"nightly\" {\n  service_id   = data.dokploy_postgres.main.id\n" +
			"  service_type = \"postgres\"\n  prefix       = \"nightly\"\n}",
		Secret: "the compose database credentials", SecretAttr: "The `compose_database_*` credentials exist",
		Resource: "`dokploy_backup`",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.Backup]{
		Name:        "backup",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":                     child.IDAttribute("service_id"),
			"service_id":             dsutil.LookupString("Id of the service that owns the backup. Set it with `service_type` and `prefix`."),
			"service_type":           dsutil.LookupString("Type of the service: `postgres`, `mysql`, `mariadb`, `mongo`, `libsql` or `compose`."),
			"prefix":                 dsutil.LookupString("Prefix of the backup files in the destination."),
			"compose_database_type":  dsutil.String("Database engine of a compose service, or null for a database service."),
			"database":               dsutil.String("Name of the database to back up."),
			"cron_expression":        dsutil.String("Cron expression of the backup schedule."),
			"destination_id":         dsutil.String("Id of the destination that stores the backup."),
			"enabled":                dsutil.Bool("Whether the backup runs."),
			"keep_latest_count":      dsutil.Int64("Number of backups to keep, or null to keep all."),
			"include_encryption_key": dsutil.Bool("Whether the backup includes the encryption key."),
			"service_name":           dsutil.String("Compose service name, or null."),
			"app_name":               dsutil.String("Internal name of the backup."),
		},
		Validators: dsutil.ChildValidators("service_id", []string{"service_type"}, "prefix"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.Backup, error) {
			return c.GetBackup(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.Backup, error) {
			ref := client.ParentRef{Type: m.ServiceType.ValueString(), ID: m.ServiceID.ValueString()}
			backups, err := c.ListBackups(ctx, ref)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(backups, func(x client.Backup) bool { return x.Prefix == m.Prefix.ValueString() },
				"backup", "the prefix on "+ref.Type+" "+ref.ID)
		},
		Flatten: flatten,
	})()
}
