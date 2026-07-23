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
		Description: "A Dokploy project. Dokploy creates a default `production` environment with every project; service resources reference its id via the `environments` attribute.",
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
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp (server-side).",
			},
			"environments": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Environments in this project, including the auto-created `production` environment.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, Description: "Environment id."},
						"name": schema.StringAttribute{Computed: true, Description: "Environment name."},
					},
				},
			},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
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
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating project", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ProjectID)

	current, err := r.client.GetProject(ctx, created.ProjectID)
	if err != nil {
		// Spec §5.4: record the created id in state and return the error;
		// the next apply converges. Unknown computed fields become null.
		plan.CreatedAt = types.StringNull()
		plan.Environments = types.ListNull(EnvironmentObjectType)
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
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.UpdateProject(ctx, client.UpdateProjectRequest{
		ProjectID:   plan.ID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Updating project", err.Error())
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
