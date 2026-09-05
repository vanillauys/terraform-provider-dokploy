// Package giteaprovider holds the dokploy_gitea_provider resource.
package giteaprovider

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
	_ resource.Resource                = (*giteaResource)(nil)
	_ resource.ResourceWithConfigure   = (*giteaResource)(nil)
	_ resource.ResourceWithImportState = (*giteaResource)(nil)
)

type giteaResource struct{ client *client.Client }

func NewResource() resource.Resource { return &giteaResource{} }

type resourceModel struct {
	ID                    types.String `tfsdk:"id"`
	GitProviderID         types.String `tfsdk:"git_provider_id"`
	Name                  types.String `tfsdk:"name"`
	GiteaURL              types.String `tfsdk:"gitea_url"`
	GiteaInternalURL      types.String `tfsdk:"gitea_internal_url"`
	ClientID              types.String `tfsdk:"client_id"`
	ClientSecret          types.String `tfsdk:"client_secret"`
	ClientSecretWo        types.String `tfsdk:"client_secret_wo"`
	ClientSecretWoVersion types.Int64  `tfsdk:"client_secret_wo_version"`
	RedirectURI           types.String `tfsdk:"redirect_uri"`
	Scopes                types.String `tfsdk:"scopes"`
	CreatedAt             types.String `tfsdk:"created_at"`
}

var secretNames = []string{"client_secret"}

func (r *giteaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gitea_provider"
}

func (r *giteaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			Description:   "The `giteaId`. The `gitea.gitea_id` of an application or a compose references it.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"git_provider_id": schema.StringAttribute{
			Computed:      true,
			Description:   "Id of the generic git-provider record that owns this Gitea record.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name":      schema.StringAttribute{Required: true, Description: "Display name. Dokploy does not enforce a unique name."},
		"gitea_url": schema.StringAttribute{Required: true, Description: "URL of the Gitea instance, for example `https://gitea.example.com`."},
		"gitea_internal_url": schema.StringAttribute{
			Optional: true,
			Description: "URL that the Dokploy server uses to reach Gitea when it differs from `gitea_url`, for example an " +
				"address on a private network. If you remove it from the configuration, the provider clears it.",
		},
		"client_id": schema.StringAttribute{Required: true, Description: "Client ID of the OAuth2 application that you created in Gitea."},
		"client_secret": schema.StringAttribute{
			Optional: true, Sensitive: true,
			Description: "Client secret of the OAuth2 application. Set this attribute or `client_secret_wo`.",
		},
		"redirect_uri": schema.StringAttribute{
			Optional: true, Computed: true,
			Description: "Redirect URI of the OAuth2 application. Defaults to `<endpoint>/api/providers/gitea/callback`, " +
				"built from the provider's `endpoint`. Register the same URI in Gitea.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"scopes": schema.StringAttribute{
			Optional: true, Computed: true, Default: stringdefault.StaticString(client.GiteaDefaultScopes),
			Description: "OAuth2 scopes that Dokploy requests, comma-separated. Defaults to `" + client.GiteaDefaultScopes + "`.",
		},
		"created_at": schema.StringAttribute{
			Computed:      true,
			Description:   "Creation timestamp from the server.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	}
	for name, attr := range tfutil.WriteOnlyCompanions("client_secret", tfutil.WriteOnlyOptions{ExactlyOne: true}) {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		Description: "A Gitea connection (Git > Gitea): the OAuth2 application that lets Dokploy list and clone " +
			"repositories. Applications and composes then use the `gitea` source block with this record's `id`.\n\n" +
			"~> **A person must authorize the application once in a browser.** Terraform stores the OAuth2 application " +
			"credentials. Until someone opens Git > Gitea in the Dokploy UI and completes the authorization, Dokploy " +
			"holds no access token, the `dokploy_gitea_provider` data source reports `is_configured = false`, and a " +
			"deploy from this provider fails.\n\n" +
			"~> **Dokploy stores and returns `client_secret` in cleartext.** The attribute is sensitive, so Terraform " +
			"does not print it, but anyone with API access to the server can read it. The `client_secret_wo` companion " +
			"keeps it out of the Terraform state.",
		Attributes: attrs,
	}
}

func hideWriteOnly(m *resourceModel, inUse map[string]bool) {
	if inUse["client_secret"] {
		m.ClientSecret = types.StringNull()
	}
}

func (r *giteaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.client = c
	}
}

func flatten(g *client.GiteaProvider, m *resourceModel) {
	m.ID = types.StringValue(g.GiteaID)
	m.GitProviderID = types.StringValue(g.GitProviderID)
	m.Name = types.StringValue(g.GitProvider.Name)
	m.GiteaURL = types.StringValue(g.GiteaURL)
	m.GiteaInternalURL = tfutil.StringOrNull(&g.GiteaInternalURL)
	m.ClientID = types.StringValue(g.ClientID)
	m.ClientSecret = types.StringValue(g.ClientSecret)
	m.RedirectURI = types.StringValue(g.RedirectURI)
	m.Scopes = types.StringValue(g.Scopes)
	m.CreatedAt = types.StringValue(g.GitProvider.CreatedAt)
}

func (r *giteaResource) redirectURI(v types.String) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return r.client.Endpoint() + "/api/providers/gitea/callback"
}

func (r *giteaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"client_secret": !cfg.ClientSecretWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	created, err := r.client.CreateGitea(ctx, client.CreateGiteaRequest{
		Name:             plan.Name.ValueString(),
		GiteaURL:         plan.GiteaURL.ValueString(),
		ClientID:         plan.ClientID.ValueString(),
		ClientSecret:     tfutil.SecretToCreate(plan.ClientSecret, cfg.ClientSecretWo),
		RedirectURI:      r.redirectURI(plan.RedirectURI),
		Scopes:           plan.Scopes.ValueString(),
		GiteaInternalURL: plan.GiteaInternalURL.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating Gitea provider", err.Error())
		return
	}
	flatten(created, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *giteaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	inUse, flagDiags := tfutil.WriteOnlyFlags(ctx, req.Private, secretNames)
	resp.Diagnostics.Append(flagDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetGitea(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Reading Gitea provider", err.Error())
		return
	}
	flatten(g, &state)
	hideWriteOnly(&state, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *giteaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg resourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inUse := map[string]bool{"client_secret": !cfg.ClientSecretWo.IsNull()}
	resp.Diagnostics.Append(tfutil.SetWriteOnlyFlags(ctx, resp.Private, secretNames, inUse)...)
	secret, send := tfutil.SecretToUpdate(plan.ClientSecret, cfg.ClientSecretWo, state.ClientSecret, plan.ClientSecretWoVersion, state.ClientSecretWoVersion)
	if !send {
		current, err := r.client.GetGitea(ctx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Reading Gitea provider before update", err.Error())
			return
		}
		secret = current.ClientSecret
	}
	if err := r.client.UpdateGitea(ctx, client.UpdateGiteaRequest{
		GiteaID:          plan.ID.ValueString(),
		GitProviderID:    state.GitProviderID.ValueString(),
		Name:             plan.Name.ValueString(),
		GiteaURL:         plan.GiteaURL.ValueString(),
		ClientID:         plan.ClientID.ValueString(),
		ClientSecret:     secret,
		RedirectURI:      r.redirectURI(plan.RedirectURI),
		Scopes:           plan.Scopes.ValueString(),
		GiteaInternalURL: plan.GiteaInternalURL.ValueStringPointer(),
	}); err != nil {
		resp.Diagnostics.AddError("Updating Gitea provider", err.Error())
		return
	}
	g, err := r.client.GetGitea(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading Gitea provider after update", err.Error())
		return
	}
	flatten(g, &plan)
	hideWriteOnly(&plan, inUse)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *giteaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.RemoveGitProvider(ctx, state.GitProviderID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting Gitea provider", err.Error())
	}
}

func (r *giteaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
