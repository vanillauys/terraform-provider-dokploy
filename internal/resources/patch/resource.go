// Package patch holds dokploy_patch: a file change that Dokploy applies to
// the repository of an application or a compose service before the build.
package patch

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                     = (*patchResource)(nil)
	_ resource.ResourceWithConfigure        = (*patchResource)(nil)
	_ resource.ResourceWithImportState      = (*patchResource)(nil)
	_ resource.ResourceWithConfigValidators = (*patchResource)(nil)
)

type patchResource struct{ client *client.Client }

func NewResource() resource.Resource { return &patchResource{} }

func (r *patchResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_patch"
}

func (r *patchResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A file change that Dokploy applies to the repository of an application or a compose " +
			"service before each build: it creates, replaces, or deletes one file. A change applies at the next deploy.\n\n" +
			"~> Each file path can have one patch for each service.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Patch id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"application_id": schema.StringAttribute{
				Optional:      true,
				Description:   "Id of the application whose repository the patch changes. Set exactly one of `application_id` or `compose_id`. A change replaces the patch.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"compose_id": schema.StringAttribute{
				Optional:      true,
				Description:   "Id of the compose service whose repository the patch changes. Set exactly one of `application_id` or `compose_id`. A change replaces the patch.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"file_path": schema.StringAttribute{
				Required:    true,
				Description: "Path of the file, relative to the repository root, for example `config/app.yaml`.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("update"),
				Description: "What the patch does to the file: `create` adds it, `update` replaces its content, and " +
					"`delete` removes it. Defaults to `update`.",
				Validators: []validator.String{stringvalidator.OneOf("create", "update", "delete")},
			},
			"content": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(""),
				Description: "Full content of the file. Defaults to `\"\"`, the value for a `delete` patch. " +
					"Dokploy adds a final newline to a non-empty content when it updates the patch.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Apply the patch at the next build. Defaults to `true`.",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp from the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// ConfigValidators: patch.create rejects a body with neither parent, and
// the record has no meaning with both.
func (r *patchResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(path.MatchRoot("application_id"), path.MatchRoot("compose_id")),
	}
}

func (r *patchResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// Create calls patch.update after patch.create for a disabled patch,
// because patch.create always stores enabled = true.
func (r *patchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.CreatePatch(ctx, createRequest(plan))
	if err != nil {
		resp.Diagnostics.AddError("Creating patch", err.Error())
		return
	}
	if !plan.Enabled.ValueBool() {
		disabled, err := r.client.UpdatePatch(ctx, updateRequest(p.PatchID, plan))
		if err != nil {
			// The patch exists: keep it in the state, so the next apply
			// retries the update instead of a second create.
			flatten(p, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Disabling patch", err.Error())
			return
		}
		p = disabled
	}
	flatten(p, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *patchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetPatch(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading patch", err.Error())
		return
	}
	flatten(p, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *patchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.UpdatePatch(ctx, updateRequest(plan.ID.ValueString(), plan))
	if err != nil {
		resp.Diagnostics.AddError("Updating patch", err.Error())
		return
	}
	flatten(p, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *patchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePatch(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting patch", err.Error())
	}
}

func (r *patchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
