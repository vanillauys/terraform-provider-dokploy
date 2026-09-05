package user

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestSchema_PasswordIsReplaceOnly(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&userResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema(): %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("ValidateImplementation(): %v", diags)
	}
	plain := resp.Schema.Attributes["password"].(schema.StringAttribute)
	if !plain.Optional || !plain.Sensitive || len(plain.PlanModifiers) == 0 {
		t.Errorf("password must be Optional+Sensitive+RequiresReplace, got %+v", plain)
	}
	version := resp.Schema.Attributes["password_wo_version"].(schema.Int64Attribute)
	if len(version.PlanModifiers) == 0 {
		t.Errorf("password_wo_version must carry RequiresReplace")
	}
}

func TestFlattenKeepsThePassword(t *testing.T) {
	m := resourceModel{Password: types.StringValue("kept")}
	flatten(&client.Member{ID: "m1", UserID: "u1", Role: "admin", User: client.User{Email: "a@x"}}, &m)
	if m.Password.ValueString() != "kept" || m.ID.ValueString() != "u1" || m.MemberID.ValueString() != "m1" || m.Role.ValueString() != "admin" {
		t.Errorf("flatten() = %+v", m)
	}
}
