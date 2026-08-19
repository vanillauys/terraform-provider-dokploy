// Package tfutil holds small schema helpers shared by service resources.
//
// TRAP: adding an Optional+Computed *nested* attribute (SingleNestedAttribute
// / ListNestedAttribute) to a resource will, on its own, produce a resource
// that never converges — every plan shows "(known after apply)" on unrelated
// computed attributes forever. The cause is not in this package: when config
// omits a nested-type attribute, Terraform core's objchange proposes null for
// it rather than carrying the prior object forward, so the proposed state
// differs from prior state, which opens the framework's gate on
// MarkComputedNilsAsUnknown (terraform-plugin-framework
// internal/fwserver/server_planresourcechange.go) — and that pass then marks
// EVERY Computed attribute whose *config* value is null as unknown, sweeping
// up id/created_at/app_name/status as collateral. The worked-through
// diagnosis and the working countermeasure live on the `build` attribute in
// internal/resources/application/resource.go (~lines 115-129); read that
// before adding such an attribute. Deliberately NOT generalised into a helper
// here — see the wave-1 notes.
package tfutil

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

// Schema defaults for the deploy-engine attributes (spec §5.5). These are
// referenced by DeployAttributes, ParseTimeout and ImportDeployDefaults so
// the three can never drift apart.
const (
	DefaultDeployOnChange    = true
	DefaultDeploymentTimeout = "15m"
)

// DeployAttributes returns the deploy-engine attributes every service
// resource carries (spec §5.5): deploy_on_change default true,
// deployment_timeout default "15m".
func DeployAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"deploy_on_change": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(DefaultDeployOnChange),
			Description: "Deploy after create and after changes to deploy-triggering attributes. Defaults to `true`.",
		},
		"deployment_timeout": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(DefaultDeploymentTimeout),
			Validators:  []validator.String{DurationString()},
			Description: "How long to wait for a triggered deployment to reach a terminal status, as a Go duration string. Defaults to `\"15m\"`. On timeout the apply fails but the server-side deployment keeps running.",
		},
	}
}

// ImportDeployDefaults seeds the deploy-engine attributes with their schema
// defaults in freshly imported state.
//
// Without this, `terraform import` can never produce a clean follow-up plan.
// The framework applies an attribute's Default on EVERY non-destroy plan
// whose *config* value is null, not only on create
// (terraform-plugin-framework internal/fwserver/server_planresourcechange.go,
// the AttributeDefault pass). deploy_on_change and deployment_timeout are
// provider-only — nothing server-side to import them from — so passthrough
// import leaves them null in state. A config that omits them (the normal
// case, since they have defaults) then plans `true` / `"15m"` against a null
// prior value, and the import is followed by a permanent non-empty plan.
// Writing the defaults at import time makes prior state equal the planned
// value, so the plan is empty.
func ImportDeployDefaults(ctx context.Context, state *tfsdk.State) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.Append(state.SetAttribute(ctx, path.Root("deploy_on_change"), DefaultDeployOnChange)...)
	diags.Append(state.SetAttribute(ctx, path.Root("deployment_timeout"), DefaultDeploymentTimeout)...)
	return diags
}

// ClientFromProviderData converts a provider's ProviderData into a
// *client.Client. It returns a nil client with no diagnostics when
// providerData is nil, which the framework does on the first Configure pass
// before the provider itself has been configured — callers must treat that as
// "nothing to do yet", not an error.
//
// Extracted because all six resources and data sources had a byte-identical
// copy of this. Lives in tfutil rather than a resource package so it can be
// shared without an import cycle (tfutil imports client; the resource and
// data-source packages import tfutil, never the reverse).
// StringOrNull maps a server-side optional string onto a Terraform value,
// treating BOTH JSON null and the empty string as "unset".
//
// Dokploy represents an unset optional string inconsistently even within one
// record: a field that was never set reads back as JSON null, while a field
// that was set and then cleared reads back as a literal "". Terraform config
// that omits the attribute holds null in either case, so a Read that
// reported "" produces a permanent `"" -> null` diff and the resource never
// converges.
//
// This is not hypothetical. internal/resources/environment established the
// rule for `env` in wave 2, but the siblings kept using
// types.StringPointerValue, which preserves "". The gap was invisible on the
// acceptance rig -- records created through the API get null, never "" --
// and only surfaced when wave 3 ran the round-trip against a production
// instance whose project and applications had been created through the
// Dokploy UI, which stores "". It produced a four-resource diff that could
// not be applied away.
//
// Use this for every OPTIONAL string on a read path. Do not use it for a
// field where "" is a meaningful value distinct from unset; none of the
// current resources has one.
func StringOrNull(s *string) types.String {
	if s == nil || *s == "" {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

// StringSetOrNull maps a server string array onto a set attribute. Both nil
// (JSON null) and [] collapse to a NULL set: the v0.30.0 network endpoints
// normalize a cleared list to [], and the attributes are Optional with no
// default, so an empty set in state would diff against config's null forever.
// A schema validator (SizeAtLeast(1)) keeps config from expressing [], so
// the collapse loses nothing.
func StringSetOrNull(ctx context.Context, items []string, diags *diag.Diagnostics) types.Set {
	if len(items) == 0 {
		return types.SetNull(types.StringType)
	}
	set, d := types.SetValueFrom(ctx, types.StringType, items)
	diags.Append(d...)
	return set
}

// StringSetRequest is the inverse: a null or unknown set means "unset",
// which the dialect B update endpoints expect as an explicit JSON null.
func StringSetRequest(ctx context.Context, set types.Set, diags *diag.Diagnostics) *[]string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	var items []string
	diags.Append(set.ElementsAs(ctx, &items, false)...)
	return &items
}

func ClientFromProviderData(providerData any) (*client.Client, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", providerData))
		return nil, diags
	}
	return c, diags
}

// ParseTimeout parses deployment_timeout, defaulting to
// DefaultDeploymentTimeout on null/unknown.
func ParseTimeout(v types.String) (time.Duration, error) {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return time.ParseDuration(DefaultDeploymentTimeout)
	}
	return time.ParseDuration(v.ValueString())
}

type durationString struct{}

func DurationString() validator.String { return durationString{} }

func (durationString) Description(context.Context) string {
	return "a Go duration string such as \"15m\" or \"1h30m\""
}

func (d durationString) MarkdownDescription(ctx context.Context) string {
	return d.Description(ctx)
}

func (durationString) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.ParseDuration(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid duration", err.Error())
	}
}
