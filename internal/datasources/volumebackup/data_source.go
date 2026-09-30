// Package volumebackup holds the dokploy_volume_backup data source.
package volumebackup

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
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	ServiceID       types.String `tfsdk:"service_id"`
	ServiceType     types.String `tfsdk:"service_type"`
	VolumeName      types.String `tfsdk:"volume_name"`
	Prefix          types.String `tfsdk:"prefix"`
	CronExpression  types.String `tfsdk:"cron_expression"`
	DestinationID   types.String `tfsdk:"destination_id"`
	ServiceName     types.String `tfsdk:"service_name"`
	KeepLatestCount types.Int64  `tfsdk:"keep_latest_count"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	TurnOff         types.Bool   `tfsdk:"turn_off"`
	AppName         types.String `tfsdk:"app_name"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func flatten(v *client.VolumeBackup, out *model) {
	out.ID = types.StringValue(v.VolumeBackupID)
	out.Name = types.StringValue(v.Name)
	serviceID := v.ParentRef().ID
	out.ServiceID = tfutil.StringOrNull(&serviceID)
	out.ServiceType = types.StringValue(v.ServiceType)
	out.VolumeName = types.StringValue(v.VolumeName)
	out.Prefix = types.StringValue(v.Prefix)
	out.CronExpression = types.StringValue(v.CronExpression)
	out.DestinationID = types.StringValue(v.DestinationID)
	out.ServiceName = tfutil.StringOrNull(v.ServiceName)
	out.KeepLatestCount = types.Int64PointerValue(v.KeepLatestCount)
	out.Enabled = types.BoolValue(v.Enabled != nil && *v.Enabled)
	out.TurnOff = types.BoolValue(v.TurnOff)
	out.AppName = types.StringValue(v.AppName)
	out.CreatedAt = types.StringValue(v.CreatedAt)
}

// NewDataSource returns the dokploy_volume_backup data source.
func NewDataSource() datasource.DataSource {
	child := dsutil.Child{
		Kind: "volume backup",
		What: "a volume backup schedule of a service that already exists in Dokploy",
		Example: "data \"dokploy_volume_backup\" \"data\" {\n  service_id   = data.dokploy_application.web.id\n" +
			"  service_type = \"application\"\n  name         = \"data-volume\"\n}",
	}
	return dsutil.NewDataSource(dsutil.Record[model, client.VolumeBackup]{
		Name:        "volume_backup",
		Description: child.Description(),
		Attributes: map[string]schema.Attribute{
			"id":                child.IDAttribute("service_id"),
			"service_id":        dsutil.LookupString("Id of the service that owns the volume backup. Set it with `service_type` and `name`."),
			"service_type":      dsutil.LookupString("Type of the service: `application`, `compose`, `postgres`, `mysql`, `mariadb`, `mongo`, `redis` or `libsql`."),
			"name":              dsutil.LookupString("Display name of the volume backup."),
			"volume_name":       dsutil.String("Name of the volume to back up."),
			"prefix":            dsutil.String("Prefix of the backup files in the destination."),
			"cron_expression":   dsutil.String("Cron expression of the backup schedule."),
			"destination_id":    dsutil.String("Id of the destination that stores the backup."),
			"service_name":      dsutil.String("Compose service name, or null."),
			"keep_latest_count": dsutil.Int64("Number of backups to keep, or null to keep all."),
			"enabled":           dsutil.Bool("Whether the backup runs."),
			"turn_off":          dsutil.Bool("Whether Dokploy stops the service during the backup."),
			"app_name":          dsutil.String("Internal name of the volume backup."),
			"created_at":        dsutil.String("Creation timestamp from the server."),
		},
		Validators: dsutil.ChildValidators("service_id", []string{"service_type"}, "name"),
		ID:         func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.VolumeBackup, error) {
			return c.GetVolumeBackup(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.VolumeBackup, error) {
			ref := client.ParentRef{Type: m.ServiceType.ValueString(), ID: m.ServiceID.ValueString()}
			backups, err := c.ListVolumeBackups(ctx, ref)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(backups, func(x client.VolumeBackup) bool { return x.Name == m.Name.ValueString() },
				"volume backup", "the name on "+ref.Type+" "+ref.ID)
		},
		Flatten: flatten,
	})()
}
