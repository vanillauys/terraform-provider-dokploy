package tfutil

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

func TestParseTimeout(t *testing.T) {
	d, err := ParseTimeout(types.StringValue("10m"))
	if err != nil || d != 10*time.Minute {
		t.Errorf("d=%v err=%v", d, err)
	}
	// Null falls back to the spec default of 15m.
	d, err = ParseTimeout(types.StringNull())
	if err != nil || d != 15*time.Minute {
		t.Errorf("null: d=%v err=%v", d, err)
	}
	if _, err = ParseTimeout(types.StringValue("banana")); err == nil {
		t.Error("invalid duration accepted")
	}
}

func TestDeployAttributes(t *testing.T) {
	attrs := DeployAttributes()
	for _, name := range []string{"deploy_on_change", "deployment_timeout"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
}

// ImportDeployDefaults writes DefaultDeployOnChange / DefaultDeploymentTimeout
// into imported state, and the framework re-applies the *schema* defaults on
// every plan. If the two ever disagree, `terraform import` silently goes back
// to producing a non-empty plan — the exact defect the constants were
// introduced to close. This asserts the schema really does default to the
// constants, rather than trusting that both call sites were updated together.
func TestDeployAttributeDefaultsMatchConstants(t *testing.T) {
	ctx := context.Background()
	attrs := DeployAttributes()

	boolAttr, ok := attrs["deploy_on_change"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("deploy_on_change is %T, want schema.BoolAttribute", attrs["deploy_on_change"])
	}
	boolResp := defaults.BoolResponse{}
	boolAttr.Default.DefaultBool(ctx, defaults.BoolRequest{}, &boolResp)
	if boolResp.PlanValue.ValueBool() != DefaultDeployOnChange {
		t.Errorf("deploy_on_change schema default = %v, but DefaultDeployOnChange = %v",
			boolResp.PlanValue, DefaultDeployOnChange)
	}

	strAttr, ok := attrs["deployment_timeout"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("deployment_timeout is %T, want schema.StringAttribute", attrs["deployment_timeout"])
	}
	strResp := defaults.StringResponse{}
	strAttr.Default.DefaultString(ctx, defaults.StringRequest{}, &strResp)
	if strResp.PlanValue.ValueString() != DefaultDeploymentTimeout {
		t.Errorf("deployment_timeout schema default = %v, but DefaultDeploymentTimeout = %q",
			strResp.PlanValue, DefaultDeploymentTimeout)
	}

	// ParseTimeout's null fallback must agree too.
	d, err := ParseTimeout(types.StringNull())
	if err != nil {
		t.Fatalf("ParseTimeout(null): %v", err)
	}
	want, err := time.ParseDuration(DefaultDeploymentTimeout)
	if err != nil {
		t.Fatalf("DefaultDeploymentTimeout %q is not a valid duration: %v", DefaultDeploymentTimeout, err)
	}
	if d != want {
		t.Errorf("ParseTimeout(null) = %v, want %v", d, want)
	}
}

func TestClientFromProviderData(t *testing.T) {
	// The framework calls Configure once with a nil ProviderData, before the
	// provider itself is configured. That must be a silent no-op, not an
	// error: erroring there breaks every resource in the provider.
	c, diags := ClientFromProviderData(nil)
	if c != nil || diags.HasError() {
		t.Errorf("nil provider data: client = %v, diags = %v; want (nil, no error)", c, diags)
	}

	want, err := client.New("https://example.com", "k", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, diags := ClientFromProviderData(want)
	if diags.HasError() {
		t.Errorf("diags = %v", diags)
	}
	if got != want {
		t.Errorf("client = %v, want %v", got, want)
	}

	got, diags = ClientFromProviderData("not a client")
	if got != nil {
		t.Errorf("client = %v, want nil", got)
	}
	if !diags.HasError() {
		t.Error("a wrongly-typed ProviderData must produce an error diagnostic")
	}
}
