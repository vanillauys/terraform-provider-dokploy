// Package bitbucketprovider holds the dokploy_bitbucket_provider resource.
package bitbucketprovider

import (
	"context"
	"errors"

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
	_ resource.Resource                   = (*bitbucketResource)(nil)
	_ resource.ResourceWithConfigure      = (*bitbucketResource)(nil)
	_ resource.ResourceWithImportState    = (*bitbucketResource)(nil)
	_ resource.ResourceWithValidateConfig = (*bitbucketResource)(nil)
)

type bitbucketResource struct{ client *client.Client }

func NewResource() resource.Resource { return &bitbucketResource{} }

type resourceModel struct {
	ID                   types.String `tfsdk:"id"`
	GitProviderID        types.String `tfsdk:"git_provider_id"`
	Name                 types.String `tfsdk:"name"`
	Username             types.String `tfsdk:"username"`
	AppPassword          types.String `tfsdk:"app_password"`
	AppPasswordWo        types.String `tfsdk:"app_password_wo"`
	AppPasswordWoVersion types.Int64  `tfsdk:"app_password_wo_version"`
	Email                types.String `tfsdk:"email"`
	APIToken             types.String `tfsdk:"api_token"`
	APITokenWo           types.String `tfsdk:"api_token_wo"`
	APITokenWoVersion    types.Int64  `tfsdk:"api_token_wo_version"`
	WorkspaceName        types.String `tfsdk:"workspace_name"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

var secretNames = []string{"app_password", "api_token"}

func (r *bitbucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_provider"
}

func (r *bitbucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "The `bitbucketId`. The `bitbucket.bitbucket_id` of an application or a compose references it.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"git_provider_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the generic git-provider record that owns this Bitbucket record.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"username": schema.StringAttribute{
			Optional:    true,
			Description: "Bitbucket username, for the app-password shape. Set `username` with `app_password` (or `app_password_wo`), or `email` with `api_token` (or `api_token_wo`).",
		},
		"app_password": schema.StringAttribute{
			Optional: true, Sensitive: true,
			Description: "Bitbucket app password that belongs to `username`. Atlassian has deprecated app passwords; prefer `email` with `api_token`.",
		},
		"email": schema.StringAttribute{
			Optional: true,
			Description: "Atlassian account email, for the API-token shape. Dokploy validates it as an address and cannot clear " +
				"it, so a change replaces the resource.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"api_token": schema.StringAttribute{
			Optional: true, Sensitive: true,
			Description: "Atlassian API token that belongs to `email`.",
		},
		"workspace_name": schema.StringAttribute{
			Optional:    true,
			Description: "Bitbucket workspace whose repositories Dokploy lists. If you remove it from the configuration, the provider clears it.",
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for _, name := range secretNames {
		for k, v := range tfutil.WriteOnlyCompanions(name, tfutil.WriteOnlyOptions{}) {
			attrs[k] = v
		}
	}
	resp.Schema = schema.Schema{
		Description: "A Bitbucket connection (Git > Bitbucket). Applications and composes then use the `bitbucket` " +
			"source block with this record's `id`. Two credential shapes exist: `username` with an app password, which " +
			"Atlassian has deprecated, or `email` with an API token. Set exactly one shape.\n\n" +
			"~> **Dokploy stores and returns `app_password` and `api_token` in cleartext.** Both attributes are sensitive, " +
			"so Terraform does not print them, but anyone with API access to the server can read them. The `_wo` " +
			"companions keep them out of the Terraform state.\n\n" +
			"~> Dokploy does not test the credentials on create or update. A wrong token applies successfully and fails " +
			"when Dokploy lists repositories.",
		Attributes: attrs,
	}
}

// ValidateConfig enforces the two shapes: a username needs an app password
// and an email needs an API token, and exactly one shape is set. The check
// reads the config so that a write-only value counts.
func (r *bitbucketResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg resourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hasUser := !cfg.Username.IsNull()
	hasAppPassword := !cfg.AppPassword.IsNull() || !cfg.AppPasswordWo.IsNull()
	hasEmail := !cfg.Email.IsNull()
	hasToken := !cfg.APIToken.IsNull() || !cfg.APITokenWo.IsNull()
	switch {
	case hasUser && hasEmail:
		resp.Diagnostics.AddError("Invalid Bitbucket credentials", "Set either `username` with an app password or `email` with an API token, not both.")
	case hasUser && !hasAppPassword:
		resp.Diagnostics.AddAttributeError(path.Root("app_password"), "Missing app password", "`username` needs `app_password` or `app_password_wo`.")
	case hasEmail && !hasToken:
		resp.Diagnostics.AddAttributeError(path.Root("api_token"), "Missing API token", "`email` needs `api_token` or `api_token_wo`.")
	case !hasUser && !hasEmail:
		if cfg.Username.IsUnknown() || cfg.Email.IsUnknown() {
			return
		}
		resp.Diagnostics.AddError("Missing Bitbucket credentials", "Set `username` with an app password, or `email` with an API token.")
	}
}

func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["app_password"] {
		m.AppPassword = types.StringNull()
	}
	if inUse["api_token"] {
		m.APIToken = types.StringNull()
	}
}

func (r *bitbucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

// flatten collapses each unused credential field to null: the server stores
// "" or null for the shape not in use, and the configuration has null.
func flatten(b *client.BitbucketProvider, m *resourceModel) {
	m.ID = types.StringValue(b.BitbucketID)
	m.GitProviderID = types.StringValue(b.GitProviderID)
	m.Name = types.StringValue(b.GitProvider.Name)
	m.Username = tfutil.StringOrNull(&b.BitbucketUsername)
	m.AppPassword = tfutil.StringOrNull(&b.AppPassword)
	m.Email = tfutil.StringOrNull(&b.BitbucketEmail)
	m.APIToken = tfutil.StringOrNull(&b.APIToken)
	m.WorkspaceName = tfutil.StringOrNull(&b.BitbucketWorkspaceName)
	m.CreatedAt = types.StringValue(b.GitProvider.CreatedAt)
}

func emailRequest(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func (r *bitbucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"app_password": !cfg.AppPasswordWo.IsNull(), "api_token": !cfg.APITokenWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	member, err := r.client.GetCurrentMember(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading the current user", err.Error())
		return
	}
	created, err := r.client.CreateBitbucket(ctx, client.CreateBitbucketRequest{
		AuthID:                 member.UserID,
		Name:                   plan.Name.ValueString(),
		BitbucketUsername:      plan.Username.ValueString(),
		AppPassword:            tfutil.SecretToCreate(plan.AppPassword, cfg.AppPasswordWo),
		BitbucketEmail:         emailRequest(plan.Email),
		APIToken:               tfutil.SecretToCreate(plan.APIToken, cfg.APITokenWo),
		BitbucketWorkspaceName: plan.WorkspaceName.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating Bitbucket provider", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bitbucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetBitbucket(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Bitbucket provider", err.Error())
		return
	}
	flatten(b, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// secretForUpdate returns the value the full-body update sends for one
// credential: the new value when the plan or a changed version carries
// one, the stored value when a companion is in use with nothing new, and
// "" when the credential belongs to the shape not in use.
func secretForUpdate(plain, wo, prior types.String, version, priorVersion types.Int64, stored string) string {
	value, send := tfutil.SecretToUpdate(plain, wo, prior, version, priorVersion)
	switch {
	case send:
		return value
	case !wo.IsNull():
		return stored
	default:
		return ""
	}
}

func (r *bitbucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"app_password": !cfg.AppPasswordWo.IsNull(), "api_token": !cfg.APITokenWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	current, err := r.client.GetBitbucket(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading Bitbucket provider before update", err.Error())
		return
	}
	if err := r.client.UpdateBitbucket(ctx, client.UpdateBitbucketRequest{
		BitbucketID:            plan.ID.ValueString(),
		GitProviderID:          state.GitProviderID.ValueString(),
		Name:                   plan.Name.ValueString(),
		BitbucketUsername:      plan.Username.ValueString(),
		AppPassword:            secretForUpdate(plan.AppPassword, cfg.AppPasswordWo, state.AppPassword, plan.AppPasswordWoVersion, state.AppPasswordWoVersion, current.AppPassword),
		BitbucketEmail:         emailRequest(plan.Email),
		APIToken:               secretForUpdate(plan.APIToken, cfg.APITokenWo, state.APIToken, plan.APITokenWoVersion, state.APITokenWoVersion, current.APIToken),
		BitbucketWorkspaceName: plan.WorkspaceName.ValueString(),
	}); err != nil {
		resp.Diagnostics.AddError("Updating Bitbucket provider", err.Error())
		return
	}
	b, err := r.client.GetBitbucket(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading Bitbucket provider after update", err.Error())
		return
	}
	flatten(b, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *bitbucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveGitProvider(ctx, state.GitProviderID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting Bitbucket provider", err.Error())
	}
}

func (r *bitbucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
