package userpermissions

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestSchemaValidates(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&permissionsResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
}

// TestRequestSendsEmptyListsNotNull pins the dialect A body: a null set
// must reach the wire as [], because the endpoint requires every list.
func TestRequestSendsEmptyListsNotNull(t *testing.T) {
	var diags diag.Diagnostics
	req := request(context.Background(), resourceModel{UserID: types.StringValue("u1")}, &diags)
	if diags.HasError() || req.ID != "u1" || req.AccessedProjects == nil || len(req.AccessedProjects) != 0 || req.AccessedServers == nil {
		t.Errorf("request() = %+v, %v", req, diags)
	}
}

func TestFlattenMapsEveryFlag(t *testing.T) {
	var diags diag.Diagnostics
	var m resourceModel
	flatten(context.Background(), &m, &client.Member{ID: "m1", UserID: "u1", CanAccessToAPI: true, CanDeleteServices: true,
		AccessedProjects: []string{"p1"}}, &diags)
	if diags.HasError() || m.ID.ValueString() != "u1" || m.MemberID.ValueString() != "m1" || !m.CanAccessToAPI.ValueBool() ||
		!m.CanDeleteServices.ValueBool() || m.CanCreateProjects.ValueBool() || len(m.AccessedProjects.Elements()) != 1 ||
		m.AccessedServers.IsNull() || len(m.AccessedServers.Elements()) != 0 {
		t.Errorf("flatten() = %+v, %v", m, diags)
	}
}
