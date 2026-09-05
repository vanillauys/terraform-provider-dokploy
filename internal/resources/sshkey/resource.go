// Package sshkey holds the dokploy_ssh_key resource.
package sshkey

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*sshKeyResource)(nil)
	_ resource.ResourceWithConfigure   = (*sshKeyResource)(nil)
	_ resource.ResourceWithImportState = (*sshKeyResource)(nil)
)

type sshKeyResource struct{ client *client.Client }

func NewResource() resource.Resource { return &sshKeyResource{} }

type resourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	PublicKey   types.String `tfsdk:"public_key"`
	PrivateKey  types.String `tfsdk:"private_key"`
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries a _wo value; the plan and the state hold null for it.
	PrivateKeyWo        types.String `tfsdk:"private_key_wo"`
	PrivateKeyWoVersion types.Int64  `tfsdk:"private_key_wo_version"`
	OrganizationID      types.String `tfsdk:"organization_id"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

// secretNames lists the attributes with write-only companions.
var secretNames = []string{"private_key"}

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "SSH key id. `dokploy_server.ssh_key_id` and the `git.ssh_key_id` of an application or a compose reference it.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"description": schema.StringAttribute{
			Optional:    true,
			Description: "Free-text description. If you remove it from the configuration, the provider clears it on the server.",
		},
		"public_key": schema.StringAttribute{
			Required:      true,
			Description:   "Public key in OpenSSH format, for example `ssh-ed25519 AAAA... deploy`. A change replaces the resource.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		// Optional, not Required, only because the write-only companion can
		// replace it; the ExactlyOneOf validator on the companion still
		// demands one of the two.
		"private_key": schema.StringAttribute{
			Optional:      true,
			Sensitive:     true,
			Description:   "Private key in OpenSSH or PEM format. Set this attribute or `private_key_wo`. A change replaces the resource.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"organization_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the organization that owns the key. The provider fills it from the API key's active organization.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("private_key", tfutil.WriteOnlyOptions{
		ExactlyOne: true,
		Effect:     "A version change replaces the resource, because Dokploy cannot change a stored key pair.",
	}) {
		if name == "private_key_wo_version" {
			// Dokploy has no update path for the key material, so a new
			// write-only value means a new key pair, and a new key pair means
			// a new record.
			version := attr.(schema.Int64Attribute)
			version.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
			attr = version
		}
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "An SSH key pair that Dokploy uses to reach a remote server (`dokploy_server`) or a private git " +
			"repository (the `git` source of an application or a compose).\n\n" +
			"~> **Dokploy stores and returns `private_key` in cleartext.** The attribute is sensitive, so Terraform does not " +
			"print it, but anyone with API access to the server can read it. The `private_key_wo` companion keeps it out " +
			"of the Terraform state.\n\n" +
			"~> **Dokploy cannot change a stored key pair.** A change to `public_key`, `private_key`, or " +
			"`private_key_wo_version` replaces the resource. Each `dokploy_server` that references the key through " +
			"`ssh_key_id` then updates to the new id in the same apply.\n\n" +
			"~> **Dokploy validates the private key format.** Supply a real key, for example from the `tls_private_key` " +
			"resource of the `hashicorp/tls` provider or from `ssh-keygen`. A placeholder string fails with " +
			"`Invalid private key format`.",
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

func (r *sshKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(k *client.SSHKey, m *resourceModel) {
	m.ID = types.StringValue(k.SSHKeyID)
	m.Name = types.StringValue(k.Name)
	m.Description = tfutil.StringOrNull(&k.Description)
	m.PublicKey = types.StringValue(k.PublicKey)
	m.PrivateKey = types.StringValue(k.PrivateKey)
	m.OrganizationID = types.StringValue(k.OrganizationID)
	m.CreatedAt = types.StringValue(k.CreatedAt)
}

// descriptionRequest maps the attribute onto the dialect B field: nil
// marshals to an explicit null, which clears the stored value.
func descriptionRequest(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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
	created, err := r.client.CreateSSHKey(ctx, client.CreateSSHKeyRequest{
		Name:           plan.Name.ValueString(),
		Description:    plan.Description.ValueString(),
		OrganizationID: org.ID,
		PublicKey:      plan.PublicKey.ValueString(),
		PrivateKey:     tfutil.SecretToCreate(plan.PrivateKey, cfg.PrivateKeyWo),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating SSH key", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	k, err := r.client.GetSSHKey(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading SSH key", err.Error())
		return
	}
	flatten(k, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes the name and the description only: the key material is
// RequiresReplace, and sshKey.update accepts neither half of it.
func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"private_key": !cfg.PrivateKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	if err := r.client.UpdateSSHKey(ctx, client.UpdateSSHKeyRequest{
		SSHKeyID:    plan.ID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: descriptionRequest(plan.Description),
	}); err != nil {
		resp.Diagnostics.AddError("Updating SSH key", err.Error())
		return
	}
	k, err := r.client.GetSSHKey(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading SSH key after update", err.Error())
		return
	}
	flatten(k, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSSHKey(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting SSH key", err.Error())
	}
}

func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
