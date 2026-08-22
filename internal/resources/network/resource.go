// Package network holds the dokploy_network resource.
//
// Docker networks are immutable and Dokploy exposes no network.update
// endpoint, so every attribute below is RequiresReplace and Update is
// unreachable. network.recreate and network.networksToSync are deliberately
// unmodeled (spec decision 7): Terraform expresses a rebuild as replace or
// taint, and networksToSync is a UI import-dialog helper. network.import
// (adopting a Docker-level network into Dokploy's DB) is also unmodeled: a
// network outside Dokploy has no networkId for Terraform to address. The
// documented workaround: import it in the Dokploy UI once, then read it with
// the dokploy_network data source or `terraform import` the resource with
// the new id.
package network

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*networkResource)(nil)
	_ resource.ResourceWithConfigure   = (*networkResource)(nil)
	_ resource.ResourceWithImportState = (*networkResource)(nil)
)

type networkResource struct{ client *client.Client }

func NewResource() resource.Resource { return &networkResource{} }

type ipamConfigModel struct {
	Subnet  types.String `tfsdk:"subnet"`
	Gateway types.String `tfsdk:"gateway"`
	IPRange types.String `tfsdk:"ip_range"`
}

type ipamModel struct {
	Driver types.String `tfsdk:"driver"`
	Config types.List   `tfsdk:"config"`
}

type resourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Driver     types.String `tfsdk:"driver"`
	Internal   types.Bool   `tfsdk:"internal"`
	Attachable types.Bool   `tfsdk:"attachable"`
	EnableIPv4 types.Bool   `tfsdk:"enable_ipv4"`
	EnableIPv6 types.Bool   `tfsdk:"enable_ipv6"`
	MTU        types.Int64  `tfsdk:"mtu"`
	IPAM       types.Object `tfsdk:"ipam"`
	ServerID   types.String `tfsdk:"server_id"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func ipamConfigAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"subnet":   types.StringType,
		"gateway":  types.StringType,
		"ip_range": types.StringType,
	}
}

func ipamAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"driver": types.StringType,
		"config": types.ListType{ElemType: types.ObjectType{AttrTypes: ipamConfigAttrTypes()}},
	}
}

func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *networkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Docker network managed by Dokploy, for attaching services to " +
			"extra networks beyond the default `dokploy-network` (see `network_ids` on " +
			"`dokploy_application` and the database resources, and `service_networks` on `dokploy_compose`).\n\n" +
			"~> **Networks are immutable.** Dokploy exposes no update endpoint, so changing any " +
			"attribute replaces the network. Attached services keep working until their next deploy; " +
			"re-attach them to the replacement id (a `dokploy_network.<name>.id` reference does this " +
			"automatically) and redeploy.\n\n" +
			"~> **Deleting an attached network does not fail and does not detach it.** " +
			"`network.remove` succeeds even while an application still references the network in " +
			"its `network_ids`; the reference is left dangling until that application is next " +
			"updated or redeployed.\n\n" +
			"-> A network created outside Dokploy (e.g. `docker network create`) has no Dokploy id. " +
			"Import it once in the Dokploy UI (Networks -> Import), then read it with the " +
			"`dokploy_network` data source or `terraform import`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Network id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "Network name. Changing it replaces the network.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"driver": schema.StringAttribute{
				Optional: true, Computed: true,
				Default:       stringdefault.StaticString("bridge"),
				Description:   "Network driver: `bridge` or `overlay`. Defaults to `bridge`.",
				Validators:    []validator.String{stringvalidator.OneOf("bridge", "overlay")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"internal": schema.BoolAttribute{
				Optional: true, Computed: true,
				Default:       booldefault.StaticBool(false),
				Description:   "Restrict external access to the network.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"attachable": schema.BoolAttribute{
				Optional: true, Computed: true,
				Default:       booldefault.StaticBool(false),
				Description:   "Allow manual container attachment (overlay networks).",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"enable_ipv4": schema.BoolAttribute{
				Optional: true, Computed: true,
				Default:       booldefault.StaticBool(true),
				Description:   "Enable IPv4 on the network.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"enable_ipv6": schema.BoolAttribute{
				Optional: true, Computed: true,
				Default:       booldefault.StaticBool(false),
				Description:   "Enable IPv6 on the network.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"mtu": schema.Int64Attribute{
				Optional:      true,
				Description:   "MTU for the network, 68-65535. Unset leaves Docker's default.",
				Validators:    []validator.Int64{int64validator.Between(68, 65535)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"ipam": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Custom IP address management. Unset leaves Docker's defaults.",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.RequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"driver": schema.StringAttribute{Optional: true, Description: "IPAM driver, e.g. `default`."},
					"config": schema.ListNestedAttribute{
						Optional:    true,
						Description: "Address pools.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"subnet":   schema.StringAttribute{Optional: true, Description: "Pool subnet in CIDR form, e.g. `172.28.0.0/16`."},
								"gateway":  schema.StringAttribute{Optional: true, Description: "Gateway address for the pool."},
								"ip_range": schema.StringAttribute{Optional: true, Description: "Sub-range to allocate from, in CIDR form."},
							},
						},
					},
				},
			},
			"server_id": schema.StringAttribute{
				Optional:      true,
				Description:   "Id of the remote server to create the network on. Unset targets the Dokploy host.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp (server-side).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func expandIPAM(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.NetworkIPAM {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var m ipamModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	out := &client.NetworkIPAM{Driver: m.Driver.ValueString()}
	if !m.Config.IsNull() && !m.Config.IsUnknown() {
		var cfgs []ipamConfigModel
		diags.Append(m.Config.ElementsAs(ctx, &cfgs, false)...)
		for _, c := range cfgs {
			out.Config = append(out.Config, client.NetworkIPAMConfig{
				Subnet:  c.Subnet.ValueString(),
				Gateway: c.Gateway.ValueString(),
				IPRange: c.IPRange.ValueString(),
			})
		}
	}
	return out
}

// flattenIPAM collapses both nil and the "empty shape" to a null object.
//
// An omitted ipam key on network.create reads back as an empty object, {}
// (driver "", config nil) - not null and not a materialized Docker default
// (internal/client/doc.go, wave 6b "ipam: an omitted key and an explicit
// null are not the same shape" section). This client's Create always sends
// an explicit JSON null when ipam is unset (CreateNetworkRequest.IPAM has
// no omitempty), so the {} shape never reaches this function through
// Create. But `terraform import` against a network that was created
// outside this provider - through the Dokploy UI, say - can read back {}.
// A flatten that turned {} into a populated object would leave that
// imported resource diffing forever against a config that omits ipam, so
// both shapes must collapse to null here.
func flattenIPAM(ctx context.Context, ipam *client.NetworkIPAM, diags *diag.Diagnostics) types.Object {
	if ipam == nil || (ipam.Driver == "" && len(ipam.Config) == 0) {
		return types.ObjectNull(ipamAttrTypes())
	}
	cfgs := make([]attr.Value, 0, len(ipam.Config))
	for _, c := range ipam.Config {
		cfgs = append(cfgs, types.ObjectValueMust(ipamConfigAttrTypes(), map[string]attr.Value{
			"subnet":   tfutil.StringOrNull(&c.Subnet),
			"gateway":  tfutil.StringOrNull(&c.Gateway),
			"ip_range": tfutil.StringOrNull(&c.IPRange),
		}))
	}
	configList := types.ListNull(types.ObjectType{AttrTypes: ipamConfigAttrTypes()})
	if len(cfgs) > 0 {
		configList = types.ListValueMust(types.ObjectType{AttrTypes: ipamConfigAttrTypes()}, cfgs)
	}
	obj, d := types.ObjectValue(ipamAttrTypes(), map[string]attr.Value{
		"driver": tfutil.StringOrNull(&ipam.Driver),
		"config": configList,
	})
	diags.Append(d...)
	return obj
}

func flatten(ctx context.Context, n *client.Network, m *resourceModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(n.NetworkID)
	m.Name = types.StringValue(n.Name)
	m.Driver = types.StringValue(n.Driver)
	m.Internal = types.BoolValue(n.Internal)
	m.Attachable = types.BoolValue(n.Attachable)
	m.EnableIPv4 = types.BoolValue(n.EnableIPv4)
	m.EnableIPv6 = types.BoolValue(n.EnableIPv6)
	m.MTU = types.Int64PointerValue(n.MTU)
	m.IPAM = flattenIPAM(ctx, n.IPAM, diags)
	m.ServerID = tfutil.StringOrNull(n.ServerID)
	m.CreatedAt = types.StringValue(n.CreatedAt)
}

func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateNetwork(ctx, client.CreateNetworkRequest{
		Name:       plan.Name.ValueString(),
		Driver:     plan.Driver.ValueString(),
		Internal:   plan.Internal.ValueBool(),
		Attachable: plan.Attachable.ValueBool(),
		EnableIPv4: plan.EnableIPv4.ValueBool(),
		EnableIPv6: plan.EnableIPv6.ValueBool(),
		MTU:        plan.MTU.ValueInt64Pointer(),
		IPAM:       expandIPAM(ctx, plan.IPAM, &resp.Diagnostics),
		ServerID:   plan.ServerID.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating network", err.Error())
		return
	}
	flatten(ctx, created, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	n, err := r.client.GetNetwork(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading network", err.Error())
		return
	}
	flatten(ctx, n, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update can never be reached: every attribute is RequiresReplace because
// Dokploy has no network.update endpoint. Reaching it is a provider bug.
func (r *networkResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Updating network",
		"dokploy_network has no update path; every attribute requires replacement. This is a bug in the provider - please report it.",
	)
}

func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNetwork(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting network", err.Error())
	}
}

func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
