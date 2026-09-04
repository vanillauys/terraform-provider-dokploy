// Package backup holds dokploy_backup: a scheduled logical dump of a
// database to an S3-compatible destination.
package backup

import (
	"context"
	"errors"
	"fmt"

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
	_ resource.Resource                = (*backupResource)(nil)
	_ resource.ResourceWithConfigure   = (*backupResource)(nil)
	_ resource.ResourceWithImportState = (*backupResource)(nil)
)

type backupResource struct{ client *client.Client }

func NewResource() resource.Resource { return &backupResource{} }

func (r *backupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup"
}

// serviceTypes are the parents this resource accepts: the database engines
// Dokploy can logically dump, plus compose.
//
// `redis` is deliberately absent — see the validator message. `web-server`
// is deliberately absent too: it backs up Dokploy's own database rather than
// a service, has no parent id, and needs its own validation path.
var serviceTypes = append([]string{"compose"}, client.BackupDatabaseTypes...)

func (r *backupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A scheduled logical dump of a database to an S3-compatible destination.\n\n" +
			"~> **This resource does not support Redis.** Dokploy has no logical dump for Redis. Use " +
			"`dokploy_volume_backup`, which archives the volume and accepts a Redis parent.\n\n" +
			"~> This resource does not expose a backup of the Dokploy server itself (the Dokploy `web-server` backup type). " +
			"That backup type has no parent service and needs a separate validation path.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Backup id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"service_id": schema.StringAttribute{
				Required: true,
				Description: "Id of the database or compose service to dump. A change forces a replacement: " +
					"the Dokploy update endpoint has no parent field, so a retarget is not possible.",
				PlanModifiers: requiresReplace,
			},
			"service_type": schema.StringAttribute{
				Required: true,
				Description: "Kind of service that `service_id` refers to: one of `postgres`, `mysql`, `mariadb`, " +
					"`mongo`, `libsql`, `compose`. A change forces a replacement. The provider derives the Dokploy `databaseType` " +
					"and `backupType` fields from this attribute. Independent values would allow a " +
					"record whose type and parent disagree, so the provider does not expose them.",
				PlanModifiers: requiresReplace,
				Validators:    []validator.String{stringvalidator.OneOf(serviceTypes...)},
			},
			"database": schema.StringAttribute{
				Required:    true,
				Description: "Name of the database to dump.",
			},
			"prefix": schema.StringAttribute{
				Required:    true,
				Description: "Key prefix inside the destination bucket, for example `backups/app/`.",
			},
			"schedule": schema.StringAttribute{
				Required:    true,
				Description: "Standard five-field cron expression, for example `0 3 * * *`.",
			},
			"destination_id": schema.StringAttribute{
				Required:    true,
				Description: "Id of the `dokploy_destination` that receives the dumps.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Whether the backup runs. Defaults to `true`. Dokploy leaves this field null for a record " +
					"from the API alone, which is neither on nor off, and a backup in the " +
					"configuration that never runs is the worse failure.",
			},
			"include_encryption_key": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Include the database encryption key in the dump. Defaults to `true`, the value that " +
					"Dokploy stores for a new backup. The provider always sends the field: the Dokploy " +
					"update endpoint stores `false` for an omitted field, so a request without it would " +
					"turn the key off on a record that had it on.",
			},
			"keep_latest_count": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of dumps to keep. Omit it to keep all of them.",
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
		},
	}
}

func (r *backupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// validateServiceType rejects Redis with an explanation and a pointer at the
// resource that does support it, rather than letting the apply fail on a zod
// enum error that says only "invalid option".
//
// The OneOf validator in the schema already rejects it at plan time; this is
// the belt-and-braces path for a value that reaches Create some other way,
// and the place the better message lives.
func validateServiceType(m resourceModel) error {
	if m.ServiceType.ValueString() == "redis" {
		return fmt.Errorf(
			"`service_type` cannot be `redis`: Dokploy has no logical dump for Redis. " +
				"Use `dokploy_volume_backup`, which snapshots the volume and does accept a Redis parent")
	}
	return nil
}

func (r *backupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateServiceType(plan); err != nil {
		resp.Diagnostics.AddError("Invalid backup configuration", err.Error())
		return
	}
	created, err := r.client.CreateBackup(ctx, parentRef(plan), createRequest(plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating backup", err.Error())
		return
	}
	flatten(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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
		resp.Diagnostics.AddError("Reading backup", err.Error())
		return
	}
	flatten(b, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *backupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateBackup(ctx, updateRequest(plan)); err != nil {
		resp.Diagnostics.AddError("Updating backup", err.Error())
		return
	}
	b, err := r.client.GetBackup(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading backup after update", err.Error())
		return
	}
	flatten(b, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *backupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteBackup(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting backup", err.Error())
	}
}

func (r *backupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
