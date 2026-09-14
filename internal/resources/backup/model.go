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

// resourceModelV0 is the schema version 0 state shape. It differs from the
// current model in two ways: the cron attribute was named `schedule`, and
// there was no `compose_database_type` (added in version 2 — see
// resourceModelV1 for the version 1 shape, which sits between the two).
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

// upgrade copies every field into the current model, moves the cron
// expression to its new name, and leaves compose_database_type null: no
// version 0 state can have carried a value for an attribute that did not
// exist yet.
func (v resourceModelV0) upgrade() resourceModel {
	return resourceModel{
		ID:                   v.ID,
		ServiceID:            v.ServiceID,
		ServiceType:          v.ServiceType,
		ComposeDatabaseType:  types.StringNull(),
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

// resourceModelV1 is the schema version 1 state shape: the current model
// minus compose_database_type, which version 2 (this release) added.
//
// Every version 1 state necessarily has ServiceType != "compose": Dokploy's
// backup.create schema never accepted a literal "compose" databaseType (see
// databaseTypeFor), so a compose-parented dokploy_backup could never
// successfully apply before this release, and no other combination of
// service_type/service_id described a compose parent. There is nothing to
// backfill; the upgrade sets compose_database_type to null.
type resourceModelV1 struct {
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

func (v resourceModelV1) upgrade() resourceModel {
	return resourceModel{
		ID:                   v.ID,
		ServiceID:            v.ServiceID,
		ServiceType:          v.ServiceType,
		ComposeDatabaseType:  types.StringNull(),
		Database:             v.Database,
		Prefix:               v.Prefix,
		CronExpression:       v.CronExpression,
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

// databaseTypeFor derives the wire `databaseType`: the real database engine,
// regardless of whether the parent is a standalone database resource or a
// dokploy_compose service.
//
// Dokploy's backup.create schema (packages/server/src/db/schema/backups.ts,
// v0.30.6) only ever accepts databaseType in {postgres, mariadb, mysql,
// mongo, web-server, libsql} — never "compose". backupType is what tells
// Dokploy the parent is a compose service; databaseType still has to name the
// engine actually running inside it, because runComposeBackup
// (packages/server/src/utils/backups/compose.ts) reads databaseType to build
// the dump command regardless of backupType. service_type alone cannot carry
// both pieces of information for a compose parent, so compose_database_type
// supplies the engine in that case.
func databaseTypeFor(m resourceModel) string {
	if m.ServiceType.ValueString() == "compose" {
		return m.ComposeDatabaseType.ValueString()
	}
	return m.ServiceType.ValueString()
}

func flatten(b *client.Backup, out *resourceModel) {
	out.ID = types.StringValue(b.BackupID)

	// A compose parent's wire record carries the real engine in
	// databaseType (see databaseTypeFor) and "compose" only in backupType.
	// service_type has to read back "compose" here regardless of the
	// engine, with the engine itself moved to compose_database_type.
	if b.BackupType == "compose" {
		out.ServiceType = types.StringValue("compose")
		out.ComposeDatabaseType = types.StringValue(b.DatabaseType)
	} else {
		out.ServiceType = types.StringValue(b.DatabaseType)
		out.ComposeDatabaseType = types.StringNull()
	}

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
		DatabaseType:         databaseTypeFor(m),
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
// record coherent. It is never taken from user input: service_type and
// compose_database_type are both RequiresReplace, so plan and state always
// agree on them here.
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
		DatabaseType:         databaseTypeFor(m),
		Enabled:              m.Enabled.ValueBoolPointer(),
		KeepLatestCount:      m.KeepLatestCount.ValueInt64Pointer(),
		IncludeEncryptionKey: m.IncludeEncryptionKey.ValueBool(),
		ServiceName:          m.ServiceName.ValueStringPointer(),
		Metadata:             nil,
	}
}
