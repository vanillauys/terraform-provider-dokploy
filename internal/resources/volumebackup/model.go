package volumebackup

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
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

func flatten(v *client.VolumeBackup, out *resourceModel) {
	out.ID = types.StringValue(v.VolumeBackupID)
	out.Name = types.StringValue(v.Name)
	out.ServiceType = types.StringValue(v.ServiceType)
	out.VolumeName = types.StringValue(v.VolumeName)
	out.Prefix = types.StringValue(v.Prefix)
	out.CronExpression = types.StringValue(v.CronExpression)
	out.DestinationID = types.StringValue(v.DestinationID)
	out.ServiceName = tfutil.StringOrNull(v.ServiceName)
	out.AppName = types.StringValue(v.AppName)
	out.CreatedAt = types.StringValue(v.CreatedAt)
	out.TurnOff = types.BoolValue(v.TurnOff)

	// enabled is nullable server-side but Optional+Computed with a default
	// here, so a null read resolves to a concrete false rather than leaving
	// state unknown. Dokploy only produces null for records created outside
	// this provider.
	out.Enabled = types.BoolValue(v.Enabled != nil && *v.Enabled)

	if v.KeepLatestCount != nil {
		out.KeepLatestCount = types.Int64Value(*v.KeepLatestCount)
	} else {
		out.KeepLatestCount = types.Int64Null()
	}

	if id := v.ParentRef().ID; id != "" {
		out.ServiceID = types.StringValue(id)
	} else {
		out.ServiceID = types.StringNull()
	}
}

func parentRef(m resourceModel) client.ParentRef {
	return client.ParentRef{
		Type: m.ServiceType.ValueString(),
		ID:   m.ServiceID.ValueString(),
	}
}

func createRequest(m resourceModel) client.CreateVolumeBackupRequest {
	ref := parentRef(m)
	return client.CreateVolumeBackupRequest{
		Name:            m.Name.ValueString(),
		VolumeName:      m.VolumeName.ValueString(),
		Prefix:          m.Prefix.ValueString(),
		CronExpression:  m.CronExpression.ValueString(),
		DestinationID:   m.DestinationID.ValueString(),
		ServiceType:     m.ServiceType.ValueString(),
		ServiceName:     m.ServiceName.ValueStringPointer(),
		KeepLatestCount: m.KeepLatestCount.ValueInt64Pointer(),
		Enabled:         m.Enabled.ValueBoolPointer(),
		TurnOff:         m.TurnOff.ValueBool(),
		ApplicationID:   ref.ColumnFor("application"),
		ComposeID:       ref.ColumnFor("compose"),
		PostgresID:      ref.ColumnFor("postgres"),
		MysqlID:         ref.ColumnFor("mysql"),
		MariadbID:       ref.ColumnFor("mariadb"),
		MongoID:         ref.ColumnFor("mongo"),
		RedisID:         ref.ColumnFor("redis"),
		LibsqlID:        ref.ColumnFor("libsql"),
	}
}

// updateRequest carries no parent field: volumeBackups.update sets the
// column it is given without clearing the others, so a retarget leaves the
// record owned by two services. service_id/service_type are RequiresReplace.
func updateRequest(m resourceModel) client.UpdateVolumeBackupRequest {
	return client.UpdateVolumeBackupRequest{
		VolumeBackupID:  m.ID.ValueString(),
		Name:            m.Name.ValueString(),
		VolumeName:      m.VolumeName.ValueString(),
		Prefix:          m.Prefix.ValueString(),
		CronExpression:  m.CronExpression.ValueString(),
		DestinationID:   m.DestinationID.ValueString(),
		ServiceName:     m.ServiceName.ValueStringPointer(),
		KeepLatestCount: m.KeepLatestCount.ValueInt64Pointer(),
		Enabled:         m.Enabled.ValueBoolPointer(),
		TurnOff:         m.TurnOff.ValueBool(),
	}
}
