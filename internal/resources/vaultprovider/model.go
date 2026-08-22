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

type hashicorpModel struct {
	URL       types.String `tfsdk:"url"`
	Token     types.String `tfsdk:"token"`
	Namespace types.String `tfsdk:"namespace"`
	Mount     types.String `tfsdk:"mount"`
}

type infisicalModel struct {
	SiteURL         types.String `tfsdk:"site_url"`
	ClientID        types.String `tfsdk:"client_id"`
	ClientSecret    types.String `tfsdk:"client_secret"`
	ProjectID       types.String `tfsdk:"project_id"`
	EnvironmentSlug types.String `tfsdk:"environment_slug"`
	SecretPath      types.String `tfsdk:"secret_path"`
}

type awsModel struct {
	Region          types.String `tfsdk:"region"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	Endpoint        types.String `tfsdk:"endpoint"`
}

type dopplerModel struct {
	ServiceToken types.String `tfsdk:"service_token"`
	Project      types.String `tfsdk:"project"`
	Config       types.String `tfsdk:"config"`
}

type azureModel struct {
	VaultURI     types.String `tfsdk:"vault_uri"`
	TenantID     types.String `tfsdk:"tenant_id"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
}

type scalewayModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	SecretKey types.String `tfsdk:"secret_key"`
	Region    types.String `tfsdk:"region"`
	APIURL    types.String `tfsdk:"api_url"`
}

type assignmentModel struct {
	ProjectID      types.String `tfsdk:"project_id"`
	EnvironmentIDs types.Set    `tfsdk:"environment_ids"`
}

func hashicorpAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"url":       types.StringType,
		"token":     types.StringType,
		"namespace": types.StringType,
		"mount":     types.StringType,
	}
}

func infisicalAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"site_url":         types.StringType,
		"client_id":        types.StringType,
		"client_secret":    types.StringType,
		"project_id":       types.StringType,
		"environment_slug": types.StringType,
		"secret_path":      types.StringType,
	}
}

func awsAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"region":            types.StringType,
		"access_key_id":     types.StringType,
		"secret_access_key": types.StringType,
		"endpoint":          types.StringType,
	}
}

func dopplerAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"service_token": types.StringType,
		"project":       types.StringType,
		"config":        types.StringType,
	}
}

func azureAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"vault_uri":     types.StringType,
		"tenant_id":     types.StringType,
		"client_id":     types.StringType,
		"client_secret": types.StringType,
	}
}

func scalewayAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"project_id": types.StringType,
		"secret_key": types.StringType,
		"region":     types.StringType,
		"api_url":    types.StringType,
	}
}

func assignmentAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"project_id":      types.StringType,
		"environment_ids": types.SetType{ElemType: types.StringType},
	}
}

// --- expand: model block -> typed client config struct ---

func expandHashicorpConfig(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.VaultHashicorpConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m hashicorpModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.VaultHashicorpConfig{
		ProviderType: client.VaultProviderTypeHashicorp,
		URL:          m.URL.ValueString(),
		Token:        m.Token.ValueString(),
		Namespace:    m.Namespace.ValueString(),
		Mount:        m.Mount.ValueString(),
	}
}

func expandInfisicalConfig(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.VaultInfisicalConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m infisicalModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.VaultInfisicalConfig{
		ProviderType:    client.VaultProviderTypeInfisical,
		SiteURL:         m.SiteURL.ValueString(),
		ClientID:        m.ClientID.ValueString(),
		ClientSecret:    m.ClientSecret.ValueString(),
		ProjectID:       m.ProjectID.ValueString(),
		EnvironmentSlug: m.EnvironmentSlug.ValueString(),
		SecretPath:      m.SecretPath.ValueString(),
	}
}

func expandAWSConfig(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.VaultAWSConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m awsModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.VaultAWSConfig{
		ProviderType:    client.VaultProviderTypeAWS,
		Region:          m.Region.ValueString(),
		AccessKeyID:     m.AccessKeyID.ValueString(),
		SecretAccessKey: m.SecretAccessKey.ValueString(),
		Endpoint:        m.Endpoint.ValueString(),
	}
}

func expandDopplerConfig(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.VaultDopplerConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m dopplerModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.VaultDopplerConfig{
		ProviderType: client.VaultProviderTypeDoppler,
		ServiceToken: m.ServiceToken.ValueString(),
		Project:      m.Project.ValueString(),
		Config:       m.Config.ValueString(),
	}
}

func expandAzureConfig(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.VaultAzureConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m azureModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.VaultAzureConfig{
		ProviderType: client.VaultProviderTypeAzure,
		VaultURI:     m.VaultURI.ValueString(),
		TenantID:     m.TenantID.ValueString(),
		ClientID:     m.ClientID.ValueString(),
		ClientSecret: m.ClientSecret.ValueString(),
	}
}

func expandScalewayConfig(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.VaultScalewayConfig {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m scalewayModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.VaultScalewayConfig{
		ProviderType: client.VaultProviderTypeScaleway,
		Region:       m.Region.ValueString(),
		ProjectID:    m.ProjectID.ValueString(),
		SecretKey:    m.SecretKey.ValueString(),
		APIURL:       m.APIURL.ValueString(),
	}
}

// expandConfig picks the one populated config block out of m (the
// ConfigValidators ExactlyOneOf on the resource guarantees exactly one is
// set by the time Create/Update run) and returns the built client struct
// together with its wire discriminator.
func expandConfig(ctx context.Context, m resourceModel, diags *diag.Diagnostics) (any, string) {
	switch {
	case !m.Hashicorp.IsNull():
		return expandHashicorpConfig(ctx, m.Hashicorp, diags), client.VaultProviderTypeHashicorp
	case !m.Infisical.IsNull():
		return expandInfisicalConfig(ctx, m.Infisical, diags), client.VaultProviderTypeInfisical
	case !m.AWS.IsNull():
		return expandAWSConfig(ctx, m.AWS, diags), client.VaultProviderTypeAWS
	case !m.Doppler.IsNull():
		return expandDopplerConfig(ctx, m.Doppler, diags), client.VaultProviderTypeDoppler
	case !m.Azure.IsNull():
		return expandAzureConfig(ctx, m.Azure, diags), client.VaultProviderTypeAzure
	case !m.Scaleway.IsNull():
		return expandScalewayConfig(ctx, m.Scaleway, diags), client.VaultProviderTypeScaleway
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

func flattenHashicorpConfig(c *client.VaultHashicorpConfig, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(hashicorpAttrTypes())
	}
	obj, d := types.ObjectValue(hashicorpAttrTypes(), map[string]attr.Value{
		"url":       types.StringValue(c.URL),
		"token":     types.StringValue(c.Token),
		"namespace": tfutil.StringOrNull(&c.Namespace),
		"mount":     types.StringValue(c.Mount),
	})
	diags.Append(d...)
	return obj
}

func flattenInfisicalConfig(c *client.VaultInfisicalConfig, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(infisicalAttrTypes())
	}
	obj, d := types.ObjectValue(infisicalAttrTypes(), map[string]attr.Value{
		"site_url":         types.StringValue(c.SiteURL),
		"client_id":        types.StringValue(c.ClientID),
		"client_secret":    types.StringValue(c.ClientSecret),
		"project_id":       types.StringValue(c.ProjectID),
		"environment_slug": types.StringValue(c.EnvironmentSlug),
		"secret_path":      types.StringValue(c.SecretPath),
	})
	diags.Append(d...)
	return obj
}

func flattenAWSConfig(c *client.VaultAWSConfig, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(awsAttrTypes())
	}
	obj, d := types.ObjectValue(awsAttrTypes(), map[string]attr.Value{
		"region":            types.StringValue(c.Region),
		"access_key_id":     types.StringValue(c.AccessKeyID),
		"secret_access_key": types.StringValue(c.SecretAccessKey),
		"endpoint":          tfutil.StringOrNull(&c.Endpoint),
	})
	diags.Append(d...)
	return obj
}

func flattenDopplerConfig(c *client.VaultDopplerConfig, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(dopplerAttrTypes())
	}
	obj, d := types.ObjectValue(dopplerAttrTypes(), map[string]attr.Value{
		"service_token": types.StringValue(c.ServiceToken),
		"project":       tfutil.StringOrNull(&c.Project),
		"config":        tfutil.StringOrNull(&c.Config),
	})
	diags.Append(d...)
	return obj
}

func flattenAzureConfig(c *client.VaultAzureConfig, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(azureAttrTypes())
	}
	obj, d := types.ObjectValue(azureAttrTypes(), map[string]attr.Value{
		"vault_uri":     types.StringValue(c.VaultURI),
		"tenant_id":     types.StringValue(c.TenantID),
		"client_id":     types.StringValue(c.ClientID),
		"client_secret": types.StringValue(c.ClientSecret),
	})
	diags.Append(d...)
	return obj
}

func flattenScalewayConfig(c *client.VaultScalewayConfig, diags *diag.Diagnostics) types.Object {
	if c == nil {
		return types.ObjectNull(scalewayAttrTypes())
	}
	obj, d := types.ObjectValue(scalewayAttrTypes(), map[string]attr.Value{
		"project_id": types.StringValue(c.ProjectID),
		"secret_key": types.StringValue(c.SecretKey),
		"region":     types.StringValue(c.Region),
		"api_url":    types.StringValue(c.APIURL),
	})
	diags.Append(d...)
	return obj
}

// flattenConfig nulls all six config blocks in m, then sets whichever one
// matches cfg's concrete type - the union round-trip's inverse of
// expandConfig. Create and Update call it on the very struct they just sent
// the server, to normalize state to a byte-perfect reflection of what was
// actually written rather than trust the plan object as-is.
func flattenConfig(cfg any, m *resourceModel, diags *diag.Diagnostics) {
	m.Hashicorp = types.ObjectNull(hashicorpAttrTypes())
	m.Infisical = types.ObjectNull(infisicalAttrTypes())
	m.AWS = types.ObjectNull(awsAttrTypes())
	m.Doppler = types.ObjectNull(dopplerAttrTypes())
	m.Azure = types.ObjectNull(azureAttrTypes())
	m.Scaleway = types.ObjectNull(scalewayAttrTypes())

	switch c := cfg.(type) {
	case *client.VaultHashicorpConfig:
		m.Hashicorp = flattenHashicorpConfig(c, diags)
	case *client.VaultInfisicalConfig:
		m.Infisical = flattenInfisicalConfig(c, diags)
	case *client.VaultAWSConfig:
		m.AWS = flattenAWSConfig(c, diags)
	case *client.VaultDopplerConfig:
		m.Doppler = flattenDopplerConfig(c, diags)
	case *client.VaultAzureConfig:
		m.Azure = flattenAzureConfig(c, diags)
	case *client.VaultScalewayConfig:
		m.Scaleway = flattenScalewayConfig(c, diags)
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
