// Package gitlabprovider holds the dokploy_gitlab_provider resource.
package gitlabprovider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ resource.Resource                = (*gitlabResource)(nil)
	_ resource.ResourceWithConfigure   = (*gitlabResource)(nil)
	_ resource.ResourceWithImportState = (*gitlabResource)(nil)
)

type gitlabResource struct{ client *client.Client }

func NewResource() resource.Resource { return &gitlabResource{} }

type resourceModel struct {
	ID                types.String `tfsdk:"id"`
	GitProviderID     types.String `tfsdk:"git_provider_id"`
	Name              types.String `tfsdk:"name"`
	GitlabURL         types.String `tfsdk:"gitlab_url"`
	GitlabInternalURL types.String `tfsdk:"gitlab_internal_url"`
	ApplicationID     types.String `tfsdk:"application_id"`
	Secret            types.String `tfsdk:"secret"`
	SecretWo          types.String `tfsdk:"secret_wo"`
	SecretWoVersion   types.Int64  `tfsdk:"secret_wo_version"`
	GroupName         types.String `tfsdk:"group_name"`
	RedirectURI       types.String `tfsdk:"redirect_uri"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

var secretNames = []string{"secret"}

func (r *gitlabResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gitlab_provider"
}

func (r *gitlabResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "The `gitlabId`. The `gitlab.gitlab_id` of an application or a compose references it.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"git_provider_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the generic git-provider record that owns this GitLab record.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"gitlab_url": schema.StringAttribute{
			Optional: true, Computed: true, Default: stringdefault.StaticString("https://gitlab.com"),
			Description: "URL of the GitLab instance. Defaults to `https://gitlab.com`.",
		},
		"gitlab_internal_url": schema.StringAttribute{
			Optional: true,
			Description: "URL that the Dokploy server uses to reach a self-hosted GitLab when it differs from `gitlab_url`, " +
				"for example an address on a private network. If you remove it from the configuration, the provider clears it.",
		},
		"application_id": schema.StringAttribute{Required: true, Description: "Application ID of the OAuth application that you created in GitLab."},
		"secret": schema.StringAttribute{
			Optional: true, Sensitive: true,
			Description: "Secret of the OAuth application. Set this attribute or `secret_wo`.",
		},
		"group_name": schema.StringAttribute{
			Optional: true,
			Description: "GitLab group whose projects Dokploy lists. Omit it to list the projects of the authorizing user. " +
				"If you remove it from the configuration, the provider clears it.",
		},
		"redirect_uri": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: "Redirect URI of the OAuth application. Defaults to `<endpoint>/api/providers/gitlab/callback`, " +
				"built from the provider's `endpoint`. Register the same URI in GitLab.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("secret", tfutil.WriteOnlyOptions{ExactlyOne: true}) {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "A GitLab connection (Git > GitLab): the OAuth application that lets Dokploy list and clone " +
			"repositories. Applications and composes then use the `gitlab` source block with this record's `id`.\n\n" +
			"~> **A person must authorize the application once in a browser.** Terraform stores the OAuth application " +
			"credentials. Until someone opens Git > GitLab in the Dokploy UI and completes the authorization, Dokploy " +
			"holds no access token, the `dokploy_gitlab_provider` data source reports `is_configured = false`, and a " +
			"deploy from this provider fails.\n\n" +
			"~> **Dokploy stores and returns `secret` in cleartext.** The attribute is sensitive, so Terraform does not " +
			"print it, but anyone with API access to the server can read it. The `secret_wo` companion keeps it out of " +
			"the Terraform state.",
		Attributes: attrs,
	}
}

func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["secret"] {
		m.Secret = types.StringNull()
	}
}

func (r *gitlabResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(g *client.GitlabProvider, m *resourceModel) {
	m.ID = types.StringValue(g.GitlabID)
	m.GitProviderID = types.StringValue(g.GitProviderID)
	m.Name = types.StringValue(g.GitProvider.Name)
	m.GitlabURL = types.StringValue(g.GitlabURL)
	m.GitlabInternalURL = tfutil.StringOrNull(&g.GitlabInternalURL)
	m.ApplicationID = types.StringValue(g.ApplicationID)
	m.Secret = types.StringValue(g.Secret)
	m.GroupName = tfutil.StringOrNull(&g.GroupName)
	m.RedirectURI = types.StringValue(g.RedirectURI)
	m.CreatedAt = types.StringValue(g.GitProvider.CreatedAt)
}

// redirectURI returns the configured value, or the callback that the
// Dokploy UI would register for this endpoint.
func (r *gitlabResource) redirectURI(v types.String) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return r.client.Endpoint() + "/api/providers/gitlab/callback"
}

func (r *gitlabResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"secret": !cfg.SecretWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	member, err := r.client.GetCurrentMember(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Reading the current user", err.Error())
		return
	}
	created, err := r.client.CreateGitlab(ctx, client.CreateGitlabRequest{
		AuthID:            member.UserID,
		Name:              plan.Name.ValueString(),
		GitlabURL:         plan.GitlabURL.ValueString(),
		ApplicationID:     plan.ApplicationID.ValueString(),
		Secret:            tfutil.SecretToCreate(plan.Secret, cfg.SecretWo),
		GroupName:         plan.GroupName.ValueString(),
		RedirectURI:       r.redirectURI(plan.RedirectURI),
		GitlabInternalURL: plan.GitlabInternalURL.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating GitLab provider", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *gitlabResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetGitlab(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading GitLab provider", err.Error())
		return
	}
	flatten(g, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gitlabResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"secret": !cfg.SecretWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	secret, send := tfutil.SecretToUpdate(plan.Secret, cfg.SecretWo, state.Secret, plan.SecretWoVersion, state.SecretWoVersion)
	if !send {
		// gitlab.update rejects a null string and keeps an absent one, so
		// the full body resends the stored secret, which gitlab.one returns.
		current, err := r.client.GetGitlab(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading GitLab provider before update", err.Error())
			return
		}
		secret = current.Secret
	}
	if err := r.client.UpdateGitlab(ctx, client.UpdateGitlabRequest{
		GitlabID:          plan.ID.ValueString(),
		GitProviderID:     state.GitProviderID.ValueString(),
		Name:              plan.Name.ValueString(),
		GitlabURL:         plan.GitlabURL.ValueString(),
		ApplicationID:     plan.ApplicationID.ValueString(),
		Secret:            secret,
		GroupName:         plan.GroupName.ValueString(),
		RedirectURI:       r.redirectURI(plan.RedirectURI),
		GitlabInternalURL: plan.GitlabInternalURL.ValueStringPointer(),
	}); err != nil {
		resp.Diagnostics.AddError("Updating GitLab provider", err.Error())
		return
	}
	g, err := r.client.GetGitlab(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading GitLab provider after update", err.Error())
		return
	}
	flatten(g, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *gitlabResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveGitProvider(ctx, state.GitProviderID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting GitLab provider", err.Error())
	}
}

func (r *gitlabResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
