// Package webserverbackup holds dokploy_web_server_backup: a scheduled backup
// of the Dokploy host to an S3-compatible destination.
package webserverbackup

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*webServerBackupResource)(nil)
	_ resource.ResourceWithConfigure   = (*webServerBackupResource)(nil)
	_ resource.ResourceWithImportState = (*webServerBackupResource)(nil)
)

type webServerBackupResource struct{ client *client.Client }

func NewResource() resource.Resource { return &webServerBackupResource{} }

func (r *webServerBackupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_web_server_backup"
}

func (r *webServerBackupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A scheduled backup of the Dokploy host. Dokploy dumps its own database and its " +
			"configuration directory to an S3-compatible destination.\n\n" +
			"~> Dokploy lets more than one web-server backup exist. Each resource manages one backup. " +
			"The provider does not run a backup on demand.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Backup id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"destination_id": schema.StringAttribute{
				Required:    true,
				Description: "Id of the `dokploy_destination` that receives the backups.",
			},
			"cron_expression": schema.StringAttribute{
				Required:    true,
				Description: "Standard five-field cron expression, for example `0 3 * * *`.",
			},
			"prefix": schema.StringAttribute{
				Required:    true,
				Description: "Key prefix inside the destination bucket, for example `dokploy/`.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Whether the backup runs. Defaults to `true`. Dokploy leaves this field null for a " +
					"record from the API alone, which is neither on nor off.",
			},
			"include_encryption_key": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Include the encryption key in the backup. Defaults to `true`, the value that " +
					"Dokploy stores for a new backup. The provider always sends the field, because the " +
					"Dokploy update endpoint stores `false` for an omitted field.",
			},
			"keep_latest_count": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of backups to keep. Omit it to keep all of them.",
			},
		},
	}
}

func (r *webServerBackupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *webServerBackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	userID, err := r.client.CurrentUserID(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading the current user", err.Error())
		return
	}
	created, err := r.client.CreateWebServerBackup(ctx, createRequest(plan, userID))
	if err != nil {
		resp.Diagnostics.AddError("Creating web server backup", err.Error())
		return
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webServerBackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetBackup(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading web server backup", err.Error())
		return
	}
	flatten(b, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webServerBackupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateBackup(ctx, updateRequest(plan)); err != nil {
		resp.Diagnostics.AddError("Updating web server backup", err.Error())
		return
	}
	b, err := r.client.GetBackup(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading web server backup after update", err.Error())
		return
	}
	flatten(b, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webServerBackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackup(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting web server backup", err.Error())
	}
}

func (r *webServerBackupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
