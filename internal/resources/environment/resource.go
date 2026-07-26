package environment

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*environmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*environmentResource)(nil)
	_ resource.ResourceWithImportState = (*environmentResource)(nil)
)

type environmentResource struct {
	client *client.Client
}

func NewResource() resource.Resource { return &environmentResource{} }

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An environment inside a Dokploy project. Services (`dokploy_application`, `dokploy_postgres`) belong to an environment rather than directly to a project. Dokploy creates a `production` environment with every project, which cannot be deleted — see `is_default`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Environment id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the project this environment belongs to. Changing it replaces the environment: Dokploy has no endpoint that moves one between projects.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Environment name. Dokploy does not enforce uniqueness within a project, so two environments may share a name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Free-form description. Dokploy stores a cleared description as an empty string rather than null; the provider reports both as null.",
			},
			"env": schema.StringAttribute{
				Optional:    true,
				Description: "Environment-level variables shared by every service in this environment, as `KEY=value` lines. Dokploy's create endpoint ignores this field, so setting it on a new environment costs one extra API call.",
			},
			// is_default is server-assigned and immutable: Dokploy exposes no
			// endpoint that promotes or demotes an environment, so pinning the
			// prior value into the plan is safe and keeps it out of the
			// framework's MarkComputedNilsAsUnknown sweep (see the package
			// comment on internal/tfutil). This is the created_at treatment,
			// NOT the `status` one — status is genuinely mutable during an
			// apply, which is why pinning it caused "inconsistent result after
			// apply" on dokploy_application.
			"is_default": schema.BoolAttribute{
				Computed:      true,
				Description:   "True for the `production` environment Dokploy creates with each project. Dokploy refuses to delete that environment, so destroying a resource with `is_default = true` fails with an explanatory error.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateEnvironment(ctx, client.CreateEnvironmentRequest{
		Name:        plan.Name.ValueString(),
		ProjectID:   plan.ProjectID.ValueString(),
		Description: NullToEmpty(plan.Description),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating environment", err.Error())
		return
	}
	plan.ID = types.StringValue(created.EnvironmentID)

	// environment.create accepts an `env` key and silently discards it (see
	// internal/client/doc.go), so env needs a follow-up update. Without this
	// the value would vanish on the first apply and reappear as a diff on the
	// next plan.
	if env := NullToEmpty(plan.Env); env != "" {
		if err := r.client.UpdateEnvironment(ctx, client.UpdateEnvironmentRequest{
			EnvironmentID: created.EnvironmentID,
			Name:          plan.Name.ValueString(),
			Description:   NullToEmpty(plan.Description),
			Env:           env,
		}); err != nil {
			// The environment exists; record its id so the next apply
			// converges instead of orphaning it.
			plan.IsDefault = types.BoolValue(created.IsDefault)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			resp.Diagnostics.AddError("Setting environment variables after create",
				fmt.Sprintf("environment %s was created, but setting `env` failed: %s. The next apply will converge.", created.EnvironmentID, err))
			return
		}
	}

	current, err := r.client.GetEnvironment(ctx, created.EnvironmentID)
	if err != nil {
		plan.IsDefault = types.BoolValue(created.IsDefault)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Reading environment after create",
			fmt.Sprintf("environment %s was created, but reading it back failed: %s. The next apply will converge.", created.EnvironmentID, err))
		return
	}
	resp.Diagnostics.Append(setComputed(current, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client.GetEnvironment(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning("Environment not found",
			fmt.Sprintf("environment %s no longer exists; removing it from state", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading environment", err.Error())
		return
	}
	resp.Diagnostics.Append(flatten(ctx, e, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Dialect C: every field goes on every call. An absent key silently keeps
	// the stored value, so omitting one would make it unclearable.
	err := r.client.UpdateEnvironment(ctx, client.UpdateEnvironmentRequest{
		EnvironmentID: plan.ID.ValueString(),
		Name:          plan.Name.ValueString(),
		Description:   NullToEmpty(plan.Description),
		Env:           NullToEmpty(plan.Env),
	})
	if err != nil {
		resp.Diagnostics.AddError("Updating environment", err.Error())
		return
	}
	current, err := r.client.GetEnvironment(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading environment after update", err.Error())
		return
	}
	resp.Diagnostics.Append(setComputed(current, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if reason := deleteBlockedReason(&state); reason != "" {
		resp.Diagnostics.AddError("Cannot delete the default environment", reason)
		return
	}
	err := r.client.DeleteEnvironment(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting environment", err.Error())
	}
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
