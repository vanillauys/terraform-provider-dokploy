package backup

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID                   types.String `tfsdk:"id"`
	ServiceID            types.String `tfsdk:"service_id"`
	ServiceType          types.String `tfsdk:"service_type"`
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

// resourceModelV0 is the schema version 0 state shape. It differs from
// resourceModel in one field: the cron attribute was named `schedule`.
type resourceModelV0 struct {
	ID                   types.String `tfsdk:"id"`
	ServiceID            types.String `tfsdk:"service_id"`
	ServiceType          types.String `tfsdk:"service_type"`
	Database             types.String `tfsdk:"database"`
	Prefix               types.String `tfsdk:"prefix"`
	Schedule             types.String `tfsdk:"schedule"`
	DestinationID        types.String `tfsdk:"destination_id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	KeepLatestCount      types.Int64  `tfsdk:"keep_latest_count"`
	IncludeEncryptionKey types.Bool   `tfsdk:"include_encryption_key"`
	ServiceName          types.String `tfsdk:"service_name"`
	AppName              types.String `tfsdk:"app_name"`
}

// upgrade copies every field into the current model and moves the cron
// expression to its new name.
func (v resourceModelV0) upgrade() resourceModel {
	return resourceModel{
		ID:                   v.ID,
		ServiceID:            v.ServiceID,
		ServiceType:          v.ServiceType,
		Database:             v.Database,
		Prefix:               v.Prefix,
		CronExpression:       v.Schedule,
		DestinationID:        v.DestinationID,
		Enabled:              v.Enabled,
		KeepLatestCount:      v.KeepLatestCount,
		IncludeEncryptionKey: v.IncludeEncryptionKey,
		ServiceName:          v.ServiceName,
		AppName:              v.AppName,
	}
}

// backupTypeFor derives the wire `backupType` from the parent kind. Dokploy
// splits its own records into "database" and "compose"; the user never says
// which, because the parent already determines it.
func backupTypeFor(serviceType string) string {
	if serviceType == "compose" {
		return "compose"
	}
	return "database"
}

func flatten(b *client.Backup, out *resourceModel) {
	out.ID = types.StringValue(b.BackupID)
	out.ServiceType = types.StringValue(b.DatabaseType)
	out.Database = types.StringValue(b.Database)
	out.Prefix = types.StringValue(b.Prefix)
	out.CronExpression = types.StringValue(b.Schedule)
	out.DestinationID = types.StringValue(b.DestinationID)
	out.ServiceName = tfutil.StringOrNull(b.ServiceName)
	out.AppName = types.StringValue(b.AppName)
	out.IncludeEncryptionKey = types.BoolValue(b.IncludeEncryptionKey)

	// enabled is nullable server-side but Optional+Computed with a default
	// here, so a null read resolves to a concrete false. Dokploy only
	// produces null for records created outside this provider.
	out.Enabled = types.BoolValue(b.Enabled != nil && *b.Enabled)

	if b.KeepLatestCount != nil {
		out.KeepLatestCount = types.Int64Value(*b.KeepLatestCount)
	} else {
		out.KeepLatestCount = types.Int64Null()
	}

	if id := b.ParentRef().ID; id != "" {
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

func createRequest(m resourceModel) client.CreateBackupRequest {
	ref := parentRef(m)
	return client.CreateBackupRequest{
		Schedule:             m.CronExpression.ValueString(),
		Database:             m.Database.ValueString(),
		Prefix:               m.Prefix.ValueString(),
		DestinationID:        m.DestinationID.ValueString(),
		DatabaseType:         ref.Type,
		BackupType:           backupTypeFor(ref.Type),
		Enabled:              m.Enabled.ValueBoolPointer(),
		KeepLatestCount:      m.KeepLatestCount.ValueInt64Pointer(),
		IncludeEncryptionKey: m.IncludeEncryptionKey.ValueBool(),
		ServiceName:          m.ServiceName.ValueStringPointer(),
		PostgresID:           ref.ColumnFor("postgres"),
		MysqlID:              ref.ColumnFor("mysql"),
		MariadbID:            ref.ColumnFor("mariadb"),
		MongoID:              ref.ColumnFor("mongo"),
		LibsqlID:             ref.ColumnFor("libsql"),
		ComposeID:            ref.ColumnFor("compose"),
	}
}

// updateRequest re-sends databaseType unchanged.
//
// backup.update requires the key, and it is the ONLY parent-ish field the
// endpoint carries — so sending the value already in state is what keeps the
// record coherent. It is never taken from user input: service_type is
// RequiresReplace, so plan and state always agree on it here.
//
// metadata is sent as an explicit nil. Its schema is `anyOf: [{}, null]` —
// genuinely untyped — and it has read back null on every record observed, so
// there is no shape to model and no value to preserve. Recorded in
// censusExempt rather than left silent.
func updateRequest(m resourceModel) client.UpdateBackupRequest {
	return client.UpdateBackupRequest{
		BackupID:             m.ID.ValueString(),
		Schedule:             m.CronExpression.ValueString(),
		Database:             m.Database.ValueString(),
		Prefix:               m.Prefix.ValueString(),
		DestinationID:        m.DestinationID.ValueString(),
		DatabaseType:         m.ServiceType.ValueString(),
		Enabled:              m.Enabled.ValueBoolPointer(),
		KeepLatestCount:      m.KeepLatestCount.ValueInt64Pointer(),
		IncludeEncryptionKey: m.IncludeEncryptionKey.ValueBool(),
		ServiceName:          m.ServiceName.ValueStringPointer(),
		Metadata:             nil,
	}
}
