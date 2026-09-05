package destination

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSchema_WriteOnlyCompanions pins the D1(a) shape on both credentials
// and runs the framework's own schema checks, which only an acceptance run
// reached before.
func TestSchema_WriteOnlyCompanions(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&destinationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	for _, name := range secretNames {
		plain, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || plain.Required || !plain.Optional || !plain.Sensitive {
			t.Errorf("%s must be Optional+Sensitive, got %+v", name, resp.Schema.Attributes[name])
		}
		wo, ok := resp.Schema.Attributes[name+"_wo"].(schema.StringAttribute)
		if !ok || !wo.WriteOnly || !wo.Sensitive || !wo.Optional {
			t.Errorf("%s_wo must be Optional+WriteOnly+Sensitive, got %+v", name, resp.Schema.Attributes[name+"_wo"])
		}
		if _, ok := resp.Schema.Attributes[name+"_wo_version"].(schema.Int64Attribute); !ok {
			t.Errorf("%s_wo_version is %T", name, resp.Schema.Attributes[name+"_wo_version"])
		}
	}
}

func TestHideWriteOnly(t *testing.T) {
	m := resourceModel{AccessKey: types.StringValue("AKIA"), SecretAccessKey: types.StringValue("secret")}
	hideWriteOnly(&m, map[string]bool{"secret_access_key": true})
	if m.AccessKey.ValueString() != "AKIA" || !m.SecretAccessKey.IsNull() {
		t.Errorf("hideWriteOnly() = %+v, want only secret_access_key null", m)
	}
}
