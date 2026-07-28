package destination

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*destinationResource)(nil)
	_ resource.ResourceWithConfigure   = (*destinationResource)(nil)
	_ resource.ResourceWithImportState = (*destinationResource)(nil)
)

type destinationResource struct{ client *client.Client }

func NewResource() resource.Resource { return &destinationResource{} }

type resourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Provider        types.String `tfsdk:"provider_name"`
	Endpoint        types.String `tfsdk:"endpoint"`
	Bucket          types.String `tfsdk:"bucket"`
	Region          types.String `tfsdk:"region"`
	AccessKey       types.String `tfsdk:"access_key"`
	SecretAccessKey types.String `tfsdk:"secret_access_key"`
	AdditionalFlags types.List   `tfsdk:"additional_flags"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

func (r *destinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_destination"
}

func (r *destinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An S3-compatible bucket Dokploy writes backups to.\n\n" +
			"~> Dokploy stores and returns `access_key` and `secret_access_key` in cleartext. Both are marked " +
			"sensitive so Terraform will not print them, but anyone with API access to the instance can read them.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Destination id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Display name."},
			// Named provider_name, not provider: `provider` is a reserved
			// meta-argument in Terraform configuration and cannot be used as
			// an attribute name.
			"provider_name": schema.StringAttribute{
				Required:    true,
				Description: "Storage provider label, e.g. `Cloudflare`, `AWS`, `DigitalOcean`. Free text; Dokploy does not validate it.",
			},
			"endpoint": schema.StringAttribute{Required: true, Description: "S3 endpoint URL."},
			"bucket":   schema.StringAttribute{Required: true, Description: "Bucket name."},
			"region":   schema.StringAttribute{Required: true, Description: "Bucket region."},
			"access_key": schema.StringAttribute{
				Required: true, Sensitive: true, Description: "S3 access key id.",
			},
			"secret_access_key": schema.StringAttribute{
				Required: true, Sensitive: true, Description: "S3 secret access key.",
			},
			// Optional+Computed WITH a Default. Without the Default, removing
			// the attribute from config carries the prior value forward
			// (Computed semantics) instead of clearing it, so a flag could
			// never be removed. The server stores [] rather than null when
			// nothing is set, so the empty list is the correct default and
			// Read agrees with it.
			"additional_flags": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default: listdefault.StaticValue(
					types.ListValueMust(types.StringType, []attr.Value{}),
				),
				Description: "Extra flags passed to the underlying storage client. Defaults to an empty list; removing it from configuration clears any flags.",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp (server-side).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *destinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// flagsValue maps the server's additionalFlags onto the attribute. The
// server stores an empty array rather than null when nothing is set, so an
// empty list is the correct flattening — a null here would disagree with the
// Optional+Computed default and never converge.
func flagsValue(ctx context.Context, flags []string, diags *diag.Diagnostics) types.List {
	if flags == nil {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	list, d := types.ListValueFrom(ctx, types.StringType, flags)
	diags.Append(d...)
	return list
}

func flagsRequest(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	if list.IsNull() || list.IsUnknown() {
		return []string{}
	}
	var flags []string
	diags.Append(list.ElementsAs(ctx, &flags, false)...)
	return flags
}

func flatten(ctx context.Context, d *client.Destination, m *resourceModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(d.DestinationID)
	m.Name = types.StringValue(d.Name)
	m.Provider = types.StringValue(d.Provider)
	m.Endpoint = types.StringValue(d.Endpoint)
	m.Bucket = types.StringValue(d.Bucket)
	m.Region = types.StringValue(d.Region)
	m.AccessKey = types.StringValue(d.AccessKey)
	m.SecretAccessKey = types.StringValue(d.SecretAccessKey)
	m.AdditionalFlags = flagsValue(ctx, d.AdditionalFlags, diags)
	m.CreatedAt = types.StringValue(d.CreatedAt)
}

func (r *destinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateDestination(ctx, client.CreateDestinationRequest{
		Name:            plan.Name.ValueString(),
		Provider:        plan.Provider.ValueString(),
		Endpoint:        plan.Endpoint.ValueString(),
		Bucket:          plan.Bucket.ValueString(),
		Region:          plan.Region.ValueString(),
		AccessKey:       plan.AccessKey.ValueString(),
		SecretAccessKey: plan.SecretAccessKey.ValueString(),
		AdditionalFlags: flagsRequest(ctx, plan.AdditionalFlags, &resp.Diagnostics),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating destination", err.Error())
		return
	}
	flatten(ctx, created, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *destinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDestination(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading destination", err.Error())
		return
	}
	flatten(ctx, d, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *destinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateDestination(ctx, client.UpdateDestinationRequest{
		DestinationID:   plan.ID.ValueString(),
		Name:            plan.Name.ValueString(),
		Provider:        plan.Provider.ValueString(),
		Endpoint:        plan.Endpoint.ValueString(),
		Bucket:          plan.Bucket.ValueString(),
		Region:          plan.Region.ValueString(),
		AccessKey:       plan.AccessKey.ValueString(),
		SecretAccessKey: plan.SecretAccessKey.ValueString(),
		AdditionalFlags: flagsRequest(ctx, plan.AdditionalFlags, &resp.Diagnostics),
	}); err != nil {
		resp.Diagnostics.AddError("Updating destination", err.Error())
		return
	}
	d, err := r.client.GetDestination(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading destination after update", err.Error())
		return
	}
	flatten(ctx, d, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *destinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDestination(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting destination", err.Error())
	}
}

func (r *destinationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
