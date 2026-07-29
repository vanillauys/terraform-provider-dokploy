package schedule

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func TestFlattenReadsTheParentNamedByScheduleType(t *testing.T) {
	// The corrupt shape schedule.update can produce: two parent columns set.
	// The discriminator decides which one is real.
	s := &client.Schedule{
		ScheduleID: "s1", Name: "nightly", CronExpression: "0 3 * * *",
		Command: "echo hi", ShellType: "bash", ScheduleType: "compose",
		ApplicationID: strPtr("stale-app"), ComposeID: strPtr("comp1"),
		Enabled: boolPtr(true),
	}
	var out resourceModel
	flatten(s, &out)

	if out.ServiceID.ValueString() != "comp1" {
		t.Errorf("service_id = %q, want comp1", out.ServiceID.ValueString())
	}
	if out.ScheduleType.ValueString() != "compose" {
		t.Errorf("schedule_type = %q", out.ScheduleType.ValueString())
	}
}

// A dokploy-server schedule has no parent at all. service_id must come back
// null, matching what configuration is required to say.
func TestFlattenNullsServiceIDForDokployServer(t *testing.T) {
	s := &client.Schedule{ScheduleID: "s1", ScheduleType: "dokploy-server", Enabled: boolPtr(true)}
	var out resourceModel
	flatten(s, &out)
	if !out.ServiceID.IsNull() {
		t.Errorf("service_id = %v, want null: dokploy-server has no parent service", out.ServiceID)
	}
}

// enabled is nullable server-side but the schema defaults it to true, so a
// null read must resolve to a concrete bool rather than leaving state
// unknown. Dokploy only produces null for schedules created outside this
// provider.
func TestFlattenResolvesNullEnabled(t *testing.T) {
	var out resourceModel
	flatten(&client.Schedule{ScheduleID: "s1", ScheduleType: "dokploy-server"}, &out)
	if out.Enabled.IsNull() || out.Enabled.IsUnknown() {
		t.Errorf("enabled = %v, want a concrete bool", out.Enabled)
	}
	if out.Enabled.ValueBool() {
		t.Error("a server-side null enabled should read as false, not true")
	}
}

func TestCreateRequestPopulatesExactlyOneParentColumn(t *testing.T) {
	for _, tc := range []struct {
		schedType, id, wantCol string
	}{
		{"application", "app1", "applicationId"},
		{"compose", "comp1", "composeId"},
		{"server", "srv1", "serverId"},
		{"dokploy-server", "", ""},
	} {
		t.Run(tc.schedType, func(t *testing.T) {
			req := createRequest(resourceModel{
				Name:         types.StringValue("n"),
				ScheduleType: types.StringValue(tc.schedType),
				ServiceID:    types.StringValue(tc.id),
			})
			got := map[string]*string{
				"applicationId": req.ApplicationID,
				"composeId":     req.ComposeID,
				"serverId":      req.ServerID,
			}
			var set []string
			for col, v := range got {
				if v != nil {
					set = append(set, col)
					if *v != tc.id {
						t.Errorf("%s = %q, want %q", col, *v, tc.id)
					}
				}
			}
			if tc.wantCol == "" {
				if len(set) != 0 {
					t.Errorf("populated %v, want none for dokploy-server", set)
				}
				return
			}
			if len(set) != 1 || set[0] != tc.wantCol {
				t.Errorf("populated %v, want exactly [%s]", set, tc.wantCol)
			}
		})
	}
}

func TestValidateParent(t *testing.T) {
	m := func(t, id string) resourceModel {
		return resourceModel{ScheduleType: types.StringValue(t), ServiceID: types.StringValue(id)}
	}
	null := func(t string) resourceModel {
		return resourceModel{ScheduleType: types.StringValue(t), ServiceID: types.StringNull()}
	}
	for name, tc := range map[string]struct {
		model   resourceModel
		wantErr bool
	}{
		"application with id":       {m("application", "app1"), false},
		"application without id":    {null("application"), true},
		"server without id":         {null("server"), true},
		"dokploy-server without id": {null("dokploy-server"), false},
		"dokploy-server WITH an id": {m("dokploy-server", "app1"), true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateParent(tc.model); (err != nil) != tc.wantErr {
				t.Errorf("validateParent = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

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
// The field list is every optional string client.Schedule carries, not a
// sample: checking three of five is how domain/model.go's two call sites
// survived the acf76ab sweep.
func TestFlattenEmptyStringsBecomeNull(t *testing.T) {
	s := &client.Schedule{
		ScheduleID: "s1", Name: "nightly", CronExpression: "0 3 * * *",
		Command: "echo hi", ShellType: "bash", ScheduleType: "application",
		Enabled: boolPtr(true),

		// Every optional string carries "" rather than nil.
		Description: strPtr(""),
		Script:      strPtr(""),
		Timezone:    strPtr(""),
		ServiceName: strPtr(""),
	}

	var out resourceModel
	flatten(s, &out)

	for name, got := range map[string]types.String{
		"description":  out.Description,
		"script":       out.Script,
		"timezone":     out.Timezone,
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
	s := &client.Schedule{
		ScheduleID: "s1", ScheduleType: "application",
		ApplicationID: strPtr(""), Enabled: boolPtr(true),
	}

	var out resourceModel
	flatten(s, &out)

	if !out.ServiceID.IsNull() {
		t.Errorf("service_id = %q, want null: an empty parent column means unset", out.ServiceID.ValueString())
	}
}
