package tag

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
	(&tagResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
}

// A null colour on the server must flatten to a null attribute. A stored ""
// (written outside Terraform; the schema rejects it) reads back as null
// too, so that a configuration without the attribute plans nothing.
func TestFlattenColor(t *testing.T) {
	var m resourceModel
	flatten(&client.Tag{TagID: "t1", Name: "n", CreatedAt: "c", OrganizationID: "o"}, &m)
	if !m.Color.IsNull() {
		t.Errorf("Color = %v, want null", m.Color)
	}
	if m.ID.ValueString() != "t1" || m.Name.ValueString() != "n" || m.CreatedAt.ValueString() != "c" || m.OrganizationID.ValueString() != "o" {
		t.Errorf("model = %+v", m)
	}
	empty := ""
	flatten(&client.Tag{Color: &empty}, &m)
	if !m.Color.IsNull() {
		t.Errorf("Color = %v, want null for a stored empty string", m.Color)
	}
	teal := "#0a8a74"
	flatten(&client.Tag{Color: &teal}, &m)
	if m.Color.ValueString() != teal {
		t.Errorf("Color = %v, want %q", m.Color, teal)
	}
}

func TestColorRequest(t *testing.T) {
	if colorRequest(types.StringNull()) != nil || colorRequest(types.StringUnknown()) != nil {
		t.Error("a null or unknown colour must map to nil, the JSON null that clears it")
	}
	if got := colorRequest(types.StringValue("#fff")); got == nil || *got != "#fff" {
		t.Errorf("colorRequest(#fff) = %v", got)
	}
}
