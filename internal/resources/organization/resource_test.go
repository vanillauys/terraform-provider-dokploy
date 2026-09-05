package organization

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestSchemaValidates(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&organizationResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
}

func TestUpdateRequestOmitsAnUnsetDefaultRole(t *testing.T) {
	req := updateRequest("o1", resourceModel{Name: types.StringValue("n"), Logo: types.StringNull()})
	if req.DefaultRole != nil || req.Logo != "" || req.Name != "n" {
		t.Errorf("updateRequest() = %+v", req)
	}
	req = updateRequest("o1", resourceModel{Name: types.StringValue("n"), DefaultRole: types.StringValue("admin")})
	if req.DefaultRole == nil || *req.DefaultRole != "admin" {
		t.Errorf("updateRequest() default role = %v", req.DefaultRole)
	}
}

func TestFlattenCollapsesEmptyLogo(t *testing.T) {
	var m resourceModel
	flatten(&client.Organization{ID: "o1", Name: "n", Logo: ""}, &m)
	if !m.Logo.IsNull() || !m.DefaultRole.IsNull() || m.ID.ValueString() != "o1" {
		t.Errorf("flatten() = %+v", m)
	}
}
