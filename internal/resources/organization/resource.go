// Package organization holds the dokploy_organization resource.
package organization

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*organizationResource)(nil)
	_ resource.ResourceWithConfigure   = (*organizationResource)(nil)
	_ resource.ResourceWithImportState = (*organizationResource)(nil)
)

type organizationResource struct{ client *client.Client }

func NewResource() resource.Resource { return &organizationResource{} }

type resourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Logo        types.String `tfsdk:"logo"`
	DefaultRole types.String `tfsdk:"default_role"`
	Slug        types.String `tfsdk:"slug"`
	OwnerID     types.String `tfsdk:"owner_id"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (r *organizationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (r *organizationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An organization: the top-level tenant that owns projects, servers, git providers, and members. The " +
			"API key's user becomes the owner of an organization that Terraform creates.\n\n" +
			"~> **The provider works inside the API key's active organization and cannot switch to another one.** Every " +
			"other resource of this provider lands in the active organization. Use this resource to keep an " +
			"organization record under Terraform (its name, logo, and default role), not to place resources in it. To " +
			"manage resources in a second organization, configure a second provider instance with an API key whose " +
			"active organization is that one.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Organization id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Display name."},
			"logo": schema.StringAttribute{
				Optional:    true,
				Description: "URL of the logo image. If you remove it from the configuration, the provider clears it.",
			},
			"default_role": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Role that a new member gets: `member`, `admin`, or the name of a custom role. Dokploy " +
					"rejects an unknown role name and keeps the stored role when you remove the attribute, so the " +
					"provider keeps the last value in the state after a removal.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"slug": schema.StringAttribute{
				Computed:      true,
				Description:   "URL slug that Dokploy generates.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"owner_id": schema.StringAttribute{
				Computed:      true,
				Description:   "User id of the owner: the user of the API key that created the organization.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp from the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *organizationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(o *client.Organization, m *resourceModel) {
	m.ID = types.StringValue(o.ID)
	m.Name = types.StringValue(o.Name)
	m.Logo = tfutil.StringOrNull(&o.Logo)
	m.DefaultRole = tfutil.StringOrNull(&o.DefaultRole)
	m.Slug = tfutil.StringOrNull(&o.Slug)
	m.OwnerID = types.StringValue(o.OwnerID)
	m.CreatedAt = types.StringValue(o.CreatedAt)
}

func (r *organizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateOrganization(ctx, client.CreateOrganizationRequest{
		Name: plan.Name.ValueString(),
		Logo: plan.Logo.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating organization", err.Error())
		return
	}
	if !plan.DefaultRole.IsNull() && !plan.DefaultRole.IsUnknown() {
		// organization.create has no defaultRole field; the follow-up
		// update sets it.
		if err := r.client.UpdateOrganization(ctx, updateRequest(created.ID, plan)); err != nil {
			resp.Diagnostics.AddError("Setting the default role after create", err.Error())
			return
		}
		if created, err = r.client.GetOrganization(ctx, created.ID); err != nil {
			resp.Diagnostics.AddError("Reading organization after create", err.Error())
			return
		}
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func updateRequest(id string, m resourceModel) client.UpdateOrganizationRequest {
	req := client.UpdateOrganizationRequest{
		OrganizationID: id,
		Name:           m.Name.ValueString(),
		Logo:           m.Logo.ValueString(),
	}
	if !m.DefaultRole.IsNull() && !m.DefaultRole.IsUnknown() {
		req.DefaultRole = m.DefaultRole.ValueStringPointer()
	}
	return req
}

func (r *organizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, err := r.client.GetOrganization(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading organization", err.Error())
		return
	}
	flatten(o, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *organizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateOrganization(ctx, updateRequest(plan.ID.ValueString(), plan)); err != nil {
		resp.Diagnostics.AddError("Updating organization", err.Error())
		return
	}
	o, err := r.client.GetOrganization(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading organization after update", err.Error())
		return
	}
	flatten(o, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *organizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteOrganization(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting organization", err.Error())
	}
}

func (r *organizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
