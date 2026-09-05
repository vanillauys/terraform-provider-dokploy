package registry

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestSchema_WriteOnlyCompanions(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&registryResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	plain, ok := resp.Schema.Attributes["password"].(schema.StringAttribute)
	if !ok || plain.Required || !plain.Optional || !plain.Sensitive {
		t.Errorf("password must be Optional+Sensitive, got %+v", resp.Schema.Attributes["password"])
	}
	wo, ok := resp.Schema.Attributes["password_wo"].(schema.StringAttribute)
	if !ok || !wo.WriteOnly || !wo.Sensitive || !wo.Optional {
		t.Errorf("password_wo must be Optional+WriteOnly+Sensitive, got %+v", resp.Schema.Attributes["password_wo"])
	}
}

// TestFlattenKeepsThePassword pins the read path: registry.one never returns
// the password, so flatten must not blank the state's copy.
func TestFlattenKeepsThePassword(t *testing.T) {
	m := resourceModel{Password: types.StringValue("kept")}
	flatten(&client.Registry{RegistryID: "r1", ImagePrefix: ""}, &m)
	if m.Password.ValueString() != "kept" {
		t.Errorf("flatten() changed password to %v", m.Password)
	}
	if !m.ImagePrefix.IsNull() {
		t.Errorf("flatten() image_prefix = %v, want null for \"\"", m.ImagePrefix)
	}
}
