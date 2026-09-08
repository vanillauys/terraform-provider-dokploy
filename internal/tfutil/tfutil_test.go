package tfutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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

// TestStringSetOrNullCollapsesEmpty pins the v0.30.0 networkIds rule (see
// internal/client/doc.go, v0.30.0 section): a fresh record reads networkIds
// back as `[]`, and an explicit clear reads it back as a literal `null`. Both
// must collapse to a null set, or Read disagrees with config's null forever.
func TestStringSetOrNullCollapsesEmpty(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	if got := StringSetOrNull(ctx, nil, &diags); !got.IsNull() {
		t.Errorf("nil -> %v, want null set", got)
	}
	if got := StringSetOrNull(ctx, []string{}, &diags); !got.IsNull() {
		t.Errorf("[] -> %v, want null set", got)
	}
	got := StringSetOrNull(ctx, []string{"a"}, &diags)
	if got.IsNull() || len(got.Elements()) != 1 {
		t.Errorf(`["a"] -> %v, want one-element set`, got)
	}
	if diags.HasError() {
		t.Fatal(diags)
	}
}

// TestStringSetRequestNullMeansNil pins the inverse: a null or unknown set
// must marshal as an explicit JSON null, which the v0.30.0 update endpoints
// read as "clear the field" (see internal/client/doc.go).
func TestStringSetRequestNullMeansNil(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	if got := StringSetRequest(ctx, types.SetNull(types.StringType), &diags); got != nil {
		t.Errorf("null set -> %v, want nil", got)
	}
	set, _ := types.SetValueFrom(ctx, types.StringType, []string{"a"})
	got := StringSetRequest(ctx, set, &diags)
	if got == nil || len(*got) != 1 || (*got)[0] != "a" {
		t.Errorf("set -> %v, want &[a]", got)
	}
}

// TestStringOrNull pins the rule that broke wave 3's first production
// round-trip: Dokploy stores "" for an optional string cleared through its
// UI, and null for one never set. Both must present as null, or Read
// disagrees with config forever.
func TestStringOrNull(t *testing.T) {
	empty, value := "", "set"
	for name, tc := range map[string]struct {
		in       *string
		wantNull bool
		want     string
	}{
		"nil pointer":  {nil, true, ""},
		"empty string": {&empty, true, ""},
		"real value":   {&value, false, "set"},
	} {
		t.Run(name, func(t *testing.T) {
			got := StringOrNull(tc.in)
			if got.IsNull() != tc.wantNull {
				t.Errorf("IsNull() = %v, want %v", got.IsNull(), tc.wantNull)
			}
			if !tc.wantNull && got.ValueString() != tc.want {
				t.Errorf("ValueString() = %q, want %q", got.ValueString(), tc.want)
			}
		})
	}
}

func TestServerGeneratedAppName(t *testing.T) {
	v := ServerGeneratedAppName()
	for name, value := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("app_name"), ConfigValue: value}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%s: unexpected error: %v", name, resp.Diagnostics)
		}
	}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("app_name"), ConfigValue: types.StringValue("glitchtip")}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("a configured app_name must be rejected")
	}
	if got := resp.Diagnostics[0].Summary(); got != "Dokploy generates app_name" {
		t.Errorf("summary = %q", got)
	}
}

func TestAppNamePrefix(t *testing.T) {
	for in, want := range map[string]string{
		"glitchtip-app03-dfw-wr2oq7": "glitchtip-app03-dfw",
		"glitchtip-nffunj":           "glitchtip",
		"glitchtip":                  "glitchtip",
		"glitchtip-app03":            "glitchtip-app03",  // a five-character tail is not a suffix
		"glitchtip-ABCDEF":           "glitchtip-ABCDEF", // the server lowercases its suffix
		"compose-app-x7k2m9-abc123":  "compose-app-x7k2m9",
	} {
		if got := AppNamePrefix(in); got != want {
			t.Errorf("AppNamePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppNamePrefixValidators(t *testing.T) {
	for value, ok := range map[string]bool{
		"glitchtip-app03-dfw":   true,
		"a.b_c-d1":              true,
		strings.Repeat("a", 56): true,
		"GlitchTip":             false,
		"bad prefix":            false,
		"":                      false,
		strings.Repeat("a", 57): false,
	} {
		resp := &validator.StringResponse{}
		for _, v := range AppNamePrefixValidators() {
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("app_name_prefix"), ConfigValue: types.StringValue(value)}, resp)
		}
		if resp.Diagnostics.HasError() == ok {
			t.Errorf("%q: valid = %v, want %v: %v", value, !resp.Diagnostics.HasError(), ok, resp.Diagnostics)
		}
	}
}

func TestAppNameSeed(t *testing.T) {
	name := types.StringValue("glitchtip")
	for label, tc := range map[string]struct {
		prefix types.String
		want   string
	}{
		"null":    {types.StringNull(), "glitchtip"},
		"unknown": {types.StringUnknown(), "glitchtip"},
		"set":     {types.StringValue("glitchtip-app03-dfw"), "glitchtip-app03-dfw"},
	} {
		if got := AppNameSeed(tc.prefix, name); got != tc.want {
			t.Errorf("%s: AppNameSeed = %q, want %q", label, got, tc.want)
		}
	}
}
