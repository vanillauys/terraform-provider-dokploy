// Package userpermissions holds the dokploy_user_permissions resource.
package userpermissions

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*permissionsResource)(nil)
	_ resource.ResourceWithConfigure   = (*permissionsResource)(nil)
	_ resource.ResourceWithImportState = (*permissionsResource)(nil)
)

type permissionsResource struct{ client *client.Client }

func NewResource() resource.Resource { return &permissionsResource{} }

type resourceModel struct {
	ID                      types.String `tfsdk:"id"`
	UserID                  types.String `tfsdk:"user_id"`
	MemberID                types.String `tfsdk:"member_id"`
	AccessedProjects        types.Set    `tfsdk:"accessed_projects"`
	AccessedEnvironments    types.Set    `tfsdk:"accessed_environments"`
	AccessedServices        types.Set    `tfsdk:"accessed_services"`
	AccessedServers         types.Set    `tfsdk:"accessed_servers"`
	AccessedGitProviders    types.Set    `tfsdk:"accessed_git_providers"`
	CanAccessToAPI          types.Bool   `tfsdk:"can_access_api"`
	CanAccessToDocker       types.Bool   `tfsdk:"can_access_docker"`
	CanAccessToGitProviders types.Bool   `tfsdk:"can_access_git_providers"`
	CanAccessToSSHKeys      types.Bool   `tfsdk:"can_access_ssh_keys"`
	CanAccessToTraefikFiles types.Bool   `tfsdk:"can_access_traefik_files"`
	CanCreateEnvironments   types.Bool   `tfsdk:"can_create_environments"`
	CanCreateProjects       types.Bool   `tfsdk:"can_create_projects"`
	CanCreateServices       types.Bool   `tfsdk:"can_create_services"`
	CanDeleteEnvironments   types.Bool   `tfsdk:"can_delete_environments"`
	CanDeleteProjects       types.Bool   `tfsdk:"can_delete_projects"`
	CanDeleteServices       types.Bool   `tfsdk:"can_delete_services"`
}

func (r *permissionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_permissions"
}

func flag(description string) schema.Attribute {
	return schema.BoolAttribute{
		Optional: true, Computed: true, Default: booldefault.StaticBool(false),
		Description: description + " Defaults to `false`.",
	}
}

func idSet(description string) schema.Attribute {
	return schema.SetAttribute{
		Optional: true, Computed: true, ElementType: types.StringType,
		Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
		Description: description + " Defaults to an empty set.",
	}
}

func (r *permissionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The permissions of one member of the active organization: what a user with the `member` role can " +
			"see and do. An admin or the owner has every permission, and Dokploy ignores these flags for them.\n\n" +
			"~> **Destroying this resource resets every permission to the Dokploy default: no access.** The user " +
			"account stays.\n\n" +
			"~> The API key's user must be the owner of the organization; Dokploy refuses the call otherwise.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Same as `user_id`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the user: `dokploy_user.id`, or the `id` of the `dokploy_user` data source for a user that was invited in the UI.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"member_id": schema.StringAttribute{
				Computed:      true,
				Description:   "Id of the membership record.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"accessed_projects":        idSet("Ids of the projects that the user can open."),
			"accessed_environments":    idSet("Ids of the environments that the user can open."),
			"accessed_services":        idSet("Ids of the services (applications, composes, databases) that the user can open."),
			"accessed_servers":         idSet("Ids of the remote servers that the user can use."),
			"accessed_git_providers":   idSet("Ids of the git providers that the user can use."),
			"can_access_api":           flag("Let the user generate API keys and read the API docs."),
			"can_access_docker":        flag("Let the user open the Docker container views."),
			"can_access_git_providers": flag("Let the user manage git providers."),
			"can_access_ssh_keys":      flag("Let the user manage SSH keys."),
			"can_access_traefik_files": flag("Let the user edit the Traefik configuration files."),
			"can_create_environments":  flag("Let the user create environments."),
			"can_create_projects":      flag("Let the user create projects."),
			"can_create_services":      flag("Let the user create services."),
			"can_delete_environments":  flag("Let the user delete environments."),
			"can_delete_projects":      flag("Let the user delete projects."),
			"can_delete_services":      flag("Let the user delete services."),
		},
	}
}

func (r *permissionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// setValue maps a server array onto the set; the server stores [] for
// "none", and the attribute's default is the empty set, so [] stays [].
func setValue(ctx context.Context, ids []string, diags *diag.Diagnostics) types.Set {
	if ids == nil {
		ids = []string{}
	}
	set, d := types.SetValueFrom(ctx, types.StringType, ids)
	diags.Append(d...)
	return set
}

func setRequest(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	ids := []string{}
	if set.IsNull() || set.IsUnknown() {
		return ids
	}
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	return ids
}

func flatten(ctx context.Context, m *resourceModel, mem *client.Member, diags *diag.Diagnostics) {
	m.ID = types.StringValue(mem.UserID)
	m.UserID = types.StringValue(mem.UserID)
	m.MemberID = types.StringValue(mem.ID)
	m.AccessedProjects = setValue(ctx, mem.AccessedProjects, diags)
	m.AccessedEnvironments = setValue(ctx, mem.AccessedEnvironments, diags)
	m.AccessedServices = setValue(ctx, mem.AccessedServices, diags)
	m.AccessedServers = setValue(ctx, mem.AccessedServers, diags)
	m.AccessedGitProviders = setValue(ctx, mem.AccessedGitProviders, diags)
	m.CanAccessToAPI = types.BoolValue(mem.CanAccessToAPI)
	m.CanAccessToDocker = types.BoolValue(mem.CanAccessToDocker)
	m.CanAccessToGitProviders = types.BoolValue(mem.CanAccessToGitProviders)
	m.CanAccessToSSHKeys = types.BoolValue(mem.CanAccessToSSHKeys)
	m.CanAccessToTraefikFiles = types.BoolValue(mem.CanAccessToTraefikFiles)
	m.CanCreateEnvironments = types.BoolValue(mem.CanCreateEnvironments)
	m.CanCreateProjects = types.BoolValue(mem.CanCreateProjects)
	m.CanCreateServices = types.BoolValue(mem.CanCreateServices)
	m.CanDeleteEnvironments = types.BoolValue(mem.CanDeleteEnvironments)
	m.CanDeleteProjects = types.BoolValue(mem.CanDeleteProjects)
	m.CanDeleteServices = types.BoolValue(mem.CanDeleteServices)
}

func request(ctx context.Context, m resourceModel, diags *diag.Diagnostics) client.AssignPermissionsRequest {
	return client.AssignPermissionsRequest{
		ID:                      m.UserID.ValueString(),
		AccessedProjects:        setRequest(ctx, m.AccessedProjects, diags),
		AccessedEnvironments:    setRequest(ctx, m.AccessedEnvironments, diags),
		AccessedServices:        setRequest(ctx, m.AccessedServices, diags),
		AccessedServers:         setRequest(ctx, m.AccessedServers, diags),
		AccessedGitProviders:    setRequest(ctx, m.AccessedGitProviders, diags),
		CanAccessToAPI:          m.CanAccessToAPI.ValueBool(),
		CanAccessToDocker:       m.CanAccessToDocker.ValueBool(),
		CanAccessToGitProviders: m.CanAccessToGitProviders.ValueBool(),
		CanAccessToSSHKeys:      m.CanAccessToSSHKeys.ValueBool(),
		CanAccessToTraefikFiles: m.CanAccessToTraefikFiles.ValueBool(),
		CanCreateEnvironments:   m.CanCreateEnvironments.ValueBool(),
		CanCreateProjects:       m.CanCreateProjects.ValueBool(),
		CanCreateServices:       m.CanCreateServices.ValueBool(),
		CanDeleteEnvironments:   m.CanDeleteEnvironments.ValueBool(),
		CanDeleteProjects:       m.CanDeleteProjects.ValueBool(),
		CanDeleteServices:       m.CanDeleteServices.ValueBool(),
	}
}

// apply sends the full permission set and reads the member back.
func (r *permissionsResource) apply(ctx context.Context, m *resourceModel, diags *diag.Diagnostics) {
	req := request(ctx, *m, diags)
	if diags.HasError() {
		return
	}
	if err := r.client.AssignPermissions(ctx, req); err != nil {
		diags.AddError("Assigning user permissions", err.Error())
		return
	}
	mem, err := r.client.GetMember(ctx, m.UserID.ValueString())
	if err != nil {
		diags.AddError("Reading user permissions after write", err.Error())
		return
	}
	flatten(ctx, m, mem, diags)
}

func (r *permissionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *permissionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	mem, err := r.client.GetMember(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading user permissions", err.Error())
		return
	}
	flatten(ctx, &state, mem, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *permissionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete resets every permission to the Dokploy default, which is no
// access; the account itself stays. A user that is already gone is fine.
func (r *permissionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	reset := client.AssignPermissionsRequest{
		ID:               state.UserID.ValueString(),
		AccessedProjects: []string{}, AccessedEnvironments: []string{}, AccessedServices: []string{},
		AccessedServers: []string{}, AccessedGitProviders: []string{},
	}
	if err := r.client.AssignPermissions(ctx, reset); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Resetting user permissions", err.Error())
	}
}

// ImportState takes the user id.
func (r *permissionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), req.ID)...)
}
