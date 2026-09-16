// Package registry holds the dokploy_registry data source.
package registry

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
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
	return dsutil.IDOrName()
}

func (d *registryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	lookup := dsutil.Lookup{
		Kind: "registry", Plural: "registries",
		What: "a container registry login that already exists in Dokploy (Settings > Registry), so that an " +
			"application can push its built images to it",
		Example: "data \"dokploy_registry\" \"ghcr\" {\n  name = \"ghcr\"\n}\n\n" +
			"resource \"dokploy_application\" \"api\" {\n  registry_id = data.dokploy_registry.ghcr.id\n  # ...\n}",
		Secret: "the password", SecretAttr: "`password`", Resource: "`dokploy_registry`",
	}
	attrs := lookup.Attributes()
	attrs["url"] = dsutil.String("Registry host, with an optional port and without a scheme.")
	attrs["username"] = dsutil.String("Login user.")
	attrs["image_prefix"] = dsutil.String("Path that Dokploy puts in front of each image name it pushes, or null.")
	attrs["registry_type"] = dsutil.String("Registry type, `cloud` on Dokploy v0.30.")
	attrs["organization_id"] = dsutil.String("Id of the organization that owns the registry.")
	attrs["created_at"] = dsutil.String("Creation timestamp from the server.")
	resp.Schema = schema.Schema{Description: lookup.Description(), Attributes: attrs}
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
