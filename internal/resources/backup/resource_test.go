package backup

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestUpgradeStateV0 feeds a version 0 state, which names the cron attribute
// `schedule`, through the upgrader and asserts that every field arrives in
// the current model with the cron expression under `cron_expression`.
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
	if current.Schema.Version != 1 {
		t.Fatalf("current schema version = %d, want 1", current.Schema.Version)
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
	if want := v0.upgrade(); got != want {
		t.Errorf("upgraded model = %+v, want %+v", got, want)
	}
}
