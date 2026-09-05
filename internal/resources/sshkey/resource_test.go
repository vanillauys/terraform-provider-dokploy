package sshkey

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSchema_WriteOnlyCompanions pins the D1(a) shape on the private key,
// the RequiresReplace on its version, and runs the framework's own schema
// checks.
func TestSchema_WriteOnlyCompanions(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&sshKeyResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	plain, ok := resp.Schema.Attributes["private_key"].(schema.StringAttribute)
	if !ok || plain.Required || !plain.Optional || !plain.Sensitive || len(plain.PlanModifiers) == 0 {
		t.Errorf("private_key must be Optional+Sensitive+RequiresReplace, got %+v", resp.Schema.Attributes["private_key"])
	}
	wo, ok := resp.Schema.Attributes["private_key_wo"].(schema.StringAttribute)
	if !ok || !wo.WriteOnly || !wo.Sensitive || !wo.Optional {
		t.Errorf("private_key_wo must be Optional+WriteOnly+Sensitive, got %+v", resp.Schema.Attributes["private_key_wo"])
	}
	version, ok := resp.Schema.Attributes["private_key_wo_version"].(schema.Int64Attribute)
	if !ok || len(version.PlanModifiers) == 0 {
		t.Errorf("private_key_wo_version must carry RequiresReplace, got %+v", resp.Schema.Attributes["private_key_wo_version"])
	}
}

func TestHideWriteOnly(t *testing.T) {
	m := resourceModel{PrivateKey: types.StringValue("pem")}
	hideWriteOnly(&m, map[string]bool{"private_key": true})
	if !m.PrivateKey.IsNull() {
		t.Errorf("hideWriteOnly() left private_key = %v", m.PrivateKey)
	}
	m = resourceModel{PrivateKey: types.StringValue("pem")}
	hideWriteOnly(&m, map[string]bool{})
	if m.PrivateKey.ValueString() != "pem" {
		t.Errorf("hideWriteOnly() with no companion in use changed private_key to %v", m.PrivateKey)
	}
}

func TestDescriptionRequest(t *testing.T) {
	if got := descriptionRequest(types.StringNull()); got != nil {
		t.Errorf("null description = %q, want nil (an explicit JSON null clears it)", *got)
	}
	if got := descriptionRequest(types.StringValue("d")); got == nil || *got != "d" {
		t.Errorf("description = %v, want \"d\"", got)
	}
}
