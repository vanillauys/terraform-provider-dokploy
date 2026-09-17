package project

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*projectResource)(nil)
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
)

type projectResource struct {
	client *client.Client
}

func NewResource() resource.Resource { return &projectResource{} }

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Dokploy project. Dokploy creates a default `production` environment with each project. Service resources reference its id through `production_environment_id`. The `environments` attribute lists every environment in the project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Project id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Project name.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Free-form description.",
			},
			"env": schema.StringAttribute{
				Optional: true,
				Description: "Variables that each service in the project can reference, as `KEY=value` lines. A service reads " +
					"them through the `${{project.KEY}}` syntax; an environment does not inherit them into its own `env`. " +
					"An omitted value and `\"\"` both read back as null. Omit the attribute to clear it.",
			},
			// created_at is immutable server-side, so pinning the prior value
			// into the plan is always safe and keeps it out of the framework's
			// MarkComputedNilsAsUnknown sweep (see the package comment on
			// internal/tfutil). Deliberately NOT applied to `status` on the
			// service resources — that one is genuinely server-mutable and
			// pinning it caused "Provider produced inconsistent result after
			// apply"; see internal/resources/application/resource.go.
			"created_at": schema.StringAttribute{
				Computed:      true,
				Description:   "Creation timestamp from the server.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"environments": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Environments in this project, the auto-created `production` environment included.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, Description: "Environment id."},
						"name": schema.StringAttribute{Computed: true, Description: "Environment name."},
					},
				},
			},
			// The default environment cannot be deleted, and environment.update
			// has no isDefault field (census, v0.30.5), so the id is fixed for
			// the life of the project. Pinning the prior value keeps a project
			// update from planning "(known after apply)" here, which would
			// otherwise flow into every environment_id that references it and
			// force a replacement of those services.
			"production_environment_id": schema.StringAttribute{
				Computed:      true,
				Description:   "Id of the default environment. Dokploy creates it with the project and names it `production`. The provider selects it with the server's `isDefault` flag, not by name, so a rename does not change the value. Use it as the `environment_id` of a service in the default environment.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tag_ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Ids of the `dokploy_tag` records that the project carries. The provider sends the whole set " +
					"on each change, so the assignments in Dokploy match the set exactly. Omit the attribute to clear them.",
			},
		},
	}
}

// assignTags sends the planned tag set when it differs from the prior one.
// tag.bulkAssign replaces the assignments of the project in one call, and
// a null plan clears them (client.BulkAssignTags sends []).
func (r *projectResource) assignTags(ctx context.Context, id string, plan, prior types.Set) error {
	if plan.IsUnknown() || plan.Equal(prior) {
		return nil
	}
	var ids []string
	if !plan.IsNull() {
		if diags := plan.ElementsAs(ctx, &ids, false); diags.HasError() {
			return fmt.Errorf("reading tag_ids: %s", diags[0].Detail())
		}
	}
	return r.client.BulkAssignTags(ctx, client.BulkAssignTagsRequest{ProjectID: id, TagIDs: ids})
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateProject(ctx, client.CreateProjectRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueStringPointer(),
		Env:         plan.Env.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating project", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ProjectID)

	// project.create ignores tagIds (client.Project), so the assignment
	// is a second call. A failure here leaves the project created with no
	// tags; the id goes to state so that the next apply converges.
	if err := r.assignTags(ctx, created.ProjectID, plan.TagIDs, types.SetNull(types.StringType)); err != nil {
		plan.CreatedAt = types.StringNull()
		plan.Environments = types.ListNull(EnvironmentObjectType)
		plan.ProductionEnvironmentID = types.StringNull()
		plan.TagIDs = types.SetNull(types.StringType)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Assigning tags after create",
			fmt.Sprintf("project %s was created, but assigning its tags failed: %s. The next apply will converge.", created.ProjectID, err))
		return
	}

	current, err := r.client.GetProject(ctx, created.ProjectID)
	if err != nil {
		// Spec §5.4: record the created id in state and return the error;
		// the next apply converges. Unknown computed fields become null.
		plan.CreatedAt = types.StringNull()
		plan.Environments = types.ListNull(EnvironmentObjectType)
		plan.ProductionEnvironmentID = types.StringNull()
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Reading project after create",
			fmt.Sprintf("project %s was created, but reading it back failed: %s. The next apply will converge.", created.ProjectID, err))
		return
	}
	resp.Diagnostics.Append(setComputed(current, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetProject(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddWarning("Project not found",
			fmt.Sprintf("project %s no longer exists; removing it from state", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading project", err.Error())
		return
	}
	resp.Diagnostics.Append(flatten(ctx, p, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.UpdateProject(ctx, client.UpdateProjectRequest{
		ProjectID:   plan.ID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueStringPointer(),
		Env:         plan.Env.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Updating project", err.Error())
		return
	}
	if err := r.assignTags(ctx, plan.ID.ValueString(), plan.TagIDs, state.TagIDs); err != nil {
		resp.Diagnostics.AddError("Assigning tags", err.Error())
		return
	}
	current, err := r.client.GetProject(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading project after update", err.Error())
		return
	}
	resp.Diagnostics.Append(setComputed(current, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteProject(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting project", err.Error())
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
