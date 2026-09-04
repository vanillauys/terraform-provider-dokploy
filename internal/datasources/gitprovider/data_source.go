// Package gitprovider holds the dokploy_github_provider data source.
//
// Scoped to GitHub deliberately. Dokploy also has gitlab, bitbucket and
// gitea providers, and each has its own `<type>Providers` list endpoint —
// but none is modelled here, because none has been observed live: the
// acceptance rig has no provider of any type (installing one is a
// browser-bound flow), and the only instance available to develop against
// has a GitHub App and nothing else. Adding three data sources whose
// response shapes were inferred from this one is precisely the assumption
// internal/client/census_test.go exists to prevent. They wait for evidence.
package gitprovider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*githubProviderDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*githubProviderDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*githubProviderDataSource)(nil)
)

type githubProviderDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &githubProviderDataSource{} }

type model struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	GitProviderID          types.String `tfsdk:"git_provider_id"`
	ProviderType           types.String `tfsdk:"provider_type"`
	CreatedAt              types.String `tfsdk:"created_at"`
	SharedWithOrganization types.Bool   `tfsdk:"shared_with_organization"`
}

func (d *githubProviderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_github_provider"
}

func (d *githubProviderDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *githubProviderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a GitHub App that is registered in Dokploy (Git > GitHub).\n\n" +
			"Use it so that the `github_id` of `dokploy_application` is not a hardcoded opaque id:\n\n" +
			"```terraform\n" +
			"data \"dokploy_github_provider\" \"main\" {\n  name = \"my-org\"\n}\n\n" +
			"resource \"dokploy_application\" \"web\" {\n  github = {\n    github_id = data.dokploy_github_provider.main.id\n    # ...\n  }\n}\n" +
			"```\n\n" +
			"~> **`id` is the `githubId`, not the `gitProviderId`.** Dokploy keeps both: `git_provider_id` is the generic record, " +
			"and `id` is the GitHub-specific record. An application references the GitHub-specific record. Validation accepts " +
			"the wrong id, and the request then fails with an HTTP 500, because only the database layer enforces the foreign key.\n\n" +
			"~> Terraform cannot create a GitHub App. The Dokploy API has no `github.create`, and the installation is a " +
			"browser flow. This data source reads what already exists.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "The `githubId`. Set it for a lookup by id, or leave it unset and set `name`. " +
					"This is the value that `dokploy_application.github.github_id` expects.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Provider name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"git_provider_id": schema.StringAttribute{
				Computed:    true,
				Description: "Id of the generic git-provider record that owns this GitHub App. An application does not reference it.",
			},
			"provider_type": schema.StringAttribute{
				Computed:    true,
				Description: "Always `github` for this data source.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp from the server.",
			},
			"shared_with_organization": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the provider is shared with the whole Dokploy organization.",
			},
		},
	}
}

func (d *githubProviderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// find returns the single provider matching id or name.
//
// Errors on zero AND on multiple matches rather than taking [0]: nothing in
// Dokploy makes provider names unique, and silently binding an application
// to whichever duplicate sorted first is the failure mode this codebase
// treats as a bug everywhere else.
func find(providers []client.GithubProvider, id, name string) (*client.GithubProvider, error) {
	var matches []client.GithubProvider
	for _, p := range providers {
		if (id != "" && p.GithubID == id) || (name != "" && p.GitProvider.Name == name) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		if id != "" {
			return nil, fmt.Errorf("no GitHub provider with id %q", id)
		}
		return nil, fmt.Errorf("no GitHub provider named %q", name)
	default:
		return nil, fmt.Errorf(
			"%d GitHub providers are named %q; names are not unique in Dokploy, so look it up by id instead",
			len(matches), name)
	}
}

func (d *githubProviderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	providers, err := d.client.ListGithubProviders(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing GitHub providers", err.Error())
		return
	}
	found, err := find(providers, config.ID.ValueString(), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Finding the GitHub provider", err.Error())
		return
	}

	config.ID = types.StringValue(found.GithubID)
	config.Name = types.StringValue(found.GitProvider.Name)
	config.GitProviderID = types.StringValue(found.GitProvider.GitProviderID)
	config.ProviderType = types.StringValue(found.GitProvider.ProviderType)
	config.CreatedAt = types.StringValue(found.GitProvider.CreatedAt)
	config.SharedWithOrganization = types.BoolValue(found.GitProvider.SharedWithOrganization)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
