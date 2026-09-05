// Package certificate holds the dokploy_certificate resource.
package certificate

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*certificateResource)(nil)
	_ resource.ResourceWithConfigure   = (*certificateResource)(nil)
	_ resource.ResourceWithImportState = (*certificateResource)(nil)
)

type certificateResource struct{ client *client.Client }

func NewResource() resource.Resource { return &certificateResource{} }

type resourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CertificateData types.String `tfsdk:"certificate_data"`
	PrivateKey      types.String `tfsdk:"private_key"`
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries a _wo value; the plan and the state hold null for it.
	PrivateKeyWo        types.String `tfsdk:"private_key_wo"`
	PrivateKeyWoVersion types.Int64  `tfsdk:"private_key_wo_version"`
	AutoRenew           types.Bool   `tfsdk:"auto_renew"`
	ServerID            types.String `tfsdk:"server_id"`
	CertificatePath     types.String `tfsdk:"certificate_path"`
	OrganizationID      types.String `tfsdk:"organization_id"`
}

// secretNames lists the attributes with write-only companions.
var secretNames = []string{"private_key"}

func (r *certificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *certificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "Certificate id.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"certificate_data": schema.StringAttribute{
			Required:    true,
			Description: "The certificate chain in PEM format. Read it from a file with `file()`, or take it from an ACME provider resource.",
		},
		// Optional, not Required, only because the write-only companion can
		// replace it; the ExactlyOneOf validator on the companion still
		// demands one of the two.
		"private_key": schema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "The private key in PEM format. Set this attribute or `private_key_wo`.",
		},
		"auto_renew": schema.BoolAttribute{
			Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			Description: "Whether Dokploy renews the certificate. Defaults to `false`. Dokploy cannot change it on a stored " +
				"certificate, so a change replaces the resource.",
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
		},
		"server_id": schema.StringAttribute{
			Optional: true,
			Description: "Id of the `dokploy_server` that serves the certificate. Omit it for the Dokploy host. A change " +
				"replaces the resource.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"certificate_path": schema.StringAttribute{
			Computed:      true,
			Description:   "Name of the Traefik certificate file that Dokploy generates.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"organization_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the organization that owns the certificate. The provider fills it from the API key's active organization.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("private_key", tfutil.WriteOnlyOptions{ExactlyOne: true}) {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "A TLS certificate that Traefik serves for the domains that reference it. Upload the chain and the " +
			"key here, then select the certificate on a `dokploy_domain` with `certificate_type = \"custom\"`.\n\n" +
			"~> **Dokploy stores and returns `private_key` in cleartext.** The attribute is sensitive, so Terraform does not " +
			"print it, but anyone with API access to the server can read it. The `private_key_wo` companion keeps it out " +
			"of the Terraform state.\n\n" +
			"~> Dokploy does not validate the PEM content on create or update. A malformed certificate applies " +
			"successfully and fails when Traefik loads it.",
		Attributes: attrs,
	}
}

// hideWriteOnly nulls the secret when its companion is in use, after flatten
// put the server's cleartext value in.
func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["private_key"] {
		m.PrivateKey = types.StringNull()
	}
}

func (r *certificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(cert *client.Certificate, m *resourceModel) {
	m.ID = types.StringValue(cert.CertificateID)
	m.Name = types.StringValue(cert.Name)
	m.CertificateData = types.StringValue(cert.CertificateData)
	m.PrivateKey = types.StringValue(cert.PrivateKey)
	m.AutoRenew = types.BoolValue(cert.AutoRenew)
	m.ServerID = tfutil.StringOrNull(&cert.ServerID)
	m.CertificatePath = types.StringValue(cert.CertificatePath)
	m.OrganizationID = types.StringValue(cert.OrganizationID)
}

func serverRequest(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only value: the framework
	// nulls it in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"private_key": !cfg.PrivateKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	org, err := r.client.GetActiveOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading the active organization", err.Error())
		return
	}
	created, err := r.client.CreateCertificate(ctx, client.CreateCertificateRequest{
		Name:            plan.Name.ValueString(),
		CertificateData: plan.CertificateData.ValueString(),
		PrivateKey:      tfutil.SecretToCreate(plan.PrivateKey, cfg.PrivateKeyWo),
		AutoRenew:       plan.AutoRenew.ValueBool(),
		OrganizationID:  org.ID,
		ServerID:        serverRequest(plan.ServerID),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating certificate", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	cert, err := r.client.GetCertificate(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading certificate", err.Error())
		return
	}
	flatten(cert, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"private_key": !cfg.PrivateKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	key, send := tfutil.SecretToUpdate(plan.PrivateKey, cfg.PrivateKeyWo, state.PrivateKey, plan.PrivateKeyWoVersion, state.PrivateKeyWoVersion)
	if !send {
		// certificates.update carries the full body (client/certificate.go),
		// so a write-only key with nothing new to send resends the stored
		// one, which the API returns in cleartext.
		current, err := r.client.GetCertificate(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading certificate before update", err.Error())
			return
		}
		key = current.PrivateKey
	}
	if err := r.client.UpdateCertificate(ctx, client.UpdateCertificateRequest{
		CertificateID:   plan.ID.ValueString(),
		Name:            plan.Name.ValueString(),
		CertificateData: plan.CertificateData.ValueString(),
		PrivateKey:      key,
	}); err != nil {
		resp.Diagnostics.AddError("Updating certificate", err.Error())
		return
	}
	cert, err := r.client.GetCertificate(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading certificate after update", err.Error())
		return
	}
	flatten(cert, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCertificate(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting certificate", err.Error())
	}
}

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
