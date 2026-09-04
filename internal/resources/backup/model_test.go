package backup

import (
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
