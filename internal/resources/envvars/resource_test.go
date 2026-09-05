package envvars

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSchemaValidates(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&envVarsResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
}

func TestTargetOf(t *testing.T) {
	if got := targetOf(resourceModel{ComposeID: types.StringValue("c1")}); got.String() != "compose/c1" {
		t.Errorf("targetOf(compose) = %s", got)
	}
	if got := targetOf(resourceModel{ApplicationID: types.StringValue("a1"), ComposeID: types.StringNull()}); got.kind != "application" {
		t.Errorf("targetOf(application) = %s", got)
	}
	if got := targetOf(resourceModel{EnvironmentID: types.StringValue("e1")}); got.String() != "environment/e1" {
		t.Errorf("targetOf(environment) = %s", got)
	}
}
