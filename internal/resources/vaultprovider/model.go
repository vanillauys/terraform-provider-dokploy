package vaultprovider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

type resourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Hashicorp        types.Object `tfsdk:"hashicorp"`
	Infisical        types.Object `tfsdk:"infisical"`
	AWS              types.Object `tfsdk:"aws"`
	Doppler          types.Object `tfsdk:"doppler"`
	Azure            types.Object `tfsdk:"azure"`
	Scaleway         types.Object `tfsdk:"scaleway"`
	Assignments      types.List   `tfsdk:"assignments"`
	VerifyConnection types.Bool   `tfsdk:"verify_connection"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

// Each secret field of a config block has its write-only companions
// (tfutil.WriteOnlyCompanions): `<x>_wo`, which only the config carries, and
// `<x>_wo_version`. The plan and the state hold null for `<x>_wo`.

type hashicorpModel struct {
	URL            types.String `tfsdk:"url"`
	Token          types.String `tfsdk:"token"`
	TokenWo        types.String `tfsdk:"token_wo"`
	TokenWoVersion types.Int64  `tfsdk:"token_wo_version"`
	Namespace      types.String `tfsdk:"namespace"`
	Mount          types.String `tfsdk:"mount"`
}

type infisicalModel struct {
	SiteURL               types.String `tfsdk:"site_url"`
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecret          types.String `tfsdk:"client_secret"`
	ClientSecretWo        types.String `tfsdk:"client_secret_wo"`
	ClientSecretWoVersion types.Int64  `tfsdk:"client_secret_wo_version"`
	ProjectID             types.String `tfsdk:"project_id"`
	EnvironmentSlug       types.String `tfsdk:"environment_slug"`
	SecretPath            types.String `tfsdk:"secret_path"`
}

type awsModel struct {
	Region                   types.String `tfsdk:"region"`
	AccessKeyID              types.String `tfsdk:"access_key_id"`
	AccessKeyIDWo            types.String `tfsdk:"access_key_id_wo"`
	AccessKeyIDWoVersion     types.Int64  `tfsdk:"access_key_id_wo_version"`
	SecretAccessKey          types.String `tfsdk:"secret_access_key"`
	SecretAccessKeyWo        types.String `tfsdk:"secret_access_key_wo"`
	SecretAccessKeyWoVersion types.Int64  `tfsdk:"secret_access_key_wo_version"`
	Endpoint                 types.String `tfsdk:"endpoint"`
}

type dopplerModel struct {
	ServiceToken          types.String `tfsdk:"service_token"`
	ServiceTokenWo        types.String `tfsdk:"service_token_wo"`
	ServiceTokenWoVersion types.Int64  `tfsdk:"service_token_wo_version"`
	Project               types.String `tfsdk:"project"`
	Config                types.String `tfsdk:"config"`
}

type azureModel struct {
	VaultURI              types.String `tfsdk:"vault_uri"`
	TenantID              types.String `tfsdk:"tenant_id"`
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecret          types.String `tfsdk:"client_secret"`
	ClientSecretWo        types.String `tfsdk:"client_secret_wo"`
	ClientSecretWoVersion types.Int64  `tfsdk:"client_secret_wo_version"`
}

type scalewayModel struct {
	ProjectID          types.String `tfsdk:"project_id"`
	SecretKey          types.String `tfsdk:"secret_key"`
	SecretKeyWo        types.String `tfsdk:"secret_key_wo"`
	SecretKeyWoVersion types.Int64  `tfsdk:"secret_key_wo_version"`
	Region             types.String `tfsdk:"region"`
	APIURL             types.String `tfsdk:"api_url"`
}

type assignmentModel struct {
	ProjectID      types.String `tfsdk:"project_id"`
	EnvironmentIDs types.Set    `tfsdk:"environment_ids"`
}

func hashicorpAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"url":              types.StringType,
		"token":            types.StringType,
		"token_wo":         types.StringType,
		"token_wo_version": types.Int64Type,
		"namespace":        types.StringType,
		"mount":            types.StringType,
	}
}

func infisicalAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"site_url":                 types.StringType,
		"client_id":                types.StringType,
		"client_secret":            types.StringType,
		"client_secret_wo":         types.StringType,
		"client_secret_wo_version": types.Int64Type,
		"project_id":               types.StringType,
		"environment_slug":         types.StringType,
		"secret_path":              types.StringType,
	}
}

func awsAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"region":                       types.StringType,
		"access_key_id":                types.StringType,
		"access_key_id_wo":             types.StringType,
		"access_key_id_wo_version":     types.Int64Type,
		"secret_access_key":            types.StringType,
		"secret_access_key_wo":         types.StringType,
		"secret_access_key_wo_version": types.Int64Type,
		"endpoint":                     types.StringType,
	}
}

func dopplerAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"service_token":            types.StringType,
		"service_token_wo":         types.StringType,
		"service_token_wo_version": types.Int64Type,
		"project":                  types.StringType,
		"config":                   types.StringType,
	}
}

func azureAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"vault_uri":                types.StringType,
		"tenant_id":                types.StringType,
		"client_id":                types.StringType,
		"client_secret":            types.StringType,
		"client_secret_wo":         types.StringType,
		"client_secret_wo_version": types.Int64Type,
	}
}

func scalewayAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"project_id":            types.StringType,
		"secret_key":            types.StringType,
		"secret_key_wo":         types.StringType,
		"secret_key_wo_version": types.Int64Type,
		"region":                types.StringType,
		"api_url":               types.StringType,
	}
}

func assignmentAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"project_id":      types.StringType,
		"environment_ids": types.SetType{ElemType: types.StringType},
	}
}

// decodeBlock reads a config block into target and reports whether the
// block was present. A null or unknown block leaves target at its zero
// value: every companion then reads as null, the plain-attribute case.
func decodeBlock(ctx context.Context, obj types.Object, target any, diags *diag.Diagnostics) bool {
	if obj.IsNull() || obj.IsUnknown() {
		return false
	}
	diags.Append(obj.As(ctx, target, basetypes.ObjectAsOptions{})...)
	return true
}

// --- expand: model block -> typed client config struct ---
//
// Each expand takes the plan block and the config block: only the config
// carries a write-only secret (the framework nulls it in the plan), so the
// secret comes from tfutil.SecretToCreate over the two. The vault provider
// sends the configured value on create and on every update alike: the server
// masks each secret on read (internal/client/doc.go, gate R), so an update
// can never resend a stored value, and each update carries the full body.

func expandHashicorpConfig(ctx context.Context, obj, cfgObj types.Object, diags *diag.Diagnostics) *client.VaultHashicorpConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m, wo hashicorpModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	decodeBlock(ctx, cfgObj, &wo, diags)
	if diags.HasError() {
		return nil
	}
	return &client.VaultHashicorpConfig{
		ProviderType: client.VaultProviderTypeHashicorp,
		URL:          m.URL.ValueString(),
		Token:        tfutil.SecretToCreate(m.Token, wo.TokenWo),
		Namespace:    m.Namespace.ValueString(),
		Mount:        m.Mount.ValueString(),
	}
}

func expandInfisicalConfig(ctx context.Context, obj, cfgObj types.Object, diags *diag.Diagnostics) *client.VaultInfisicalConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m, wo infisicalModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	decodeBlock(ctx, cfgObj, &wo, diags)
	if diags.HasError() {
		return nil
	}
	return &client.VaultInfisicalConfig{
		ProviderType:    client.VaultProviderTypeInfisical,
		SiteURL:         m.SiteURL.ValueString(),
		ClientID:        m.ClientID.ValueString(),
		ClientSecret:    tfutil.SecretToCreate(m.ClientSecret, wo.ClientSecretWo),
		ProjectID:       m.ProjectID.ValueString(),
		EnvironmentSlug: m.EnvironmentSlug.ValueString(),
		SecretPath:      m.SecretPath.ValueString(),
	}
}

func expandAWSConfig(ctx context.Context, obj, cfgObj types.Object, diags *diag.Diagnostics) *client.VaultAWSConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m, wo awsModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	decodeBlock(ctx, cfgObj, &wo, diags)
	if diags.HasError() {
		return nil
	}
	return &client.VaultAWSConfig{
		ProviderType:    client.VaultProviderTypeAWS,
		Region:          m.Region.ValueString(),
		AccessKeyID:     tfutil.SecretToCreate(m.AccessKeyID, wo.AccessKeyIDWo),
		SecretAccessKey: tfutil.SecretToCreate(m.SecretAccessKey, wo.SecretAccessKeyWo),
		Endpoint:        m.Endpoint.ValueString(),
	}
}

func expandDopplerConfig(ctx context.Context, obj, cfgObj types.Object, diags *diag.Diagnostics) *client.VaultDopplerConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m, wo dopplerModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	decodeBlock(ctx, cfgObj, &wo, diags)
	if diags.HasError() {
		return nil
	}
	return &client.VaultDopplerConfig{
		ProviderType: client.VaultProviderTypeDoppler,
		ServiceToken: tfutil.SecretToCreate(m.ServiceToken, wo.ServiceTokenWo),
		Project:      m.Project.ValueString(),
		Config:       m.Config.ValueString(),
	}
}

func expandAzureConfig(ctx context.Context, obj, cfgObj types.Object, diags *diag.Diagnostics) *client.VaultAzureConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m, wo azureModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	decodeBlock(ctx, cfgObj, &wo, diags)
	if diags.HasError() {
		return nil
	}
	return &client.VaultAzureConfig{
		ProviderType: client.VaultProviderTypeAzure,
		VaultURI:     m.VaultURI.ValueString(),
		TenantID:     m.TenantID.ValueString(),
		ClientID:     m.ClientID.ValueString(),
		ClientSecret: tfutil.SecretToCreate(m.ClientSecret, wo.ClientSecretWo),
	}
}

func expandScalewayConfig(ctx context.Context, obj, cfgObj types.Object, diags *diag.Diagnostics) *client.VaultScalewayConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m, wo scalewayModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	decodeBlock(ctx, cfgObj, &wo, diags)
	if diags.HasError() {
		return nil
	}
	return &client.VaultScalewayConfig{
		ProviderType: client.VaultProviderTypeScaleway,
		Region:       m.Region.ValueString(),
		ProjectID:    m.ProjectID.ValueString(),
		SecretKey:    tfutil.SecretToCreate(m.SecretKey, wo.SecretKeyWo),
		APIURL:       m.APIURL.ValueString(),
	}
}

// expandConfig picks the one populated config block out of m (the
// ConfigValidators ExactlyOneOf on the resource guarantees exactly one is
// set by the time Create/Update run) and returns the built client struct
// together with its wire discriminator. cfg is the config model, the only
// carrier of the write-only secrets; the round-trip unit tests pass a zero
// resourceModel for it.
func expandConfig(ctx context.Context, m, cfg resourceModel, diags *diag.Diagnostics) (any, string) {
	switch {
	case !m.Hashicorp.IsNull():
		return expandHashicorpConfig(ctx, m.Hashicorp, cfg.Hashicorp, diags), client.VaultProviderTypeHashicorp
	case !m.Infisical.IsNull():
		return expandInfisicalConfig(ctx, m.Infisical, cfg.Infisical, diags), client.VaultProviderTypeInfisical
	case !m.AWS.IsNull():
		return expandAWSConfig(ctx, m.AWS, cfg.AWS, diags), client.VaultProviderTypeAWS
	case !m.Doppler.IsNull():
		return expandDopplerConfig(ctx, m.Doppler, cfg.Doppler, diags), client.VaultProviderTypeDoppler
	case !m.Azure.IsNull():
		return expandAzureConfig(ctx, m.Azure, cfg.Azure, diags), client.VaultProviderTypeAzure
	case !m.Scaleway.IsNull():
		return expandScalewayConfig(ctx, m.Scaleway, cfg.Scaleway, diags), client.VaultProviderTypeScaleway
	default:
		diags.AddError(
			"Building vault provider config",
			"exactly one of hashicorp, infisical, aws, doppler, azure, or scaleway must be set; "+
				"the resource's ConfigValidators should have caught this before Create or Update ran",
		)
		return nil, ""
	}
}

// --- flatten: typed client config struct -> model block ---
//
// Fields backed by a schema Default (hashicorp.mount, infisical.site_url,
// infisical.secret_path, scaleway.region, scaleway.api_url) always carry a
// concrete value by the time a flatten function sees them, so they map with
// a plain types.StringValue. Fields with no schema default and no server
// default (hashicorp.namespace, doppler.project, doppler.config, aws.
// endpoint) use tfutil.StringOrNull, collapsing the expand path's "" back to
// null so a config that omits the field round-trips to the same null it
// started from - the "documented omitempty exception" internal/client/
// vaultprovider.go's struct comments describe.
//
// Each flatten also takes the plan block, for the write-only companions: a
// secret whose plain attribute is null in the plan is in write-only mode,
// so the state holds null for it and the plan's version companion. Without
// a plan block (the round-trip unit tests) the wire value is the state.

// secretState returns the state value of a secret after a write: null when
// the plan block has no plain value (the companion is in use), else the
// wire value.
func secretState(hasPlan bool, plan types.String, wire string) types.String {
	if hasPlan && plan.IsNull() {
		return types.StringNull()
	}
	return types.StringValue(wire)
}

func flattenHashicorpConfig(ctx context.Context, c *client.VaultHashicorpConfig, planObj types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(hashicorpAttrTypes())
	}
	var p hashicorpModel
	hasPlan := decodeBlock(ctx, planObj, &p, diags)
	obj, d := types.ObjectValue(hashicorpAttrTypes(), map[string]attr.Value{
		"url":              types.StringValue(c.URL),
		"token":            secretState(hasPlan, p.Token, c.Token),
		"token_wo":         types.StringNull(),
		"token_wo_version": p.TokenWoVersion,
		"namespace":        tfutil.StringOrNull(&c.Namespace),
		"mount":            types.StringValue(c.Mount),
	})
	diags.Append(d...)
	return obj
}

func flattenInfisicalConfig(ctx context.Context, c *client.VaultInfisicalConfig, planObj types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(infisicalAttrTypes())
	}
	var p infisicalModel
	hasPlan := decodeBlock(ctx, planObj, &p, diags)
	obj, d := types.ObjectValue(infisicalAttrTypes(), map[string]attr.Value{
		"site_url":                 types.StringValue(c.SiteURL),
		"client_id":                types.StringValue(c.ClientID),
		"client_secret":            secretState(hasPlan, p.ClientSecret, c.ClientSecret),
		"client_secret_wo":         types.StringNull(),
		"client_secret_wo_version": p.ClientSecretWoVersion,
		"project_id":               types.StringValue(c.ProjectID),
		"environment_slug":         types.StringValue(c.EnvironmentSlug),
		"secret_path":              types.StringValue(c.SecretPath),
	})
	diags.Append(d...)
	return obj
}

func flattenAWSConfig(ctx context.Context, c *client.VaultAWSConfig, planObj types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(awsAttrTypes())
	}
	var p awsModel
	hasPlan := decodeBlock(ctx, planObj, &p, diags)
	obj, d := types.ObjectValue(awsAttrTypes(), map[string]attr.Value{
		"region":                       types.StringValue(c.Region),
		"access_key_id":                secretState(hasPlan, p.AccessKeyID, c.AccessKeyID),
		"access_key_id_wo":             types.StringNull(),
		"access_key_id_wo_version":     p.AccessKeyIDWoVersion,
		"secret_access_key":            secretState(hasPlan, p.SecretAccessKey, c.SecretAccessKey),
		"secret_access_key_wo":         types.StringNull(),
		"secret_access_key_wo_version": p.SecretAccessKeyWoVersion,
		"endpoint":                     tfutil.StringOrNull(&c.Endpoint),
	})
	diags.Append(d...)
	return obj
}

func flattenDopplerConfig(ctx context.Context, c *client.VaultDopplerConfig, planObj types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(dopplerAttrTypes())
	}
	var p dopplerModel
	hasPlan := decodeBlock(ctx, planObj, &p, diags)
	obj, d := types.ObjectValue(dopplerAttrTypes(), map[string]attr.Value{
		"service_token":            secretState(hasPlan, p.ServiceToken, c.ServiceToken),
		"service_token_wo":         types.StringNull(),
		"service_token_wo_version": p.ServiceTokenWoVersion,
		"project":                  tfutil.StringOrNull(&c.Project),
		"config":                   tfutil.StringOrNull(&c.Config),
	})
	diags.Append(d...)
	return obj
}

func flattenAzureConfig(ctx context.Context, c *client.VaultAzureConfig, planObj types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(azureAttrTypes())
	}
	var p azureModel
	hasPlan := decodeBlock(ctx, planObj, &p, diags)
	obj, d := types.ObjectValue(azureAttrTypes(), map[string]attr.Value{
		"vault_uri":                types.StringValue(c.VaultURI),
		"tenant_id":                types.StringValue(c.TenantID),
		"client_id":                types.StringValue(c.ClientID),
		"client_secret":            secretState(hasPlan, p.ClientSecret, c.ClientSecret),
		"client_secret_wo":         types.StringNull(),
		"client_secret_wo_version": p.ClientSecretWoVersion,
	})
	diags.Append(d...)
	return obj
}

func flattenScalewayConfig(ctx context.Context, c *client.VaultScalewayConfig, planObj types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(scalewayAttrTypes())
	}
	var p scalewayModel
	hasPlan := decodeBlock(ctx, planObj, &p, diags)
	obj, d := types.ObjectValue(scalewayAttrTypes(), map[string]attr.Value{
		"project_id":            types.StringValue(c.ProjectID),
		"secret_key":            secretState(hasPlan, p.SecretKey, c.SecretKey),
		"secret_key_wo":         types.StringNull(),
		"secret_key_wo_version": p.SecretKeyWoVersion,
		"region":                types.StringValue(c.Region),
		"api_url":               types.StringValue(c.APIURL),
	})
	diags.Append(d...)
	return obj
}

// flattenConfig nulls all six config blocks in m, then sets whichever one
// matches cfg's concrete type - the union round-trip's inverse of
// expandConfig. Create and Update call it on the very struct they just sent
// the server, to normalize state to a byte-perfect reflection of what was
// actually written rather than trust the plan object as-is. The plan blocks
// that m carries on entry supply the write-only companions.
func flattenConfig(ctx context.Context, cfg any, m *resourceModel, diags *diag.Diagnostics) {
	plan := *m
	m.Hashicorp = types.ObjectNull(hashicorpAttrTypes())
	m.Infisical = types.ObjectNull(infisicalAttrTypes())
	m.AWS = types.ObjectNull(awsAttrTypes())
	m.Doppler = types.ObjectNull(dopplerAttrTypes())
	m.Azure = types.ObjectNull(azureAttrTypes())
	m.Scaleway = types.ObjectNull(scalewayAttrTypes())

	switch c := cfg.(type) {
	case *client.VaultHashicorpConfig:
		m.Hashicorp = flattenHashicorpConfig(ctx, c, plan.Hashicorp, diags)
	case *client.VaultInfisicalConfig:
		m.Infisical = flattenInfisicalConfig(ctx, c, plan.Infisical, diags)
	case *client.VaultAWSConfig:
		m.AWS = flattenAWSConfig(ctx, c, plan.AWS, diags)
	case *client.VaultDopplerConfig:
		m.Doppler = flattenDopplerConfig(ctx, c, plan.Doppler, diags)
	case *client.VaultAzureConfig:
		m.Azure = flattenAzureConfig(ctx, c, plan.Azure, diags)
	case *client.VaultScalewayConfig:
		m.Scaleway = flattenScalewayConfig(ctx, c, plan.Scaleway, diags)
	}
}

// secretsOf returns the configured secret values carried by cfg, for
// redactSecrets to scrub out of a server error message. Order matches each
// struct's field order; nothing distinguishes them further since
// redactSecrets treats every entry the same way.
func secretsOf(cfg any) []string {
	switch c := cfg.(type) {
	case *client.VaultHashicorpConfig:
		return []string{c.Token}
	case *client.VaultInfisicalConfig:
		return []string{c.ClientSecret}
	case *client.VaultAWSConfig:
		return []string{c.AccessKeyID, c.SecretAccessKey}
	case *client.VaultDopplerConfig:
		return []string{c.ServiceToken}
	case *client.VaultAzureConfig:
		return []string{c.ClientSecret}
	case *client.VaultScalewayConfig:
		return []string{c.SecretKey}
	default:
		return nil
	}
}

// redactSecrets replaces every occurrence of each non-empty secret in msg
// with "(redacted)". Mandatory on every server error text reaching AddError
// in Create, Update, and the verify_connection path: vaultProvider.create's
// duplicate-name failure is a raw HTTP 500 whose body leaks config secrets
// in cleartext, observed on doppler and hashicorp (internal/client/doc.go,
// wave 6c "Duplicate names") - this is the last line of defense for that
// server-side defect, and for any other endpoint that might echo a secret
// back in an error body.
func redactSecrets(msg string, secrets []string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, s, "(redacted)")
	}
	return msg
}

// --- assignments ---

func expandAssignments(ctx context.Context, list types.List, diags *diag.Diagnostics) []client.VaultAssignment {
	if list.IsNull() || list.IsUnknown() {
		return []client.VaultAssignment{}
	}
	var models []assignmentModel
	diags.Append(list.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return []client.VaultAssignment{}
	}
	out := make([]client.VaultAssignment, 0, len(models))
	for _, m := range models {
		envIDs := []string{}
		if !m.EnvironmentIDs.IsNull() && !m.EnvironmentIDs.IsUnknown() {
			var ids []string
			diags.Append(m.EnvironmentIDs.ElementsAs(ctx, &ids, false)...)
			if ids != nil {
				envIDs = ids
			}
		}
		out = append(out, client.VaultAssignment{
			ProjectID:      m.ProjectID.ValueString(),
			EnvironmentIDs: envIDs,
		})
	}
	return out
}

// flattenAssignments always produces a non-null environment_ids set, even
// for a zero-length slice - client.VaultAssignment.EnvironmentIDs carries no
// omitempty (doc comment: "[] is meaningful"), and the schema's
// Optional+Computed environment_ids has an empty-set Default, so a null set
// here would diff against that default forever.
func flattenAssignments(ctx context.Context, assignments []client.VaultAssignment, diags *diag.Diagnostics) types.List {
	values := make([]attr.Value, 0, len(assignments))
	for _, a := range assignments {
		ids := a.EnvironmentIDs
		if ids == nil {
			ids = []string{}
		}
		envSet, d := types.SetValueFrom(ctx, types.StringType, ids)
		diags.Append(d...)
		obj, d2 := types.ObjectValue(assignmentAttrTypes(), map[string]attr.Value{
			"project_id":      types.StringValue(a.ProjectID),
			"environment_ids": envSet,
		})
		diags.Append(d2...)
		values = append(values, obj)
	}
	list, d := types.ListValue(types.ObjectType{AttrTypes: assignmentAttrTypes()}, values)
	diags.Append(d...)
	return list
}
