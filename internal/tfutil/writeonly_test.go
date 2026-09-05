package tfutil

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestWriteOnlyCompanions(t *testing.T) {
	for _, exactlyOne := range []bool{true, false} {
		attrs := WriteOnlyCompanions("database_password", exactlyOne, "A version change starts a redeploy.")
		if len(attrs) != 2 {
			t.Fatalf("exactlyOne=%v: got %d attributes, want 2", exactlyOne, len(attrs))
		}
		wo, ok := attrs["database_password_wo"].(schema.StringAttribute)
		if !ok {
			t.Fatalf("exactlyOne=%v: database_password_wo is %T", exactlyOne, attrs["database_password_wo"])
		}
		if !wo.WriteOnly || !wo.Sensitive || !wo.Optional || wo.Required || wo.Computed {
			t.Errorf("exactlyOne=%v: database_password_wo must be Optional+WriteOnly+Sensitive, got %+v", exactlyOne, wo)
		}
		if len(wo.Validators) != 1 {
			t.Errorf("exactlyOne=%v: database_password_wo needs one validator, got %d", exactlyOne, len(wo.Validators))
		}
		version, ok := attrs["database_password_wo_version"].(schema.Int64Attribute)
		if !ok {
			t.Fatalf("exactlyOne=%v: database_password_wo_version is %T", exactlyOne, attrs["database_password_wo_version"])
		}
		if !version.Optional || version.Computed || version.WriteOnly {
			t.Errorf("exactlyOne=%v: database_password_wo_version must be a plain Optional attribute, got %+v", exactlyOne, version)
		}
		if len(version.Validators) != 1 {
			t.Errorf("exactlyOne=%v: database_password_wo_version needs one validator, got %d", exactlyOne, len(version.Validators))
		}
		if version.Description != "Version of `database_password_wo`. Change it to send the current `database_password_wo` value to the server. A version change starts a redeploy. It needs `database_password_wo`." {
			t.Errorf("unexpected version description: %q", version.Description)
		}
	}

	wo := WriteOnlyCompanions("database_password", true, "")["database_password_wo"].(schema.StringAttribute)
	if wo.Description != "Write-only form of `database_password`. Terraform keeps it out of the plan and the state. It needs Terraform 1.11 or later. Set exactly one of `database_password` and `database_password_wo`. A new value reaches the server only when `database_password_wo_version` changes." {
		t.Errorf("unexpected exactly-one description: %q", wo.Description)
	}
	wo = WriteOnlyCompanions("database_root_password", false, "")["database_root_password_wo"].(schema.StringAttribute)
	if wo.Description != "Write-only form of `database_root_password`. Terraform keeps it out of the plan and the state. It needs Terraform 1.11 or later. Do not set it together with `database_root_password`. A new value reaches the server only when `database_root_password_wo_version` changes." {
		t.Errorf("unexpected conflicts description: %q", wo.Description)
	}
}

func TestSecretToCreate(t *testing.T) {
	cases := []struct {
		name      string
		plain, wo types.String
		want      string
	}{
		{"plain value wins", types.StringValue("plain"), types.StringValue("wo"), "plain"},
		{"plain empty string is a value", types.StringValue(""), types.StringValue("wo"), ""},
		{"write-only value when the plain one is null", types.StringNull(), types.StringValue("wo"), "wo"},
		{"write-only value when the plain one is unknown (Computed)", types.StringUnknown(), types.StringValue("wo"), "wo"},
		{"neither reads as empty", types.StringUnknown(), types.StringNull(), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SecretToCreate(tc.plain, tc.wo); got != tc.want {
				t.Errorf("SecretToCreate() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSecretToUpdate(t *testing.T) {
	cases := []struct {
		name                  string
		plain, wo, priorPlain types.String
		version, priorVersion types.Int64
		wantValue             string
		wantSend              bool
	}{
		{"plain value wins", types.StringValue("plain"), types.StringNull(), types.StringValue("old"), types.Int64Null(), types.Int64Null(), "plain", true},
		{"plain empty string is a value", types.StringValue(""), types.StringValue("wo"), types.StringNull(), types.Int64Null(), types.Int64Null(), "", true},
		{"no write-only value sends nothing", types.StringNull(), types.StringNull(), types.StringNull(), types.Int64Null(), types.Int64Value(2), "", false},
		{"companions dropped from a Computed secret send nothing", types.StringUnknown(), types.StringNull(), types.StringNull(), types.Int64Null(), types.Int64Value(2), "", false},
		{"version change sends the write-only value", types.StringNull(), types.StringValue("wo-2"), types.StringNull(), types.Int64Value(2), types.Int64Value(1), "wo-2", true},
		{"version first set sends the write-only value", types.StringNull(), types.StringValue("wo"), types.StringNull(), types.Int64Value(1), types.Int64Null(), "wo", true},
		{"switch from the plain attribute sends the write-only value", types.StringNull(), types.StringValue("wo"), types.StringValue("old-plain"), types.Int64Null(), types.Int64Null(), "wo", true},
		{"same version sends nothing", types.StringNull(), types.StringValue("changed-but-not-versioned"), types.StringNull(), types.Int64Value(1), types.Int64Value(1), "", false},
		{"no version at all sends nothing", types.StringNull(), types.StringValue("wo"), types.StringNull(), types.Int64Null(), types.Int64Null(), "", false},
		{"zero-value versions compare equal", types.StringUnknown(), types.StringValue("wo"), types.String{}, types.Int64{}, types.Int64{}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, send := SecretToUpdate(tc.plain, tc.wo, tc.priorPlain, tc.version, tc.priorVersion)
			if value != tc.wantValue || send != tc.wantSend {
				t.Errorf("SecretToUpdate() = (%q, %v), want (%q, %v)", value, send, tc.wantValue, tc.wantSend)
			}
		})
	}
}

// TestComputedSecretPlan pins the three branches of the plan modifier on a
// two-attribute schema: the companion set plans null, a null prior state
// without the companion stays unknown, and a known prior value carries
// forward as UseStateForUnknown would.
func TestComputedSecretPlan(t *testing.T) {
	ctx := context.Background()
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"secret":    schema.StringAttribute{Optional: true, Computed: true, Sensitive: true},
		"secret_wo": schema.StringAttribute{Optional: true, WriteOnly: true, Sensitive: true},
	}}
	objType := s.Type().TerraformType(ctx).(tftypes.Object)
	str := func(v string) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }
	null := tftypes.NewValue(tftypes.String, nil)
	obj := func(secret, wo tftypes.Value) tftypes.Value {
		return tftypes.NewValue(objType, map[string]tftypes.Value{"secret": secret, "secret_wo": wo})
	}
	noState := tftypes.NewValue(objType, nil)

	cases := []struct {
		name       string
		configWo   tftypes.Value
		stateRaw   tftypes.Value
		stateValue types.String
		planValue  types.String
		want       types.String
	}{
		{"companion set: the plan is null", str("wo"), obj(str("old"), null), types.StringValue("old"), types.StringUnknown(), types.StringNull()},
		{"null prior state and no companion: the plan stays unknown", null, obj(null, null), types.StringNull(), types.StringUnknown(), types.StringUnknown()},
		{"known prior state and no companion: the prior value carries forward", null, obj(str("old"), null), types.StringValue("old"), types.StringUnknown(), types.StringValue("old")},
		{"known plan value is untouched", null, obj(str("old"), null), types.StringValue("old"), types.StringValue("new"), types.StringValue("new")},
		{"create is untouched", str("wo"), noState, types.StringNull(), types.StringUnknown(), types.StringUnknown()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.StringRequest{
				Path:        path.Root("secret"),
				Config:      tfsdk.Config{Schema: s, Raw: obj(null, tc.configWo)},
				State:       tfsdk.State{Schema: s, Raw: tc.stateRaw},
				ConfigValue: types.StringNull(),
				StateValue:  tc.stateValue,
				PlanValue:   tc.planValue,
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.planValue}
			ComputedSecretPlan("secret").PlanModifyString(ctx, req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(tc.want) {
				t.Errorf("PlanValue = %v, want %v", resp.PlanValue, tc.want)
			}
		})
	}
}

type fakePrivate struct{ data map[string][]byte }

func (f *fakePrivate) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return f.data[key], nil
}

func (f *fakePrivate) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	if len(value) == 0 {
		delete(f.data, key)
		return nil
	}
	f.data[key] = value
	return nil
}

func TestWriteOnlyFlag(t *testing.T) {
	ctx := context.Background()
	p := &fakePrivate{data: map[string][]byte{}}

	if on, _ := WriteOnlyFlag(ctx, p, "database_password"); on {
		t.Error("a missing key must read false")
	}
	if diags := SetWriteOnlyFlag(ctx, p, "database_password", true); diags.HasError() {
		t.Fatalf("SetWriteOnlyFlag(true): %v", diags)
	}
	if on, _ := WriteOnlyFlag(ctx, p, "database_password"); !on {
		t.Error("the flag must read true after SetWriteOnlyFlag(true)")
	}
	if on, _ := WriteOnlyFlag(ctx, p, "database_root_password"); on {
		t.Error("the flag of one secret must not leak into another")
	}
	if diags := SetWriteOnlyFlag(ctx, p, "database_password", false); diags.HasError() {
		t.Fatalf("SetWriteOnlyFlag(false): %v", diags)
	}
	if _, ok := p.data["write_only:database_password"]; ok {
		t.Error("SetWriteOnlyFlag(false) must remove the key")
	}
}
