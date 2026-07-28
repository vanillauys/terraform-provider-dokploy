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
