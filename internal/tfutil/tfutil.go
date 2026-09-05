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

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
			Description: "Deploy after a create, and after a change to an attribute that starts a deploy. Defaults to `true`.",
		},
		"deployment_timeout": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(DefaultDeploymentTimeout),
			Validators:  []validator.String{DurationString()},
			Description: "The maximum wait for a deploy to reach a terminal status, as a Go duration string. Defaults to `\"15m\"`. On timeout, the apply fails, but the deploy continues on the server.",
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
// (JSON null) and [] collapse to a NULL set. The v0.30.0 network endpoints
// store a literal null on an explicit clear. `[]` is only the
// fresh-create shape. A clear never produces it. The attributes are
// Optional with no default, so an empty set in state would diff against
// config's null forever. A schema validator (SizeAtLeast(1)) keeps
// config from expressing [], so the collapse loses nothing.
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

// WriteOnlyCompanions returns the two companions of the Sensitive attribute
// name: `<name>_wo`, the write-only form, and `<name>_wo_version`, the
// version that gates a new write-only value. The framework keeps a
// write-only value out of the plan and the state, so the version is the only
// signal that tells Update that the value changed (the HashiCorp convention,
// for example aws_db_instance.password_wo_version). exactlyOne is true when
// the base attribute is Optional only because the write-only form can
// replace it: the pair then needs exactly one value. Otherwise the two only
// conflict. note is an optional sentence for the effect of a version change,
// for example "A version change starts a redeploy."
//
// A Terraform CLI before 1.11 rejects a non-null write-only value at
// validation with a message that names the version (the framework's own
// check). It accepts the schema, and a config that leaves the companions
// unset behaves as before.
func WriteOnlyCompanions(name string, exactlyOne bool, note string) map[string]schema.Attribute {
	wo := name + "_wo"
	version := wo + "_version"
	pick := stringvalidator.ConflictsWith(path.MatchRoot(name))
	rule := "Do not set it together with `" + name + "`."
	if exactlyOne {
		pick = stringvalidator.ExactlyOneOf(path.MatchRoot(name))
		rule = "Set exactly one of `" + name + "` and `" + wo + "`."
	}
	if note != "" {
		note = " " + note
	}
	return map[string]schema.Attribute{
		wo: schema.StringAttribute{
			Optional:  true,
			WriteOnly: true,
			Sensitive: true,
			Description: "Write-only form of `" + name + "`. Terraform keeps it out of the plan and the state. " +
				"It needs Terraform 1.11 or later. " + rule +
				" A new value reaches the server only when `" + version + "` changes.",
			Validators: []validator.String{pick},
		},
		version: schema.Int64Attribute{
			Optional: true,
			Description: "Version of `" + wo + "`. Change it to send the current `" + wo + "` value to the server." + note +
				" It needs `" + wo + "`.",
			Validators: []validator.Int64{int64validator.AlsoRequires(path.MatchRoot(wo))},
		},
	}
}

// SecretToCreate returns the value that a Create call sends for a secret
// with write-only companions: the plain attribute when the plan has it, else
// the configured write-only value. Only the config carries a write-only
// value; the plan and the state hold null for it. "" means that neither is
// set, which a Computed secret leaves to the server.
func SecretToCreate(plain, wo types.String) string {
	if !plain.IsNull() && !plain.IsUnknown() {
		return plain.ValueString()
	}
	return wo.ValueString()
}

// SecretToUpdate returns the value that an Update call sends for a secret
// with write-only companions, and whether to send it at all. priorPlain is
// the plain attribute's prior state; version and priorVersion are the
// planned and the prior value of the version companion.
//   - the plain attribute is in the plan: send it.
//   - the config has no write-only value: send nothing. A Computed secret
//     then keeps the server's value, a plain one is left alone.
//   - the plain attribute just left the state (the config switched to the
//     companion), or the version changed: send the write-only value.
//   - otherwise: send nothing. The server keeps the value it holds.
func SecretToUpdate(plain, wo, priorPlain types.String, version, priorVersion types.Int64) (string, bool) {
	if !plain.IsNull() && !plain.IsUnknown() {
		return plain.ValueString(), true
	}
	if wo.IsNull() || wo.IsUnknown() {
		return "", false
	}
	if !priorPlain.IsNull() || !version.Equal(priorVersion) {
		return wo.ValueString(), true
	}
	return "", false
}

// ComputedSecretPlan returns the plan modifier of a Computed secret that has
// write-only companions. It replaces UseStateForUnknown, which copies a null
// prior state forward as a KNOWN null; Terraform then refuses an apply that
// fills the value in ("Provider produced inconsistent result after apply").
// The modifier keeps that behavior for a known prior value, and adds two
// cases for the companion:
//   - the config sets `<name>_wo`: the state will hold null, so the plan
//     says null now instead of "known after apply".
//   - the prior state holds null and the config sets neither: the plan stays
//     unknown, and the apply fills in the server's value. This is the
//     switch back from the companion to the server-generated value.
func ComputedSecretPlan(name string) planmodifier.String {
	return computedSecretPlan{wo: path.Root(name + "_wo")}
}

type computedSecretPlan struct{ wo path.Path }

func (computedSecretPlan) Description(ctx context.Context) string {
	return "Keeps the prior state value for an unknown plan value. The plan is null when the write-only companion is set, and stays unknown when the prior value is null."
}

func (m computedSecretPlan) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m computedSecretPlan) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Do nothing on create, for a known planned value, or for an unknown
	// configuration value (the three UseStateForUnknown guards).
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	var wo types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, m.wo, &wo)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !wo.IsNull() {
		resp.PlanValue = types.StringNull()
		return
	}
	if req.StateValue.IsNull() {
		return
	}
	resp.PlanValue = req.StateValue
}

// PrivateState is the part of the framework's resource private state that
// the write-only flag uses. The framework declares the concrete type in an
// internal package, so the Private field of every resource request and
// response satisfies this interface instead.
type PrivateState interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

func writeOnlyKey(name string) string { return "write_only:" + name }

// SetWriteOnlyFlag records in the private state whether the secret name came
// from its write-only companion. Read consults the flag with WriteOnlyFlag
// and then keeps the server's value out of the state. The Dokploy API
// returns every stored secret on a read, so without the flag a refresh would
// put the secret back into the state. A false flag removes the key.
func SetWriteOnlyFlag(ctx context.Context, p PrivateState, name string, on bool) diag.Diagnostics {
	if !on {
		return p.SetKey(ctx, writeOnlyKey(name), nil)
	}
	return p.SetKey(ctx, writeOnlyKey(name), []byte("true"))
}

// WriteOnlyFlag reports the flag that SetWriteOnlyFlag stored. A state from
// a release before the companions has no key and reads false, so the plain
// attribute keeps its refresh behavior.
func WriteOnlyFlag(ctx context.Context, p PrivateState, name string) (bool, diag.Diagnostics) {
	v, diags := p.GetKey(ctx, writeOnlyKey(name))
	return string(v) == "true", diags
}
