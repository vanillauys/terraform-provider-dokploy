package appchild

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSecuritySchema_WriteOnlyCompanions pins the D1(a) shape on the
// security password and runs the framework's own schema checks, which only
// an acceptance run reached before.
func TestSecuritySchema_WriteOnlyCompanions(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	NewResource(SecurityKind())().Schema(ctx, resource.SchemaRequest{}, &resp)
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
	if _, ok := resp.Schema.Attributes["password_wo_version"].(schema.Int64Attribute); !ok {
		t.Errorf("password_wo_version is %T", resp.Schema.Attributes["password_wo_version"])
	}
}

// TestSecurityKind_ResolveSecrets pins the create route and the update
// route that needs no server read, without a client: the server read only
// happens when the write-only password has nothing new to send.
func TestSecurityKind_ResolveSecrets(t *testing.T) {
	ctx := context.Background()
	k := SecurityKind()

	plan := SecurityModel{Password: types.StringNull()}
	cfg := SecurityModel{PasswordWo: types.StringValue("wo-1")}
	inUse, err := k.ResolveSecrets(ctx, nil, &plan, &cfg, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if plan.Password.ValueString() != "wo-1" || !inUse["password"] {
		t.Errorf("create: password = %v, inUse = %v; want the write-only value and the flag", plan.Password, inUse)
	}

	plan = SecurityModel{Password: types.StringNull(), PasswordWoVersion: types.Int64Value(2)}
	prior := SecurityModel{Password: types.StringNull(), PasswordWoVersion: types.Int64Value(1)}
	cfg = SecurityModel{PasswordWo: types.StringValue("wo-2")}
	if _, err := k.ResolveSecrets(ctx, nil, &plan, &cfg, &prior); err != nil {
		t.Fatalf("version change: %v", err)
	}
	if plan.Password.ValueString() != "wo-2" {
		t.Errorf("version change: password = %v, want wo-2", plan.Password)
	}

	plan = SecurityModel{Password: types.StringValue("plain")}
	cfg = SecurityModel{PasswordWo: types.StringNull()}
	inUse, err = k.ResolveSecrets(ctx, nil, &plan, &cfg, &prior)
	if err != nil {
		t.Fatalf("plain: %v", err)
	}
	if plan.Password.ValueString() != "plain" || inUse["password"] {
		t.Errorf("plain: password = %v, inUse = %v; want the plain value and no flag", plan.Password, inUse)
	}

	m := SecurityModel{Password: types.StringValue("secret")}
	k.HideSecret(&m, "password")
	if !m.Password.IsNull() {
		t.Error("HideSecret must null the password")
	}
}
