package apikey

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestSchemaValidatesAndEveryInputReplaces(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&apiKeyResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	for name, a := range resp.Schema.Attributes {
		var configurable, replaces bool
		switch v := a.(type) {
		case schema.StringAttribute:
			configurable, replaces = v.Required || v.Optional, len(v.PlanModifiers) > 0
		case schema.Int64Attribute:
			configurable, replaces = v.Required || v.Optional, len(v.PlanModifiers) > 0
		case schema.BoolAttribute:
			configurable, replaces = v.Required || v.Optional, len(v.PlanModifiers) > 0
		}
		if configurable && !replaces {
			t.Errorf("%s is configurable but carries no plan modifier; Dokploy has no update path for API keys", name)
		}
	}
	key := resp.Schema.Attributes["key"].(schema.StringAttribute)
	if !key.Sensitive || !key.Computed {
		t.Errorf("key must be Computed+Sensitive, got %+v", key)
	}
}
