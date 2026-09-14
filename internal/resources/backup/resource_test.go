package backup

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestUpgradeStateV0 feeds a version 0 state, which names the cron attribute
// `schedule` and lacks `compose_database_type`, through the upgrader and
// asserts that every field arrives in the current model with the cron
// expression under `cron_expression` and compose_database_type null.
func TestUpgradeStateV0(t *testing.T) {
	ctx := context.Background()

	prior := schemaV0(ctx)
	if prior.Version != 0 {
		t.Fatalf("prior schema version = %d, want 0", prior.Version)
	}
	if _, ok := prior.Attributes["schedule"]; !ok {
		t.Fatal("prior schema lacks schedule")
	}
	if _, ok := prior.Attributes["cron_expression"]; ok {
		t.Fatal("prior schema must not have cron_expression")
	}
	if _, ok := prior.Attributes["compose_database_type"]; ok {
		t.Fatal("prior schema must not have compose_database_type")
	}

	v0 := resourceModelV0{
		ID:                   types.StringValue("b1"),
		ServiceID:            types.StringValue("pg1"),
		ServiceType:          types.StringValue("postgres"),
		Database:             types.StringValue("app"),
		Prefix:               types.StringValue("backups/app/"),
		Schedule:             types.StringValue("0 3 * * *"),
		DestinationID:        types.StringValue("d1"),
		Enabled:              types.BoolValue(true),
		KeepLatestCount:      types.Int64Value(5),
		IncludeEncryptionKey: types.BoolValue(true),
		ServiceName:          types.StringNull(),
		AppName:              types.StringValue("app-backup-x"),
	}
	priorState := tfsdk.State{Schema: prior}
	if d := priorState.Set(ctx, v0); d.HasError() {
		t.Fatalf("prior state: %v", d)
	}

	var current resource.SchemaResponse
	(&backupResource{}).Schema(ctx, resource.SchemaRequest{}, &current)
	if current.Schema.Version != 2 {
		t.Fatalf("current schema version = %d, want 2", current.Schema.Version)
	}
	resp := resource.UpgradeStateResponse{State: tfsdk.State{Schema: current.Schema}}

	upgraders := (&backupResource{}).UpgradeState(ctx)
	upgrader, ok := upgraders[0]
	if !ok {
		t.Fatal("no upgrader registered for version 0")
	}
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{State: &priorState}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade: %v", resp.Diagnostics)
	}

	var got resourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatalf("read upgraded state: %v", d)
	}
	if got.CronExpression.ValueString() != "0 3 * * *" {
		t.Errorf("cron_expression = %v, want the prior schedule", got.CronExpression)
	}
	if !got.ComposeDatabaseType.IsNull() {
		t.Errorf("compose_database_type = %v, want null", got.ComposeDatabaseType)
	}
	if want := v0.upgrade(); got != want {
		t.Errorf("upgraded model = %+v, want %+v", got, want)
	}
}

// TestUpgradeStateV1 feeds a version 1 state, which has `cron_expression`
// but lacks `compose_database_type` (added in version 2), through the
// upgrader and asserts every field arrives unchanged with
// compose_database_type null.
func TestUpgradeStateV1(t *testing.T) {
	ctx := context.Background()

	prior := schemaV1(ctx)
	if prior.Version != 1 {
		t.Fatalf("prior schema version = %d, want 1", prior.Version)
	}
	if _, ok := prior.Attributes["cron_expression"]; !ok {
		t.Fatal("prior schema lacks cron_expression")
	}
	if _, ok := prior.Attributes["compose_database_type"]; ok {
		t.Fatal("prior schema must not have compose_database_type")
	}

	v1 := resourceModelV1{
		ID:                   types.StringValue("b1"),
		ServiceID:            types.StringValue("pg1"),
		ServiceType:          types.StringValue("postgres"),
		Database:             types.StringValue("app"),
		Prefix:               types.StringValue("backups/app/"),
		CronExpression:       types.StringValue("0 3 * * *"),
		DestinationID:        types.StringValue("d1"),
		Enabled:              types.BoolValue(true),
		KeepLatestCount:      types.Int64Value(5),
		IncludeEncryptionKey: types.BoolValue(true),
		ServiceName:          types.StringNull(),
		AppName:              types.StringValue("app-backup-x"),
	}
	priorState := tfsdk.State{Schema: prior}
	if d := priorState.Set(ctx, v1); d.HasError() {
		t.Fatalf("prior state: %v", d)
	}

	var current resource.SchemaResponse
	(&backupResource{}).Schema(ctx, resource.SchemaRequest{}, &current)
	resp := resource.UpgradeStateResponse{State: tfsdk.State{Schema: current.Schema}}

	upgraders := (&backupResource{}).UpgradeState(ctx)
	upgrader, ok := upgraders[1]
	if !ok {
		t.Fatal("no upgrader registered for version 1")
	}
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{State: &priorState}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade: %v", resp.Diagnostics)
	}

	var got resourceModel
	if d := resp.State.Get(ctx, &got); d.HasError() {
		t.Fatalf("read upgraded state: %v", d)
	}
	if !got.ComposeDatabaseType.IsNull() {
		t.Errorf("compose_database_type = %v, want null", got.ComposeDatabaseType)
	}
	if want := v1.upgrade(); got != want {
		t.Errorf("upgraded model = %+v, want %+v", got, want)
	}
}

// TestValidateComposeDatabaseType pins the pairing rule between service_type
// and compose_database_type: required for a compose parent, forbidden
// otherwise.
func TestValidateComposeDatabaseType(t *testing.T) {
	cases := []struct {
		name        string
		serviceType string
		composeType types.String
		wantErr     bool
	}{
		{"compose with engine", "compose", types.StringValue("mariadb"), false},
		{"compose without engine", "compose", types.StringNull(), true},
		{"compose with empty engine", "compose", types.StringValue(""), true},
		{"postgres without engine", "postgres", types.StringNull(), false},
		{"postgres with engine set", "postgres", types.StringValue("mariadb"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := resourceModel{
				ServiceType:         types.StringValue(tc.serviceType),
				ComposeDatabaseType: tc.composeType,
			}
			err := validateComposeDatabaseType(m)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateComposeDatabaseType(%+v) error = %v, wantErr %v", m, err, tc.wantErr)
			}
		})
	}
}
