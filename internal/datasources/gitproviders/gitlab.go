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
	_ datasource.DataSource                     = (*gitlabDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*gitlabDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*gitlabDataSource)(nil)
)

type gitlabDataSource struct{ client *client.Client }

func NewGitlabDataSource() datasource.DataSource { return &gitlabDataSource{} }

type gitlabModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	GitProviderID          types.String `tfsdk:"git_provider_id"`
	GitlabURL              types.String `tfsdk:"gitlab_url"`
	ApplicationID          types.String `tfsdk:"application_id"`
	IsConfigured           types.Bool   `tfsdk:"is_configured"`
	SharedWithOrganization types.Bool   `tfsdk:"shared_with_organization"`
	CreatedAt              types.String `tfsdk:"created_at"`
}

func (d *gitlabDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gitlab_provider"
}

func (d *gitlabDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return exactlyOneOfIDOrName()
}

func (d *gitlabDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a GitLab connection that is registered in Dokploy (Git > GitLab), so that the `gitlab` " +
			"source of an application or a compose does not hold a hardcoded id:\n\n" +
			"```terraform\n" +
			"data \"dokploy_gitlab_provider\" \"main\" {\n  name = \"my-group\"\n}\n\n" +
			"resource \"dokploy_application\" \"web\" {\n  gitlab = {\n    gitlab_id = data.dokploy_gitlab_provider.main.id\n    # ...\n  }\n}\n" +
			"```\n\n" +
			"~> **`id` is the `gitlabId`, not the `gitProviderId`.** An application references the GitLab-specific record.\n\n" +
			"~> `is_configured` is `false` until a person completes the OAuth authorization in the Dokploy UI. A deploy " +
			"from an unconfigured provider fails.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "The `gitlabId`. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Provider name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"git_provider_id":          schema.StringAttribute{Computed: true, Description: "Id of the generic git-provider record."},
			"gitlab_url":               schema.StringAttribute{Computed: true, Description: "URL of the GitLab instance."},
			"application_id":           schema.StringAttribute{Computed: true, Description: "Application ID of the OAuth application."},
			"is_configured":            schema.BoolAttribute{Computed: true, Description: "Whether the OAuth authorization completed and Dokploy holds an access token."},
			"shared_with_organization": schema.BoolAttribute{Computed: true, Description: "Whether the provider is shared with the whole organization."},
			"created_at":               schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *gitlabDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *gitlabDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config gitlabModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := find(ctx, d.client, "gitlab", "GitLab", config.ID.ValueString(), config.Name.ValueString(),
		func(p client.GitProviderSummary) string { return p.Gitlab.GitlabID })
	if err != nil {
		resp.Diagnostics.AddError("Finding the GitLab provider", err.Error())
		return
	}
	config.ID = types.StringValue(found.Gitlab.GitlabID)
	config.Name = types.StringValue(found.Name)
	config.GitProviderID = types.StringValue(found.GitProviderID)
	config.GitlabURL = types.StringValue(found.Gitlab.GitlabURL)
	config.ApplicationID = types.StringValue(found.Gitlab.ApplicationID)
	config.IsConfigured = types.BoolValue(found.Gitlab.IsConfigured)
	config.SharedWithOrganization = types.BoolValue(found.SharedWithOrganization)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
