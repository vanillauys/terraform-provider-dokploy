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

	// The credentials of a compose parent (client.BackupMetadata). The
	// write-only companions (tfutil.WriteOnlyCompanions) carry a value in
	// the config only; the plan and the state hold null for them.
	ComposeDatabaseUser                  types.String `tfsdk:"compose_database_user"`
	ComposeDatabasePassword              types.String `tfsdk:"compose_database_password"`
	ComposeDatabasePasswordWo            types.String `tfsdk:"compose_database_password_wo"`
	ComposeDatabasePasswordWoVersion     types.Int64  `tfsdk:"compose_database_password_wo_version"`
	ComposeDatabaseRootPassword          types.String `tfsdk:"compose_database_root_password"`
	ComposeDatabaseRootPasswordWo        types.String `tfsdk:"compose_database_root_password_wo"`
	ComposeDatabaseRootPasswordWoVersion types.Int64  `tfsdk:"compose_database_root_password_wo_version"`
}

// credentialAttributes lists the compose credential attributes that v1.7.0
// added. A version 0 or version 1 state never carried them, and the schema
// version did not move: the framework reads a new optional attribute as
// null, and the two upgraders leave the fields at their zero value, which is
// null.
var credentialAttributes = []string{
	"compose_database_user",
	"compose_database_password", "compose_database_password_wo", "compose_database_password_wo_version",
	"compose_database_root_password", "compose_database_root_password_wo", "compose_database_root_password_wo_version",
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
// v0.30.7) only ever accepts databaseType in {postgres, mariadb, mysql,
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

// composeEngine returns the engine inside a compose parent, and "" for a
// database parent: the credential attributes apply to a compose parent only,
// because a database parent carries its own credentials (client.BackupMetadata).
func composeEngine(m resourceModel) string {
	if m.ServiceType.ValueString() != "compose" {
		return ""
	}
	return m.ComposeDatabaseType.ValueString()
}

// credentialNeeds says which credential attributes an engine inside a
// compose parent needs. The shape follows Dokploy's dump commands
// (client.BackupMetadata): every field that is false must stay unset.
type credentialNeeds struct {
	user, password, rootPassword bool
}

func needsFor(engine string) credentialNeeds {
	switch engine {
	case "postgres":
		return credentialNeeds{user: true}
	case "mariadb", "mongo":
		return credentialNeeds{user: true, password: true}
	case "mysql":
		return credentialNeeds{rootPassword: true}
	}
	return credentialNeeds{}
}

// metadataFor builds the wire metadata of a compose backup, and nil for a
// database parent. password and rootPassword are the resolved secrets: the
// plain attribute, the write-only companion, or the stored value that an
// update resends (resource.go).
func metadataFor(m resourceModel, password, rootPassword string) *client.BackupMetadata {
	user := m.ComposeDatabaseUser.ValueString()
	switch composeEngine(m) {
	case "postgres":
		return &client.BackupMetadata{Postgres: &client.BackupUserMetadata{DatabaseUser: user}}
	case "mariadb":
		return &client.BackupMetadata{Mariadb: &client.BackupUserPasswordMetadata{DatabaseUser: user, DatabasePassword: password}}
	case "mongo":
		return &client.BackupMetadata{Mongo: &client.BackupUserPasswordMetadata{DatabaseUser: user, DatabasePassword: password}}
	case "mysql":
		return &client.BackupMetadata{Mysql: &client.BackupRootPasswordMetadata{DatabaseRootPassword: rootPassword}}
	}
	return nil
}

// credentials is the read side of metadataFor: the entry of the engine that
// the record's databaseType names. A field the engine has no use for, or an
// absent entry, reads as "".
type credentials struct {
	user, password, rootPassword string
}

func storedCredentials(b *client.Backup) credentials {
	if b.BackupType != "compose" || b.Metadata == nil {
		return credentials{}
	}
	md := b.Metadata
	switch b.DatabaseType {
	case "postgres":
		if md.Postgres != nil {
			return credentials{user: md.Postgres.DatabaseUser}
		}
	case "mariadb":
		if md.Mariadb != nil {
			return credentials{user: md.Mariadb.DatabaseUser, password: md.Mariadb.DatabasePassword}
		}
	case "mongo":
		if md.Mongo != nil {
			return credentials{user: md.Mongo.DatabaseUser, password: md.Mongo.DatabasePassword}
		}
	case "mysql":
		if md.Mysql != nil {
			return credentials{rootPassword: md.Mysql.DatabaseRootPassword}
		}
	}
	return credentials{}
}

// stringOrNull maps a stored "" to null: an optional attribute that the
// configuration omits holds null, and a `"" -> null` diff cannot be applied
// away (tfutil.StringOrNull says the same for a pointer).
func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
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

	// The server returns the compose credentials in cleartext. The caller
	// nulls a secret whose write-only companion is in use (hideWriteOnly).
	// The companions themselves are never read back.
	creds := storedCredentials(b)
	out.ComposeDatabaseUser = stringOrNull(creds.user)
	out.ComposeDatabasePassword = stringOrNull(creds.password)
	out.ComposeDatabaseRootPassword = stringOrNull(creds.rootPassword)

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

// createRequest and updateRequest take the wire metadata from the caller,
// because the secrets inside it come from the config (the write-only
// companions) or from the stored record, not from the plan alone.
func createRequest(m resourceModel, metadata *client.BackupMetadata) client.CreateBackupRequest {
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
		Metadata:             metadata,
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
// metadata replaces the stored object as a whole on backup.update, so the
// caller passes the full set of credentials, the password included.
func updateRequest(m resourceModel, metadata *client.BackupMetadata) client.UpdateBackupRequest {
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
		Metadata:             metadata,
	}
}
