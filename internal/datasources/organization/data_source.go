// Package organization holds the dokploy_organization data source.
package organization

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
	_ datasource.DataSource                     = (*organizationDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*organizationDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*organizationDataSource)(nil)
)

type organizationDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &organizationDataSource{} }

type model struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Logo        types.String `tfsdk:"logo"`
	DefaultRole types.String `tfsdk:"default_role"`
	Slug        types.String `tfsdk:"slug"`
	OwnerID     types.String `tfsdk:"owner_id"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

func (d *organizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *organizationDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.Conflicting(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *organizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an organization. With neither `id` nor `name` set, it returns the API key's active " +
			"organization: the one that every resource of this provider lands in.\n\n" +
			"```terraform\n" +
			"data \"dokploy_organization\" \"current\" {}\n\n" +
			"output \"organization_id\" {\n  value = data.dokploy_organization.current.id\n}\n" +
			"```\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two organizations share a name, this data source fails " +
			"instead of a guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Organization id. Set it for a lookup by id, set `name` for a lookup by name, or set neither for the active organization.",
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Display name. Do not set it together with `id`.",
			},
			"logo":         schema.StringAttribute{Computed: true, Description: "URL of the logo image, or null."},
			"default_role": schema.StringAttribute{Computed: true, Description: "Role that a new member gets, or null for the Dokploy default."},
			"slug":         schema.StringAttribute{Computed: true, Description: "URL slug that Dokploy generates."},
			"owner_id":     schema.StringAttribute{Computed: true, Description: "User id of the owner."},
			"created_at":   schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *organizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func findByName(orgs []client.Organization, name string) (*client.Organization, error) {
	var matches []client.Organization
	for _, o := range orgs {
		if o.Name == name {
			matches = append(matches, o)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no organization named %q", name)
	default:
		return nil, fmt.Errorf(
			"%d organizations are named %q; names are not unique in Dokploy, so look it up by id instead",
			len(matches), name)
	}
}

func (d *organizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Organization
	var err error
	switch {
	case config.ID.ValueString() != "":
		found, err = d.client.GetOrganization(ctx, config.ID.ValueString())
	case config.Name.ValueString() != "":
		var orgs []client.Organization
		if orgs, err = d.client.ListOrganizations(ctx); err == nil {
			found, err = findByName(orgs, config.Name.ValueString())
		}
	default:
		found, err = d.client.GetActiveOrganization(ctx)
	}
	if err != nil {
		resp.Diagnostics.AddError("Finding the organization", err.Error())
		return
	}
	config.ID = types.StringValue(found.ID)
	config.Name = types.StringValue(found.Name)
	config.Logo = tfutil.StringOrNull(&found.Logo)
	config.DefaultRole = tfutil.StringOrNull(&found.DefaultRole)
	config.Slug = tfutil.StringOrNull(&found.Slug)
	config.OwnerID = types.StringValue(found.OwnerID)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
