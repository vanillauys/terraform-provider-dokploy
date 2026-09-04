package mount

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                     = (*mountResource)(nil)
	_ resource.ResourceWithConfigure        = (*mountResource)(nil)
	_ resource.ResourceWithImportState      = (*mountResource)(nil)
	_ resource.ResourceWithConfigValidators = (*mountResource)(nil)
)

type mountResource struct{ client *client.Client }

func NewResource() resource.Resource { return &mountResource{} }

func (r *mountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mount"
}

// ConfigValidators enforces the per-type field rules at PLAN time.
//
// These are provider policy, not a server contract: Dokploy accepts a
// type="bind" mount with no host_path and stores it with hostPath null
// (verified live, v0.29.13, 2026-07-28). Such a mount is silently useless,
// so the provider refuses it up front rather than letting the apply
// "succeed" into a broken container spec.
func (r *mountResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("host_path"),
			path.MatchRoot("volume_name"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("host_path"),
			path.MatchRoot("content"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("volume_name"),
			path.MatchRoot("content"),
		),
		resourcevalidator.RequiredTogether(
			path.MatchRoot("content"),
			path.MatchRoot("file_path"),
		),
	}
}

func (r *mountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A volume, bind, or file mount on a Dokploy service.\n\n" +
			"~> **Database services create their own data mount.** A `dokploy_postgres` (or mysql/mariadb/mongo/redis/libsql) " +
			"owns a volume mount for its data directory from the moment of its creation, without a request for it. " +
			"That mount belongs to the server, not to Terraform. Do not import it, and do not declare it here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Mount id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_id": schema.StringAttribute{
				Required: true,
				Description: "Id of the service that this mount attaches to. A change forces a replacement: the Dokploy update endpoint " +
					"sets the new parent without a clear of the old one, and the mount then belongs to two services at once.",
				PlanModifiers: requiresReplace,
			},
			"service_type": schema.StringAttribute{
				Required: true,
				Description: "Kind of service that `service_id` refers to: one of `application`, `postgres`, `mysql`, `mariadb`, " +
					"`mongo`, `redis`, `compose`, `libsql`. A change forces a replacement, for the same reason as `service_id`.",
				PlanModifiers: requiresReplace,
				Validators:    []validator.String{stringvalidator.OneOf(client.MountServiceTypes...)},
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Mount kind: `bind` (a host path), `volume` (a named Docker volume), or `file` (inline content that Dokploy writes into the container).",
				Validators:  []validator.String{stringvalidator.OneOf("bind", "volume", "file")},
			},
			"mount_path": schema.StringAttribute{
				Required:    true,
				Description: "Mount path inside the container.",
			},
			"host_path": schema.StringAttribute{
				Optional:    true,
				Description: "Absolute path on the host. Required when `type` is `bind`, and invalid otherwise.",
			},
			"volume_name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of the Docker volume. Required when `type` is `volume`, and invalid otherwise.",
			},
			"file_path": schema.StringAttribute{
				Optional:    true,
				Description: "File name to create inside `mount_path`. Required when `type` is `file`, and invalid otherwise.",
			},
			"content": schema.StringAttribute{
				Optional:    true,
				Description: "Contents of the file. Required when `type` is `file`, and invalid otherwise.",
			},
		},
	}
}

func (r *mountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// validateSubtype enforces the required field for each `type`. It runs in
// Create and Update rather than as a ConfigValidator because the rule is
// conditional on another attribute's value, which the stock config
// validators cannot express.
func validateSubtype(m resourceModel) error {
	switch m.Type.ValueString() {
	case "bind":
		if m.HostPath.IsNull() {
			return fmt.Errorf("`host_path` is required when `type` is `bind`")
		}
	case "volume":
		if m.VolumeName.IsNull() {
			return fmt.Errorf("`volume_name` is required when `type` is `volume`")
		}
	case "file":
		if m.Content.IsNull() || m.FilePath.IsNull() {
			return fmt.Errorf("`content` and `file_path` are both required when `type` is `file`")
		}
	}
	return nil
}

func (r *mountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateSubtype(plan); err != nil {
		resp.Diagnostics.AddError("Invalid mount configuration", err.Error())
		return
	}
	created, err := r.client.CreateMount(ctx, createRequest(plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating mount", err.Error())
		return
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	m, err := r.client.GetMount(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading mount", err.Error())
		return
	}
	flatten(m, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateSubtype(plan); err != nil {
		resp.Diagnostics.AddError("Invalid mount configuration", err.Error())
		return
	}
	if err := r.client.UpdateMount(ctx, updateRequest(plan)); err != nil {
		resp.Diagnostics.AddError("Updating mount", err.Error())
		return
	}
	m, err := r.client.GetMount(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading mount after update", err.Error())
		return
	}
	flatten(m, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMount(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting mount", err.Error())
	}
}

// ImportState takes the mount id alone. service_id and service_type are
// recovered from the record itself rather than from the import string:
// mounts.one reports both, so requiring the user to restate them would only
// create a way to get them wrong.
func (r *mountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
