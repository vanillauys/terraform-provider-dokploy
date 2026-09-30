package webserverbackup

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

type resourceModel struct {
	ID                   types.String `tfsdk:"id"`
	DestinationID        types.String `tfsdk:"destination_id"`
	CronExpression       types.String `tfsdk:"cron_expression"`
	Prefix               types.String `tfsdk:"prefix"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	IncludeEncryptionKey types.Bool   `tfsdk:"include_encryption_key"`
	KeepLatestCount      types.Int64  `tfsdk:"keep_latest_count"`
}

func flatten(b *client.Backup, out *resourceModel) {
	out.ID = types.StringValue(b.BackupID)
	out.DestinationID = types.StringValue(b.DestinationID)
	out.CronExpression = types.StringValue(b.Schedule)
	out.Prefix = types.StringValue(b.Prefix)
	out.IncludeEncryptionKey = types.BoolValue(b.IncludeEncryptionKey)

	// enabled is nullable on the server but has a default here, so a null
	// read resolves to false. Dokploy produces null only for a record that
	// the API created without the field.
	out.Enabled = types.BoolValue(b.Enabled != nil && *b.Enabled)

	if b.KeepLatestCount != nil {
		out.KeepLatestCount = types.Int64Value(*b.KeepLatestCount)
	} else {
		out.KeepLatestCount = types.Int64Null()
	}
}

func createRequest(m resourceModel, userID string) client.CreateWebServerBackupRequest {
	return client.CreateWebServerBackupRequest{
		Schedule:             m.CronExpression.ValueString(),
		Database:             client.WebServerDatabaseName,
		Prefix:               m.Prefix.ValueString(),
		DestinationID:        m.DestinationID.ValueString(),
		DatabaseType:         client.WebServerDatabaseType,
		BackupType:           "database",
		Enabled:              m.Enabled.ValueBoolPointer(),
		KeepLatestCount:      m.KeepLatestCount.ValueInt64Pointer(),
		IncludeEncryptionKey: m.IncludeEncryptionKey.ValueBool(),
		UserID:               userID,
	}
}

// updateRequest sends every field, because backup.update is dialect A. The
// endpoint has no userId field, so the owner stays as Create set it.
func updateRequest(m resourceModel) client.UpdateBackupRequest {
	return client.UpdateBackupRequest{
		BackupID:             m.ID.ValueString(),
		Schedule:             m.CronExpression.ValueString(),
		Database:             client.WebServerDatabaseName,
		Prefix:               m.Prefix.ValueString(),
		DestinationID:        m.DestinationID.ValueString(),
		DatabaseType:         client.WebServerDatabaseType,
		Enabled:              m.Enabled.ValueBoolPointer(),
		KeepLatestCount:      m.KeepLatestCount.ValueInt64Pointer(),
		IncludeEncryptionKey: m.IncludeEncryptionKey.ValueBool(),
	}
}
