// Package tag holds the dokploy_tag data source.
package tag

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
	_ datasource.DataSource                     = (*tagDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*tagDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*tagDataSource)(nil)
)

type tagDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &tagDataSource{} }

type model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Color          types.String `tfsdk:"color"`
	CreatedAt      types.String `tfsdk:"created_at"`
	OrganizationID types.String `tfsdk:"organization_id"`
}

func (d *tagDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (d *tagDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return dsutil.IDOrName()
}

func (d *tagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	lookup := dsutil.Lookup{
		Kind: "tag", Plural: "tags",
		What:    "a tag that already exists in Dokploy, so that a project can carry it in `tag_ids`",
		Example: "data \"dokploy_tag\" \"prod\" {\n  name = \"production\"\n}",
		Note: "Tag names are unique in the organization, so a lookup by name finds at most one record; the " +
			"duplicate-name failure of the other lookups cannot happen here.",
	}
	attrs := lookup.Attributes()
	attrs["color"] = dsutil.String("Colour of the label as Dokploy stores it, or null when the tag has none.")
	attrs["created_at"] = dsutil.String("Creation timestamp.")
	attrs["organization_id"] = dsutil.String("Id of the organization that owns the tag.")
	resp.Schema = schema.Schema{Description: lookup.Description(), Attributes: attrs}
}

func (d *tagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *tagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Tag
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetTag(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the tag", err.Error())
			return
		}
		found = got
	} else {
		tags, err := d.client.ListTags(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing tags", err.Error())
			return
		}
		got, err := lookup.Record(tags, config.Name.ValueString(), "tag", func(t client.Tag) string { return t.Name })
		if err != nil {
			resp.Diagnostics.AddError("Finding the tag", err.Error())
			return
		}
		found = &got
	}
	config.ID = types.StringValue(found.TagID)
	config.Name = types.StringValue(found.Name)
	config.Color = tfutil.StringOrNull(found.Color)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	config.OrganizationID = types.StringValue(found.OrganizationID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
