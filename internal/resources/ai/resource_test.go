package ai

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
	(&aiResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	plain, ok := resp.Schema.Attributes["api_key"].(schema.StringAttribute)
	if !ok || plain.Required || !plain.Optional || !plain.Sensitive {
		t.Errorf("api_key must be Optional+Sensitive, got %+v", resp.Schema.Attributes["api_key"])
	}
	wo, ok := resp.Schema.Attributes["api_key_wo"].(schema.StringAttribute)
	if !ok || !wo.WriteOnly || !wo.Sensitive || !wo.Optional {
		t.Errorf("api_key_wo must be Optional+WriteOnly+Sensitive, got %+v", resp.Schema.Attributes["api_key_wo"])
	}
	if _, ok := resp.Schema.Attributes["api_key_wo_version"].(schema.Int64Attribute); !ok {
		t.Errorf("api_key_wo_version is %T", resp.Schema.Attributes["api_key_wo_version"])
	}
}

func TestHideWriteOnly(t *testing.T) {
	m := resourceModel{APIKey: types.StringValue("sk")}
	hideWriteOnly(&m, map[string]bool{"api_key": true})
	if !m.APIKey.IsNull() {
		t.Errorf("hideWriteOnly() left api_key = %v", m.APIKey)
	}
}
