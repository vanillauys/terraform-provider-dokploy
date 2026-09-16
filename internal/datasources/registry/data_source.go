// Package registry holds the dokploy_registry data source.
package registry

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/lookup"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*registryDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*registryDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*registryDataSource)(nil)
)

type registryDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &registryDataSource{} }

// password is deliberately NOT modelled. registry.all returns it in
// cleartext (client.Registry), so nothing on the wire keeps it out; the
// data source does, because a consumer needs only the id.
type model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	URL            types.String `tfsdk:"url"`
	Username       types.String `tfsdk:"username"`
	ImagePrefix    types.String `tfsdk:"image_prefix"`
	RegistryType   types.String `tfsdk:"registry_type"`
	OrganizationID types.String `tfsdk:"organization_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (d *registryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry"
}

func (d *registryDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *registryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a container registry login that already exists in Dokploy (Settings > Registry), so that an " +
			"application can push its built images to it:\n\n" +
			"```terraform\n" +
			"data \"dokploy_registry\" \"ghcr\" {\n  name = \"ghcr\"\n}\n\n" +
			"resource \"dokploy_application\" \"api\" {\n  registry_id = data.dokploy_registry.ghcr.id\n  # ...\n}\n" +
			"```\n\n" +
			"~> **The data source does not expose the password.** `password` exists on the `dokploy_registry` " +
			"resource, but not here, by design. A consumer needs only the id.\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two registries share a name, this data source fails instead " +
			"of a guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Registry id. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"url":             schema.StringAttribute{Computed: true, Description: "Registry host, with an optional port and without a scheme."},
			"username":        schema.StringAttribute{Computed: true, Description: "Login user."},
			"image_prefix":    schema.StringAttribute{Computed: true, Description: "Path that Dokploy puts in front of each image name it pushes, or null."},
			"registry_type":   schema.StringAttribute{Computed: true, Description: "Registry type, `cloud` on Dokploy v0.30."},
			"organization_id": schema.StringAttribute{Computed: true, Description: "Id of the organization that owns the registry."},
			"created_at":      schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *registryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *registryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Registry
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetRegistry(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the registry", err.Error())
			return
		}
		found = got
	} else {
		regs, err := d.client.ListRegistries(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing registries", err.Error())
			return
		}
		got, err := lookup.Record(regs, config.Name.ValueString(), "registry", func(r client.Registry) string { return r.RegistryName })
		if err != nil {
			resp.Diagnostics.AddError("Finding the registry", err.Error())
			return
		}
		found = &got
	}
	config.ID = types.StringValue(found.RegistryID)
	config.Name = types.StringValue(found.RegistryName)
	config.URL = types.StringValue(found.RegistryURL)
	config.Username = types.StringValue(found.Username)
	config.ImagePrefix = tfutil.StringOrNull(&found.ImagePrefix)
	config.RegistryType = types.StringValue(found.RegistryType)
	config.OrganizationID = types.StringValue(found.OrganizationID)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
