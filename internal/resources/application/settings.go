package application

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

// The preview_deployments and rollback blocks (v1.4.0, #52) are plain
// Optional nested attributes, like the source blocks: an Optional+Computed
// nested attribute would trip the MarkComputedNilsAsUnknown trap described
// on `build` in resource.go. A null block therefore means "the server
// defaults", and the resource writes those defaults on every apply that
// omits the block, because application.update is dialect B and would keep
// a value set in the Dokploy UI otherwise.
//
// Read has to decide whether a record maps to a null block or to a block
// that holds the defaults. It keeps whatever shape the prior state has: a
// null prior block stays null while the server holds the defaults, and a
// non-null prior block (config wrote `preview_deployments = {}`) stays a
// block. A record that drifted from the defaults outside Terraform always
// reads back as a block, so the next plan shows the drift.

// Server defaults of a fresh record (internal/client/application_settings.go).
const (
	previewDefaultCertificateType = "none"
	previewDefaultLimit           = int64(3)
	previewDefaultPath            = "/"
	previewDefaultPort            = int64(3000)
)

// previewSecretName is the private-state key of the build_secrets
// write-only flag (tfutil.WriteOnlyFlags).
const previewSecretName = "preview_build_secrets"

type previewModel struct {
	Enabled                        types.Bool   `tfsdk:"enabled"`
	Env                            types.String `tfsdk:"env"`
	BuildArgs                      types.String `tfsdk:"build_args"`
	BuildSecrets                   types.String `tfsdk:"build_secrets"`
	BuildSecretsWo                 types.String `tfsdk:"build_secrets_wo"`
	BuildSecretsWoVersion          types.Int64  `tfsdk:"build_secrets_wo_version"`
	CertificateType                types.String `tfsdk:"certificate_type"`
	CustomCertResolver             types.String `tfsdk:"custom_cert_resolver"`
	HTTPS                          types.Bool   `tfsdk:"https"`
	Labels                         types.List   `tfsdk:"labels"`
	Limit                          types.Int64  `tfsdk:"limit"`
	Path                           types.String `tfsdk:"path"`
	Port                           types.Int64  `tfsdk:"port"`
	RequireCollaboratorPermissions types.Bool   `tfsdk:"require_collaborator_permissions"`
	Wildcard                       types.String `tfsdk:"wildcard"`
}

var previewAttrTypes = map[string]attr.Type{
	"enabled": types.BoolType, "env": types.StringType, "build_args": types.StringType,
	"build_secrets": types.StringType, "build_secrets_wo": types.StringType, "build_secrets_wo_version": types.Int64Type,
	"certificate_type": types.StringType, "custom_cert_resolver": types.StringType, "https": types.BoolType,
	"labels": types.ListType{ElemType: types.StringType}, "limit": types.Int64Type, "path": types.StringType,
	"port": types.Int64Type, "require_collaborator_permissions": types.BoolType, "wildcard": types.StringType,
}

type rollbackModel struct {
	Enabled    types.Bool   `tfsdk:"enabled"`
	RegistryID types.String `tfsdk:"registry_id"`
}

var rollbackAttrTypes = map[string]attr.Type{
	"enabled": types.BoolType, "registry_id": types.StringType,
}

// decodeObject reads a nested block into target and reports whether the
// block was present. A null or unknown block leaves target at its zero
// value, so every attribute reads as null.
func decodeObject(ctx context.Context, obj types.Object, target any, diags *diag.Diagnostics) bool {
	if obj.IsNull() || obj.IsUnknown() {
		return false
	}
	diags.Append(obj.As(ctx, target, objectAsOptions)...)
	return true
}

// previewRequest builds the preview block of the update body. cfg is the
// config's block, which alone carries build_secrets_wo (the framework nulls
// a write-only value in the plan); the value goes out on every update, so a
// write-only secret is never lost to the dialect B keep. An absent block
// writes the server defaults.
func previewRequest(ctx context.Context, plan, cfg types.Object, diags *diag.Diagnostics) client.ApplicationPreviewUpdate {
	var p, wo previewModel
	if !decodeObject(ctx, plan, &p, diags) {
		return client.ApplicationPreviewUpdate{
			PreviewCertificateType:                previewDefaultCertificateType,
			PreviewLimit:                          previewDefaultLimit,
			PreviewPath:                           previewDefaultPath,
			PreviewPort:                           previewDefaultPort,
			PreviewRequireCollaboratorPermissions: true,
		}
	}
	decodeObject(ctx, cfg, &wo, diags)
	var secret *string
	if s := tfutil.SecretToCreate(p.BuildSecrets, wo.BuildSecretsWo); s != "" {
		secret = &s
	}
	return client.ApplicationPreviewUpdate{
		IsPreviewDeploymentsActive:            p.Enabled.ValueBool(),
		PreviewEnv:                            p.Env.ValueStringPointer(),
		PreviewBuildArgs:                      p.BuildArgs.ValueStringPointer(),
		PreviewBuildSecrets:                   secret,
		PreviewCertificateType:                p.CertificateType.ValueString(),
		PreviewCustomCertResolver:             p.CustomCertResolver.ValueStringPointer(),
		PreviewHTTPS:                          p.HTTPS.ValueBool(),
		PreviewLabels:                         stringListRequest(ctx, p.Labels, diags),
		PreviewLimit:                          p.Limit.ValueInt64(),
		PreviewPath:                           p.Path.ValueString(),
		PreviewPort:                           p.Port.ValueInt64(),
		PreviewRequireCollaboratorPermissions: p.RequireCollaboratorPermissions.ValueBool(),
		PreviewWildcard:                       p.Wildcard.ValueStringPointer(),
	}
}

func rollbackRequest(ctx context.Context, plan types.Object, diags *diag.Diagnostics) client.ApplicationRollback {
	var r rollbackModel
	if !decodeObject(ctx, plan, &r, diags) {
		return client.ApplicationRollback{}
	}
	return client.ApplicationRollback{
		RollbackActive:     r.Enabled.ValueBool(),
		RollbackRegistryID: r.RegistryID.ValueStringPointer(),
	}
}

func buildSettingsRequest(m resourceModel) client.ApplicationBuildSettings {
	return client.ApplicationBuildSettings{
		BuildServerID:   m.BuildServerID.ValueStringPointer(),
		BuildRegistryID: m.BuildRegistryID.ValueStringPointer(),
		CleanCache:      m.CleanCache.ValueBool(),
		DropBuildPath:   m.DropBuildPath.ValueStringPointer(),
	}
}

// previewIsDefault reports whether the record holds the server defaults of
// a fresh application, the state a null block writes.
func previewIsDefault(p client.ApplicationPreview) bool {
	return !p.IsPreviewDeploymentsActive &&
		strOrNull(p.PreviewEnv).IsNull() &&
		strOrNull(p.PreviewBuildArgs).IsNull() &&
		strOrNull(p.PreviewBuildSecrets).IsNull() &&
		p.PreviewCertificateType == previewDefaultCertificateType &&
		strOrNull(p.PreviewCustomCertResolver).IsNull() &&
		!p.PreviewHTTPS &&
		len(p.PreviewLabels) == 0 &&
		p.PreviewLimit == previewDefaultLimit &&
		p.PreviewPath == previewDefaultPath &&
		p.PreviewPort == previewDefaultPort &&
		p.PreviewRequireCollaboratorPermissions &&
		strOrNull(p.PreviewWildcard).IsNull()
}

// flattenPreview maps the server's preview block onto the attribute. prior
// is the block in the state before the read; it decides the null-or-block
// shape (see the file comment) and carries build_secrets_wo_version, which
// the server does not hold. build_secrets is the server's value here;
// hideWriteOnly nulls it afterwards when the write-only form is in use.
func flattenPreview(ctx context.Context, app *client.Application, prior types.Object, diags *diag.Diagnostics) types.Object {
	if prior.IsNull() && previewIsDefault(app.ApplicationPreview) {
		return types.ObjectNull(previewAttrTypes)
	}
	var p previewModel
	decodeObject(ctx, prior, &p, diags)
	obj, d := types.ObjectValueFrom(ctx, previewAttrTypes, previewModel{
		Enabled:                        types.BoolValue(app.IsPreviewDeploymentsActive),
		Env:                            strOrNull(app.PreviewEnv),
		BuildArgs:                      strOrNull(app.PreviewBuildArgs),
		BuildSecrets:                   strOrNull(app.PreviewBuildSecrets),
		BuildSecretsWo:                 types.StringNull(),
		BuildSecretsWoVersion:          p.BuildSecretsWoVersion,
		CertificateType:                types.StringValue(app.PreviewCertificateType),
		CustomCertResolver:             strOrNull(app.PreviewCustomCertResolver),
		HTTPS:                          types.BoolValue(app.PreviewHTTPS),
		Labels:                         tfutil.StringListOrNull(ctx, app.PreviewLabels, diags),
		Limit:                          types.Int64Value(app.PreviewLimit),
		Path:                           types.StringValue(app.PreviewPath),
		Port:                           types.Int64Value(app.PreviewPort),
		RequireCollaboratorPermissions: types.BoolValue(app.PreviewRequireCollaboratorPermissions),
		Wildcard:                       strOrNull(app.PreviewWildcard),
	})
	diags.Append(d...)
	return obj
}

func flattenRollback(ctx context.Context, app *client.Application, prior types.Object, diags *diag.Diagnostics) types.Object {
	if prior.IsNull() && !app.RollbackActive && strOrNull(app.RollbackRegistryID).IsNull() {
		return types.ObjectNull(rollbackAttrTypes)
	}
	obj, d := types.ObjectValueFrom(ctx, rollbackAttrTypes, rollbackModel{
		Enabled:    types.BoolValue(app.RollbackActive),
		RegistryID: strOrNull(app.RollbackRegistryID),
	})
	diags.Append(d...)
	return obj
}

// hideWriteOnly nulls preview_deployments.build_secrets in the state when
// the write-only form is in use: the server returns the secret in
// cleartext, and the state must not hold what the config keeps out of it.
func hideWriteOnly(ctx context.Context, m *resourceModel, inUse map[string]bool, diags *diag.Diagnostics) {
	if !inUse[previewSecretName] || m.PreviewDeployments.IsNull() || m.PreviewDeployments.IsUnknown() {
		return
	}
	var p previewModel
	if !decodeObject(ctx, m.PreviewDeployments, &p, diags) {
		return
	}
	p.BuildSecrets = types.StringNull()
	obj, d := types.ObjectValueFrom(ctx, previewAttrTypes, p)
	diags.Append(d...)
	m.PreviewDeployments = obj
}

// previewSecretInUse reports whether the config sets build_secrets_wo.
func previewSecretInUse(ctx context.Context, cfg resourceModel, diags *diag.Diagnostics) map[string]bool {
	var wo previewModel
	decodeObject(ctx, cfg.PreviewDeployments, &wo, diags)
	return map[string]bool{previewSecretName: !wo.BuildSecretsWo.IsNull()}
}
