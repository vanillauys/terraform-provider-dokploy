// Package tag holds the dokploy_tag resource.
package tag

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*tagResource)(nil)
	_ resource.ResourceWithConfigure   = (*tagResource)(nil)
	_ resource.ResourceWithImportState = (*tagResource)(nil)
)

type tagResource struct{ client *client.Client }

func NewResource() resource.Resource { return &tagResource{} }

type resourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Color          types.String `tfsdk:"color"`
	CreatedAt      types.String `tfsdk:"created_at"`
	OrganizationID types.String `tfsdk:"organization_id"`
}

func (r *tagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *tagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A label that a project carries. Assign it with `tag_ids` on `dokploy_project`. The Dokploy UI " +
			"filters the project list by tag.\n\n" +
			"~> Tag names are unique in the organization. A second tag with the same name fails on apply with the " +
			"database error that Dokploy returns.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Tag id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name, unique in the organization. Dokploy rejects an empty name.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"color": schema.StringAttribute{
				Optional: true,
				Description: "Colour of the label in the Dokploy UI, as a CSS colour such as `#0a8a74`. Dokploy stores " +
					"the string as is and does not validate it. Omit the attribute for no colour; an empty string is " +
					"rejected at plan time, because Dokploy would store it and the provider reads it back as null.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp from the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				Computed:      true,
				Description:   "Id of the organization that owns the tag. The provider fills it from the API key's active organization.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *tagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(tag *client.Tag, m *resourceModel) {
	m.ID = types.StringValue(tag.TagID)
	m.Name = types.StringValue(tag.Name)
	m.Color = tfutil.StringOrNull(tag.Color)
	m.CreatedAt = types.StringValue(tag.CreatedAt)
	m.OrganizationID = types.StringValue(tag.OrganizationID)
}

// colorRequest maps a null attribute to nil, which marshals as the null
// that clears the colour (client.UpdateTagRequest).
func colorRequest(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func (r *tagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateTag(ctx, client.CreateTagRequest{
		Name:  plan.Name.ValueString(),
		Color: colorRequest(plan.Color),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating tag", err.Error())
		return
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tag, err := r.client.GetTag(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading tag", err.Error())
		return
	}
	flatten(tag, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateTag(ctx, client.UpdateTagRequest{
		TagID: plan.ID.ValueString(),
		Name:  plan.Name.ValueString(),
		Color: colorRequest(plan.Color),
	}); err != nil {
		resp.Diagnostics.AddError("Updating tag", err.Error())
		return
	}
	tag, err := r.client.GetTag(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading tag after update", err.Error())
		return
	}
	flatten(tag, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTag(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting tag", err.Error())
	}
}

func (r *tagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
