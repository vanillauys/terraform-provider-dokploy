package gitproviders

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*giteaDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*giteaDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*giteaDataSource)(nil)
)

type giteaDataSource struct{ client *client.Client }

func NewGiteaDataSource() datasource.DataSource { return &giteaDataSource{} }

type giteaModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	GitProviderID          types.String `tfsdk:"git_provider_id"`
	GiteaURL               types.String `tfsdk:"gitea_url"`
	ClientID               types.String `tfsdk:"client_id"`
	IsConfigured           types.Bool   `tfsdk:"is_configured"`
	SharedWithOrganization types.Bool   `tfsdk:"shared_with_organization"`
	CreatedAt              types.String `tfsdk:"created_at"`
}

func (d *giteaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gitea_provider"
}

func (d *giteaDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return exactlyOneOfIDOrName()
}

func (d *giteaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a Gitea connection that is registered in Dokploy (Git > Gitea), so that the `gitea` " +
			"source of an application or a compose does not hold a hardcoded id.\n\n" +
			"~> **`id` is the `giteaId`, not the `gitProviderId`.** An application references the Gitea-specific record.\n\n" +
			"~> `is_configured` is `false` until a person completes the OAuth2 authorization in the Dokploy UI. A deploy " +
			"from an unconfigured provider fails.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "The `giteaId`. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Provider name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"git_provider_id":          schema.StringAttribute{Computed: true, Description: "Id of the generic git-provider record."},
			"gitea_url":                schema.StringAttribute{Computed: true, Description: "URL of the Gitea instance."},
			"client_id":                schema.StringAttribute{Computed: true, Description: "Client ID of the OAuth2 application."},
			"is_configured":            schema.BoolAttribute{Computed: true, Description: "Whether the OAuth2 authorization completed and Dokploy holds an access token."},
			"shared_with_organization": schema.BoolAttribute{Computed: true, Description: "Whether the provider is shared with the whole organization."},
			"created_at":               schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *giteaDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *giteaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config giteaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := find(ctx, d.client, "gitea", "Gitea", config.ID.ValueString(), config.Name.ValueString(),
		func(p client.GitProviderSummary) string { return p.Gitea.GiteaID })
	if err != nil {
		resp.Diagnostics.AddError("Finding the Gitea provider", err.Error())
		return
	}
	config.ID = types.StringValue(found.Gitea.GiteaID)
	config.Name = types.StringValue(found.Name)
	config.GitProviderID = types.StringValue(found.GitProviderID)
	config.GiteaURL = types.StringValue(found.Gitea.GiteaURL)
	config.ClientID = types.StringValue(found.Gitea.ClientID)
	config.IsConfigured = types.BoolValue(found.Gitea.IsConfigured)
	config.SharedWithOrganization = types.BoolValue(found.SharedWithOrganization)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
