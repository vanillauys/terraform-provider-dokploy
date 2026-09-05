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
	// The write-only companions (tfutil.WriteOnlyCompanions). Only the
	// config carries a _wo value; the plan and the state hold null for it.
	AccessKeyWo              types.String `tfsdk:"access_key_wo"`
	AccessKeyWoVersion       types.Int64  `tfsdk:"access_key_wo_version"`
	SecretAccessKeyWo        types.String `tfsdk:"secret_access_key_wo"`
	SecretAccessKeyWoVersion types.Int64  `tfsdk:"secret_access_key_wo_version"`
	AdditionalFlags          types.List   `tfsdk:"additional_flags"`
	CreatedAt                types.String `tfsdk:"created_at"`
}

func (r *destinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_destination"
}

func (r *destinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
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
			Description: "Storage provider label, for example `Cloudflare`, `AWS`, or `DigitalOcean`. Free text. Dokploy does not validate it.",
		},
		"endpoint": schema.StringAttribute{Required: true, Description: "S3 endpoint URL."},
		"bucket":   schema.StringAttribute{Required: true, Description: "Bucket name."},
		"region":   schema.StringAttribute{Required: true, Description: "Bucket region."},
		// Both credentials are Optional, not Required, only because their
		// write-only companions can replace them; the ExactlyOneOf
		// validator on each companion still demands one of the two.
		"access_key": schema.StringAttribute{
			Optional: true, Sensitive: true, Description: "S3 access key id. Set this attribute or `access_key_wo`.",
		},
		"secret_access_key": schema.StringAttribute{
			Optional: true, Sensitive: true, Description: "S3 secret access key. Set this attribute or `secret_access_key_wo`.",
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
			Description: "Extra flags for the storage client. Defaults to an empty list. If you remove it from the configuration, the provider clears the flags.",
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for _, name := range secretNames {
		for k, v := range tfutil.WriteOnlyCompanions(name, tfutil.WriteOnlyOptions{ExactlyOne: true}) {
			attrs[k] = v
		}
	}
	resp.Schema = schema.Schema{
		Description: "An S3-compatible bucket that receives Dokploy backups.\n\n" +
			"~> Dokploy stores and returns `access_key` and `secret_access_key` in cleartext. Both attributes are " +
			"sensitive, so Terraform does not print them, but anyone with API access to the server can read them. " +
			"The `access_key_wo` and `secret_access_key_wo` companions keep them out of the Terraform state.",
		Attributes: attrs,
	}
}

// secretNames lists the attributes with write-only companions.
var secretNames = []string{"access_key", "secret_access_key"}

// hideWriteOnly nulls each secret whose companion is in use (inUse is keyed
// by secretNames), after flatten put the server's cleartext value in.
func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["access_key"] {
		m.AccessKey = types.StringNull()
	}
	if inUse["secret_access_key"] {
		m.SecretAccessKey = types.StringNull()
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
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The config, not the plan, carries the write-only values: the
	// framework nulls them in the plan (tfutil.WriteOnlyCompanions).
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"access_key": !cfg.AccessKeyWo.IsNull(), "secret_access_key": !cfg.SecretAccessKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	created, err := r.client.CreateDestination(ctx, client.CreateDestinationRequest{
		Name:            plan.Name.ValueString(),
		Provider:        plan.Provider.ValueString(),
		Endpoint:        plan.Endpoint.ValueString(),
		Bucket:          plan.Bucket.ValueString(),
		Region:          plan.Region.ValueString(),
		AccessKey:       tfutil.SecretToCreate(plan.AccessKey, cfg.AccessKeyWo),
		SecretAccessKey: tfutil.SecretToCreate(plan.SecretAccessKey, cfg.SecretAccessKeyWo),
		AdditionalFlags: flagsRequest(ctx, plan.AdditionalFlags, &resp.Diagnostics),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating destination", err.Error())
		return
	}
	flatten(ctx, created, &plan, &resp.Diagnostics)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *destinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	// The flags say which credentials stay out of the state: the API
	// returns both in cleartext, so a refresh would put them back otherwise.
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
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
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *destinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"access_key": !cfg.AccessKeyWo.IsNull(), "secret_access_key": !cfg.SecretAccessKeyWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	accessKey, sendKey := tfutil.SecretToUpdate(plan.AccessKey, cfg.AccessKeyWo, state.AccessKey, plan.AccessKeyWoVersion, state.AccessKeyWoVersion)
	secret, sendSecret := tfutil.SecretToUpdate(plan.SecretAccessKey, cfg.SecretAccessKeyWo, state.SecretAccessKey, plan.SecretAccessKeyWoVersion, state.SecretAccessKeyWoVersion)
	if !sendKey || !sendSecret {
		// destination.update carries the full field set (client/
		// destination.go), so a write-only credential with nothing new to
		// send resends the stored one, which the API returns in cleartext.
		current, err := r.client.GetDestination(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading destination before update", err.Error())
			return
		}
		if !sendKey {
			accessKey = current.AccessKey
		}
		if !sendSecret {
			secret = current.SecretAccessKey
		}
	}
	if err := r.client.UpdateDestination(ctx, client.UpdateDestinationRequest{
		DestinationID:   plan.ID.ValueString(),
		Name:            plan.Name.ValueString(),
		Provider:        plan.Provider.ValueString(),
		Endpoint:        plan.Endpoint.ValueString(),
		Bucket:          plan.Bucket.ValueString(),
		Region:          plan.Region.ValueString(),
		AccessKey:       accessKey,
		SecretAccessKey: secret,
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
	hideWriteOnly(&plan, inUse)
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
