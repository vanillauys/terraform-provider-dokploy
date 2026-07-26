package domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

type resourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Host               types.String `tfsdk:"host"`
	ApplicationID      types.String `tfsdk:"application_id"`
	ComposeID          types.String `tfsdk:"compose_id"`
	Path               types.String `tfsdk:"path"`
	InternalPath       types.String `tfsdk:"internal_path"`
	Port               types.Int64  `tfsdk:"port"`
	HTTPS              types.Bool   `tfsdk:"https"`
	StripPath          types.Bool   `tfsdk:"strip_path"`
	CertificateType    types.String `tfsdk:"certificate_type"`
	CustomCertResolver types.String `tfsdk:"custom_cert_resolver"`
	CustomEntrypoint   types.String `tfsdk:"custom_entrypoint"`
	ServiceName        types.String `tfsdk:"service_name"`
	ForwardAuthEnabled types.Bool   `tfsdk:"forward_auth_enabled"`
	Middlewares        types.List   `tfsdk:"middlewares"`
	DomainType         types.String `tfsdk:"domain_type"`
	UniqueConfigKey    types.Int64  `tfsdk:"unique_config_key"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

// domainTypeFor derives the server-side domainType from the attachment.
//
// The schema does not accept domain_type from config. Its two user-facing
// values are fully determined by which of application_id / compose_id is set,
// so taking it from config would only create a way to contradict yourself.
// (The third value the API accepts, "preview", belongs to preview
// deployments, which this provider does not manage.) The server defaults the
// field to "application" no matter which id it receives, so a compose domain
// must state it explicitly.
func domainTypeFor(m *resourceModel) string {
	if !m.ComposeID.IsNull() {
		return "compose"
	}
	return "application"
}

// flatten maps a full API record into the model (used by Read).
func flatten(ctx context.Context, d *client.Domain, m *resourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(d.DomainID)
	m.Host = types.StringValue(d.Host)
	m.Path = types.StringValue(d.Path)
	m.InternalPath = types.StringValue(d.InternalPath)
	m.Port = types.Int64Value(d.Port)
	m.HTTPS = types.BoolValue(d.HTTPS)
	m.StripPath = types.BoolValue(d.StripPath)
	m.CertificateType = types.StringValue(d.CertificateType)
	m.CustomCertResolver = types.StringPointerValue(d.CustomCertResolver)
	m.CustomEntrypoint = types.StringPointerValue(d.CustomEntrypoint)
	m.ServiceName = types.StringPointerValue(d.ServiceName)
	m.ForwardAuthEnabled = types.BoolValue(d.ForwardAuthEnabled)
	m.DomainType = types.StringValue(d.DomainType)
	m.UniqueConfigKey = types.Int64Value(d.UniqueConfigKey)
	m.ApplicationID = types.StringPointerValue(d.ApplicationID)
	m.ComposeID = types.StringPointerValue(d.ComposeID)
	m.CreatedAt = types.StringValue(d.CreatedAt)

	list, listDiags := types.ListValueFrom(ctx, types.StringType, d.Middlewares)
	diags.Append(listDiags...)
	m.Middlewares = list
	return diags
}

// setComputed copies only server-computed fields, leaving planned values
// intact so Create/Update cannot trip "inconsistent result after apply".
func setComputed(ctx context.Context, d *client.Domain, m *resourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(d.DomainID)
	m.DomainType = types.StringValue(d.DomainType)
	m.UniqueConfigKey = types.Int64Value(d.UniqueConfigKey)
	m.CreatedAt = types.StringValue(d.CreatedAt)

	list, listDiags := types.ListValueFrom(ctx, types.StringType, d.Middlewares)
	diags.Append(listDiags...)
	m.Middlewares = list
	return diags
}

// expandCreate builds the create payload. Every field is sent explicitly.
func expandCreate(m *resourceModel) client.CreateDomainRequest {
	return client.CreateDomainRequest{
		Host:               m.Host.ValueString(),
		Path:               m.Path.ValueString(),
		InternalPath:       m.InternalPath.ValueString(),
		Port:               m.Port.ValueInt64(),
		HTTPS:              m.HTTPS.ValueBool(),
		StripPath:          m.StripPath.ValueBool(),
		CertificateType:    m.CertificateType.ValueString(),
		CustomCertResolver: m.CustomCertResolver.ValueStringPointer(),
		CustomEntrypoint:   m.CustomEntrypoint.ValueStringPointer(),
		ServiceName:        m.ServiceName.ValueStringPointer(),
		ForwardAuthEnabled: m.ForwardAuthEnabled.ValueBool(),
		DomainType:         domainTypeFor(m),
		ApplicationID:      m.ApplicationID.ValueStringPointer(),
		ComposeID:          m.ComposeID.ValueStringPointer(),
	}
}

// expandUpdate builds the update payload. Dialect B means every managed field
// must appear on every call, or the omitted ones silently keep their old
// values and can never be cleared.
func expandUpdate(m *resourceModel) client.UpdateDomainRequest {
	return client.UpdateDomainRequest{
		DomainID:           m.ID.ValueString(),
		Host:               m.Host.ValueString(),
		Path:               m.Path.ValueString(),
		InternalPath:       m.InternalPath.ValueString(),
		Port:               m.Port.ValueInt64(),
		HTTPS:              m.HTTPS.ValueBool(),
		StripPath:          m.StripPath.ValueBool(),
		CertificateType:    m.CertificateType.ValueString(),
		CustomCertResolver: m.CustomCertResolver.ValueStringPointer(),
		CustomEntrypoint:   m.CustomEntrypoint.ValueStringPointer(),
		ServiceName:        m.ServiceName.ValueStringPointer(),
		ForwardAuthEnabled: m.ForwardAuthEnabled.ValueBool(),
		DomainType:         domainTypeFor(m),
		ApplicationID:      m.ApplicationID.ValueStringPointer(),
		ComposeID:          m.ComposeID.ValueStringPointer(),
	}
}
