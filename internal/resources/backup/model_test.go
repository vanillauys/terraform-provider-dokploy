package backup

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }

// TestFlattenEmptyStringsBecomeNull asserts the UI-storage case: Dokploy
// returns a literal "" for an optional string that was set and then cleared
// through the Dokploy UI, where a field never set returns null. Terraform
// configuration that omits the attribute holds null either way, so a model
// that preserved "" would produce a `"" -> null` diff no apply can settle.
//
// This resource has never been round-tripped against a UI-created record: it
// shipped after the acf76ab sweep and the acceptance rig creates every record
// through the API, which only ever produces null. This test is what stands in
// for that observation. The structural half is
// TestNoStringPointerValueOutsideExemptions in internal/tfutil.
//
// service_name is the only optional string client.Backup carries. database,
// prefix, cron_expression and destination_id are all Required in the schema, so
// configuration always supplies a value and a server-side "" cannot produce
// the null mismatch this test is about.
func TestFlattenEmptyStringsBecomeNull(t *testing.T) {
	b := &client.Backup{
		BackupID: "b1", Schedule: "0 3 * * *", Database: "app",
		Prefix: "app/", DestinationID: "dest1", DatabaseType: "postgres",
		PostgresID: strPtr("pg1"),

		// The only optional string, carrying "" rather than nil.
		ServiceName: strPtr(""),
	}

	var out resourceModel
	flatten(b, &out)

	for name, got := range map[string]types.String{
		"service_name": out.ServiceName,
	} {
		if !got.IsNull() {
			t.Errorf("%s = %q, want null: a \"\" from the server must collapse to null", name, got.ValueString())
		}
	}
}

// service_id reaches the model through ParentRef rather than StringOrNull, so
// the "" case has to be pinned separately: a parent column holding "" instead
// of null must still read as a null service_id, not as an empty string that
// no configuration can match.
func TestFlattenEmptyParentColumnBecomesNullServiceID(t *testing.T) {
	b := &client.Backup{
		BackupID: "b1", DatabaseType: "postgres", PostgresID: strPtr(""),
	}

	var out resourceModel
	flatten(b, &out)

	if !out.ServiceID.IsNull() {
		t.Errorf("service_id = %q, want null: an empty parent column means unset", out.ServiceID.ValueString())
	}
}

// TestCreateRequestComposeParentSendsRealEngineAsDatabaseType pins the fix
// for issue #45: a compose-parented backup must send the real database
// engine as databaseType (Dokploy's backup.create schema never accepts a
// literal "compose" there — see databaseTypeFor), "compose" as backupType,
// and the parent id under composeId, with every other per-type id column
// left nil.
func TestCreateRequestComposeParentSendsRealEngineAsDatabaseType(t *testing.T) {
	m := resourceModel{
		ServiceID:            types.StringValue("compose1"),
		ServiceType:          types.StringValue("compose"),
		ComposeDatabaseType:  types.StringValue("mariadb"),
		Database:             types.StringValue("app"),
		Prefix:               types.StringValue("backups/app/"),
		CronExpression:       types.StringValue("0 3 * * *"),
		DestinationID:        types.StringValue("d1"),
		Enabled:              types.BoolValue(true),
		IncludeEncryptionKey: types.BoolValue(true),
	}

	req := createRequest(m, nil)

	if req.DatabaseType != "mariadb" {
		t.Errorf("DatabaseType = %q, want %q: the real engine, not the literal service_type", req.DatabaseType, "mariadb")
	}
	if req.BackupType != "compose" {
		t.Errorf("BackupType = %q, want %q", req.BackupType, "compose")
	}
	if req.ComposeID == nil || *req.ComposeID != "compose1" {
		t.Errorf("ComposeID = %v, want a pointer to %q", req.ComposeID, "compose1")
	}
	for name, col := range map[string]*string{
		"PostgresID": req.PostgresID,
		"MysqlID":    req.MysqlID,
		"MariadbID":  req.MariadbID,
		"MongoID":    req.MongoID,
		"LibsqlID":   req.LibsqlID,
	} {
		if col != nil {
			t.Errorf("%s = %q, want nil: only composeId may be populated for a compose parent", name, *col)
		}
	}
}

// TestCreateRequestDatabaseParentUnchanged pins the non-compose case exactly
// as it worked before this fix: databaseType is service_type verbatim, and
// compose_database_type plays no part.
func TestCreateRequestDatabaseParentUnchanged(t *testing.T) {
	m := resourceModel{
		ServiceID:      types.StringValue("pg1"),
		ServiceType:    types.StringValue("postgres"),
		Database:       types.StringValue("app"),
		Prefix:         types.StringValue("backups/app/"),
		CronExpression: types.StringValue("0 3 * * *"),
		DestinationID:  types.StringValue("d1"),
	}

	req := createRequest(m, nil)

	if req.DatabaseType != "postgres" {
		t.Errorf("DatabaseType = %q, want %q", req.DatabaseType, "postgres")
	}
	if req.BackupType != "database" {
		t.Errorf("BackupType = %q, want %q", req.BackupType, "database")
	}
	if req.PostgresID == nil || *req.PostgresID != "pg1" {
		t.Errorf("PostgresID = %v, want a pointer to %q", req.PostgresID, "pg1")
	}
	if req.ComposeID != nil {
		t.Errorf("ComposeID = %q, want nil", *req.ComposeID)
	}
}

// TestFlattenComposeParentSplitsServiceTypeAndEngine pins the read side: a
// wire record whose backupType is "compose" must read back service_type as
// "compose" with the real engine moved to compose_database_type, not
// service_type holding the engine directly.
func TestFlattenComposeParentSplitsServiceTypeAndEngine(t *testing.T) {
	b := &client.Backup{
		BackupID: "b1", Schedule: "0 3 * * *", Database: "app",
		Prefix: "app/", DestinationID: "dest1",
		DatabaseType: "mariadb", BackupType: "compose",
		ComposeID: strPtr("compose1"),
	}

	var out resourceModel
	flatten(b, &out)

	if out.ServiceType.ValueString() != "compose" {
		t.Errorf("service_type = %q, want %q", out.ServiceType.ValueString(), "compose")
	}
	if out.ComposeDatabaseType.ValueString() != "mariadb" {
		t.Errorf("compose_database_type = %q, want %q", out.ComposeDatabaseType.ValueString(), "mariadb")
	}
	if out.ServiceID.ValueString() != "compose1" {
		t.Errorf("service_id = %q, want %q", out.ServiceID.ValueString(), "compose1")
	}
}

// TestFlattenDatabaseParentLeavesComposeDatabaseTypeNull pins the unchanged
// case: a database-parented record reads compose_database_type as null.
func TestFlattenDatabaseParentLeavesComposeDatabaseTypeNull(t *testing.T) {
	b := &client.Backup{
		BackupID: "b1", Schedule: "0 3 * * *", Database: "app",
		Prefix: "app/", DestinationID: "dest1",
		DatabaseType: "postgres", BackupType: "database",
		PostgresID: strPtr("pg1"),
	}

	var out resourceModel
	flatten(b, &out)

	if out.ServiceType.ValueString() != "postgres" {
		t.Errorf("service_type = %q, want %q", out.ServiceType.ValueString(), "postgres")
	}
	if !out.ComposeDatabaseType.IsNull() {
		t.Errorf("compose_database_type = %v, want null", out.ComposeDatabaseType)
	}
}

// TestMetadataForEachEngine pins the wire shape of the compose credentials
// (issue #71): only the key of the engine goes out, with exactly the fields
// that Dokploy's dump command for that engine reads, and a database parent
// sends null.
func TestMetadataForEachEngine(t *testing.T) {
	m := resourceModel{
		ServiceType:         types.StringValue("compose"),
		ComposeDatabaseUser: types.StringValue("app"),
	}
	cases := []struct {
		engine string
		want   *client.BackupMetadata
	}{
		{"postgres", &client.BackupMetadata{Postgres: &client.BackupUserMetadata{DatabaseUser: "app"}}},
		{"mariadb", &client.BackupMetadata{Mariadb: &client.BackupUserPasswordMetadata{DatabaseUser: "app", DatabasePassword: "pw"}}},
		{"mongo", &client.BackupMetadata{Mongo: &client.BackupUserPasswordMetadata{DatabaseUser: "app", DatabasePassword: "pw"}}},
		{"mysql", &client.BackupMetadata{Mysql: &client.BackupRootPasswordMetadata{DatabaseRootPassword: "root"}}},
		{"libsql", nil},
	}
	for _, tc := range cases {
		t.Run(tc.engine, func(t *testing.T) {
			m.ComposeDatabaseType = types.StringValue(tc.engine)
			got := metadataFor(m, "pw", "root")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("metadataFor(%s) = %s, want %s", tc.engine, jsonOf(t, got), jsonOf(t, tc.want))
			}
		})
	}
	t.Run("database parent", func(t *testing.T) {
		db := resourceModel{ServiceType: types.StringValue("postgres"), ComposeDatabaseUser: types.StringValue("app")}
		if got := metadataFor(db, "pw", "root"); got != nil {
			t.Errorf("metadataFor(database parent) = %s, want nil", jsonOf(t, got))
		}
	})
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestFlattenReadsComposeCredentials pins the read side: the entry that the
// record's databaseType names lands in the credential attributes, a field
// the engine has no use for reads as null, and a stray entry of another
// engine is ignored.
func TestFlattenReadsComposeCredentials(t *testing.T) {
	b := &client.Backup{
		BackupID: "b1", DatabaseType: "mariadb", BackupType: "compose", ComposeID: strPtr("compose1"),
		Metadata: &client.BackupMetadata{
			Mariadb: &client.BackupUserPasswordMetadata{DatabaseUser: "app", DatabasePassword: "pw"},
			Mysql:   &client.BackupRootPasswordMetadata{DatabaseRootPassword: "stray"},
		},
	}
	var out resourceModel
	flatten(b, &out)
	if out.ComposeDatabaseUser.ValueString() != "app" {
		t.Errorf("compose_database_user = %v, want app", out.ComposeDatabaseUser)
	}
	if out.ComposeDatabasePassword.ValueString() != "pw" {
		t.Errorf("compose_database_password = %v, want pw", out.ComposeDatabasePassword)
	}
	if !out.ComposeDatabaseRootPassword.IsNull() {
		t.Errorf("compose_database_root_password = %v, want null: mariadb has no root password", out.ComposeDatabaseRootPassword)
	}
}

// TestFlattenLeavesCredentialsNullWithoutMetadata covers the records that
// predate v1.7.0 and the database parents: metadata null or {} reads as
// three null attributes, never as "".
func TestFlattenLeavesCredentialsNullWithoutMetadata(t *testing.T) {
	cases := []struct {
		name string
		b    *client.Backup
	}{
		{"compose without metadata", &client.Backup{DatabaseType: "postgres", BackupType: "compose", ComposeID: strPtr("c1")}},
		{"compose with empty metadata", &client.Backup{DatabaseType: "postgres", BackupType: "compose", ComposeID: strPtr("c1"), Metadata: &client.BackupMetadata{}}},
		{"database parent with metadata", &client.Backup{DatabaseType: "postgres", BackupType: "database", PostgresID: strPtr("pg1"),
			Metadata: &client.BackupMetadata{Postgres: &client.BackupUserMetadata{DatabaseUser: "ignored"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out resourceModel
			flatten(tc.b, &out)
			for name, v := range map[string]types.String{
				"compose_database_user":          out.ComposeDatabaseUser,
				"compose_database_password":      out.ComposeDatabasePassword,
				"compose_database_root_password": out.ComposeDatabaseRootPassword,
			} {
				if !v.IsNull() {
					t.Errorf("%s = %v, want null", name, v)
				}
			}
		})
	}
}
