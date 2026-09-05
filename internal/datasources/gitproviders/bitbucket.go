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
	_ datasource.DataSource                     = (*bitbucketDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*bitbucketDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*bitbucketDataSource)(nil)
)

type bitbucketDataSource struct{ client *client.Client }

func NewBitbucketDataSource() datasource.DataSource { return &bitbucketDataSource{} }

type bitbucketModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	GitProviderID          types.String `tfsdk:"git_provider_id"`
	Username               types.String `tfsdk:"username"`
	IsConfigured           types.Bool   `tfsdk:"is_configured"`
	IsDeprecated           types.Bool   `tfsdk:"is_deprecated"`
	SharedWithOrganization types.Bool   `tfsdk:"shared_with_organization"`
	CreatedAt              types.String `tfsdk:"created_at"`
}

func (d *bitbucketDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_provider"
}

func (d *bitbucketDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return exactlyOneOfIDOrName()
}

func (d *bitbucketDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a Bitbucket connection that is registered in Dokploy (Git > Bitbucket), so that the " +
			"`bitbucket` source of an application or a compose does not hold a hardcoded id.\n\n" +
			"~> **`id` is the `bitbucketId`, not the `gitProviderId`.** An application references the Bitbucket-specific record.\n\n" +
			"~> The data source does not expose the app password or the API token.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "The `bitbucketId`. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Provider name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"git_provider_id":          schema.StringAttribute{Computed: true, Description: "Id of the generic git-provider record."},
			"username":                 schema.StringAttribute{Computed: true, Description: "Bitbucket username, or null for the API-token shape."},
			"is_configured":            schema.BoolAttribute{Computed: true, Description: "Whether Dokploy holds usable credentials."},
			"is_deprecated":            schema.BoolAttribute{Computed: true, Description: "Whether the record uses the deprecated app-password shape."},
			"shared_with_organization": schema.BoolAttribute{Computed: true, Description: "Whether the provider is shared with the whole organization."},
			"created_at":               schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *bitbucketDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *bitbucketDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config bitbucketModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := find(ctx, d.client, "bitbucket", "Bitbucket", config.ID.ValueString(), config.Name.ValueString(),
		func(p client.GitProviderSummary) string { return p.Bitbucket.BitbucketID })
	if err != nil {
		resp.Diagnostics.AddError("Finding the Bitbucket provider", err.Error())
		return
	}
	config.ID = types.StringValue(found.Bitbucket.BitbucketID)
	config.Name = types.StringValue(found.Name)
	config.GitProviderID = types.StringValue(found.GitProviderID)
	config.Username = tfutil.StringOrNull(&found.Bitbucket.BitbucketUsername)
	config.IsConfigured = types.BoolValue(found.Bitbucket.IsConfigured)
	config.IsDeprecated = types.BoolValue(found.Bitbucket.IsDeprecated)
	config.SharedWithOrganization = types.BoolValue(found.SharedWithOrganization)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
