// Package volumebackup holds dokploy_volume_backup: a scheduled archive of
// a Docker volume to an S3-compatible destination.
package volumebackup

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*volumeBackupResource)(nil)
	_ resource.ResourceWithConfigure   = (*volumeBackupResource)(nil)
	_ resource.ResourceWithImportState = (*volumeBackupResource)(nil)
)

type volumeBackupResource struct{ client *client.Client }

func NewResource() resource.Resource { return &volumeBackupResource{} }

func (r *volumeBackupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume_backup"
}

func (r *volumeBackupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A scheduled archive of a Docker volume to an S3-compatible destination.\n\n" +
			"A volume backup copies the contents of the volume as they are, so it works for any service with a volume. " +
			"That includes Redis, which `dokploy_backup` cannot cover because Dokploy has no logical dump for it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Volume backup id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Display name."},
			"service_id": schema.StringAttribute{
				Required: true,
				Description: "Id of the service that owns the volume. A change forces a replacement: the Dokploy " +
					"update endpoint sets the new parent without a clear of the old one, and the record then belongs to " +
					"two services at once.",
				PlanModifiers: requiresReplace,
			},
			"service_type": schema.StringAttribute{
				Required: true,
				Description: "Kind of service that `service_id` refers to: one of `application`, `postgres`, `mysql`, " +
					"`mariadb`, `mongo`, `redis`, `compose`, `libsql`. A change forces a replacement, for the same " +
					"reason as `service_id`.",
				PlanModifiers: requiresReplace,
				Validators:    []validator.String{stringvalidator.OneOf(client.VolumeBackupServiceTypes...)},
			},
			"volume_name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the Docker volume to archive.",
			},
			"prefix": schema.StringAttribute{
				Required:    true,
				Description: "Key prefix inside the destination bucket, for example `volumes/web/`.",
			},
			"cron_expression": schema.StringAttribute{
				Required:    true,
				Description: "Standard five-field cron expression, for example `0 4 * * *`.",
			},
			"destination_id": schema.StringAttribute{
				Required:    true,
				Description: "Id of the `dokploy_destination` that receives the archives.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Whether the backup runs. Defaults to `true`. Dokploy leaves this field null for a record " +
					"from the API alone, which is neither on nor off, and a backup in the " +
					"configuration that never runs is the worse failure.",
			},
			"turn_off": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "The Dokploy `turnOff` flag, passed through unchanged. The server coerces an absent " +
					"key and an explicit null to `false`, so this attribute always sends a value.",
			},
			"keep_latest_count": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of archives to keep. Omit it to keep all of them.",
			},
			"service_name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of the specific container, for compose services with more than one.",
			},
			"app_name": schema.StringAttribute{
				Computed:      true,
				Description:   "Internal Dokploy app name. The server generates it.",
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

func (r *volumeBackupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *volumeBackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateVolumeBackup(ctx, createRequest(plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating volume backup", err.Error())
		return
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *volumeBackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := r.client.GetVolumeBackup(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading volume backup", err.Error())
		return
	}
	flatten(v, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *volumeBackupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateVolumeBackup(ctx, updateRequest(plan)); err != nil {
		resp.Diagnostics.AddError("Updating volume backup", err.Error())
		return
	}
	v, err := r.client.GetVolumeBackup(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading volume backup after update", err.Error())
		return
	}
	flatten(v, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *volumeBackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteVolumeBackup(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting volume backup", err.Error())
	}
}

func (r *volumeBackupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
