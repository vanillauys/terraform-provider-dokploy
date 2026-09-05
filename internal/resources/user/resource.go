// Package user holds the dokploy_user resource.
package user

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*userResource)(nil)
	_ resource.ResourceWithConfigure   = (*userResource)(nil)
	_ resource.ResourceWithImportState = (*userResource)(nil)
)

type userResource struct{ client *client.Client }

func NewResource() resource.Resource { return &userResource{} }

type resourceModel struct {
	ID                types.String `tfsdk:"id"`
	MemberID          types.String `tfsdk:"member_id"`
	Email             types.String `tfsdk:"email"`
	Password          types.String `tfsdk:"password"`
	PasswordWo        types.String `tfsdk:"password_wo"`
	PasswordWoVersion types.Int64  `tfsdk:"password_wo_version"`
	Role              types.String `tfsdk:"role"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

var secretNames = []string{"password"}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "User id. `dokploy_user_permissions.user_id` references it.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"member_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the membership record in the active organization.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"email": schema.StringAttribute{
			Required:      true,
			Description:   "Login email. Dokploy refuses an email that already has an account. A change replaces the resource.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"password": schema.StringAttribute{
			Optional: true, Sensitive: true,
			Description: "Initial password, 8 characters or more. Set this attribute or `password_wo`. Dokploy has no " +
				"endpoint that resets another user's password, so a change replaces the resource.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"role": schema.StringAttribute{
			Required: true,
			Description: "Member role in the active organization: `member`, `admin`, or the name of a custom role. " +
				"`owner` is not allowed.",
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp of the membership from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("password", tfutil.WriteOnlyOptions{
		ExactlyOne: true,
		Effect:     "A version change replaces the resource, because Dokploy cannot reset another user's password.",
	}) {
		if name == "password_wo_version" {
			version := attr.(schema.Int64Attribute)
			version.PlanModifiers = []planmodifier.Int64{int64planmodifier.RequiresReplace()}
			attr = version
		}
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "A user account with an initial password, added to the API key's active organization with a member " +
			"role. The user signs in with the email and the password and can change the password afterwards.\n\n" +
			"~> **Dokploy never returns the password, and cannot reset it for another user.** The state keeps the value " +
			"that Terraform sent, or nothing with `password_wo`. A change to `password`, `password_wo_version`, or " +
			"`email` replaces the resource, which deletes the account and creates a new one. After a `terraform import`, " +
			"add `lifecycle { ignore_changes = [password] }` or accept the replacement.\n\n" +
			"~> This resource needs self-hosted Dokploy. Dokploy Cloud refuses `user.createUserWithCredentials`.",
		Attributes: attrs,
	}
}

func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["password"] {
		m.Password = types.StringNull()
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// flatten leaves m.Password alone: the server never returns it.
func flatten(mem *client.Member, m *resourceModel) {
	m.ID = types.StringValue(mem.UserID)
	m.MemberID = types.StringValue(mem.ID)
	m.Email = types.StringValue(mem.User.Email)
	m.Role = types.StringValue(mem.Role)
	m.CreatedAt = types.StringValue(mem.CreatedAt)
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"password": !cfg.PasswordWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	created, err := r.client.CreateUser(ctx, client.CreateUserRequest{
		Email:    plan.Email.ValueString(),
		Password: tfutil.SecretToCreate(plan.Password, cfg.PasswordWo),
		Role:     plan.Role.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating user", err.Error())
		return
	}
	mem, err := r.client.GetMember(ctx, created.UserID)
	if err != nil {
		resp.Diagnostics.AddError("Reading user after create", err.Error())
		return
	}
	flatten(mem, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	mem, err := r.client.GetMember(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading user", err.Error())
		return
	}
	flatten(mem, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes the member role only: everything else is RequiresReplace.
func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"password": !cfg.PasswordWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	if !plan.Role.Equal(state.Role) {
		if err := r.client.UpdateMemberRole(ctx, state.MemberID.ValueString(), plan.Role.ValueString()); err != nil {
			resp.Diagnostics.AddError("Updating user role", err.Error())
			return
		}
	}
	mem, err := r.client.GetMember(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading user after update", err.Error())
		return
	}
	flatten(mem, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUser(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting user", err.Error())
	}
}

// ImportState takes the user id.
func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
