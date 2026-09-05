package certificate

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSchema_WriteOnlyCompanions(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&certificateResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	plain, ok := resp.Schema.Attributes["private_key"].(schema.StringAttribute)
	if !ok || plain.Required || !plain.Optional || !plain.Sensitive {
		t.Errorf("private_key must be Optional+Sensitive, got %+v", resp.Schema.Attributes["private_key"])
	}
	wo, ok := resp.Schema.Attributes["private_key_wo"].(schema.StringAttribute)
	if !ok || !wo.WriteOnly || !wo.Sensitive || !wo.Optional {
		t.Errorf("private_key_wo must be Optional+WriteOnly+Sensitive, got %+v", resp.Schema.Attributes["private_key_wo"])
	}
	if _, ok := resp.Schema.Attributes["private_key_wo_version"].(schema.Int64Attribute); !ok {
		t.Errorf("private_key_wo_version is %T", resp.Schema.Attributes["private_key_wo_version"])
	}
}

func TestHideWriteOnly(t *testing.T) {
	m := resourceModel{PrivateKey: types.StringValue("pem")}
	hideWriteOnly(&m, map[string]bool{"private_key": true})
	if !m.PrivateKey.IsNull() {
		t.Errorf("hideWriteOnly() left private_key = %v", m.PrivateKey)
	}
}
