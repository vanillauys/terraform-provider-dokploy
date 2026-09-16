// Package vaultprovider holds the dokploy_vault_provider data source.
package vaultprovider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
	"github.com/vanillauys/terraform-provider-dokploy/internal/lookup"
	resourcevault "github.com/vanillauys/terraform-provider-dokploy/internal/resources/vaultprovider"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*vaultProviderDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*vaultProviderDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*vaultProviderDataSource)(nil)
)

type vaultProviderDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &vaultProviderDataSource{} }

// The config block is deliberately NOT modelled. The server masks every
// secret in it on read (client.VaultProvider), and a consumer of the data
// source needs only the name for the `${{vault.<name>.<key>}}` syntax and
// the id for a reference. provider_type says which kind of vault it is.
type model struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	ProviderType types.String `tfsdk:"provider_type"`
	Assignments  types.List   `tfsdk:"assignments"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

func (d *vaultProviderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_provider"
}

func (d *vaultProviderDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return dsutil.IDOrName()
}

func (d *vaultProviderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	lookup := dsutil.Lookup{
		Kind: "vault provider", Plural: "vault providers",
		What: "a vault provider that already exists in Dokploy (Settings > Vault Providers), for example to " +
			"confirm the name that the `${{vault.<name>.<key>}}` environment variable syntax refers to",
		Example: "data \"dokploy_vault_provider\" \"prod\" {\n  name = \"prod\"\n}",
		Note: "The `provider_type` attribute says which kind of vault the record connects to. Dokploy masks the secrets " +
			"of the connection config on every read, so the config blocks of the resource are not part of the data source.",
	}
	attrs := lookup.Attributes()
	attrs["provider_type"] = dsutil.String("Kind of vault: `hashicorp` (also OpenBao), `infisical`, `aws`, `doppler`, `azure`, `scaleway`, " +
		"`phase`, or `aws-parameter-store`.")
	attrs["assignments"] = schema.ListNestedAttribute{
		Computed:    true,
		Description: "Projects, and optionally specific environments in them, that can use this vault provider.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"project_id": dsutil.String("Id of the assigned project."),
				"environment_ids": schema.SetAttribute{
					Computed:    true,
					ElementType: types.StringType,
					Description: "Ids of the environments that the assignment covers. Empty when it covers each environment in the project.",
				},
			},
		},
	}
	attrs["created_at"] = dsutil.String("Creation timestamp from the server.")
	resp.Schema = schema.Schema{Description: lookup.Description(), Attributes: attrs}
}

func (d *vaultProviderDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *vaultProviderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.VaultProvider
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetVaultProvider(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the vault provider", err.Error())
			return
		}
		found = got
	} else {
		vs, err := d.client.ListVaultProviders(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing vault providers", err.Error())
			return
		}
		got, err := lookup.Record(vs, config.Name.ValueString(), "vault provider", func(v client.VaultProvider) string { return v.Name })
		if err != nil {
			resp.Diagnostics.AddError("Finding the vault provider", err.Error())
			return
		}
		found = &got
	}
	config.ID = types.StringValue(found.VaultProviderID)
	config.Name = types.StringValue(found.Name)
	config.ProviderType = types.StringValue(found.ProviderType)
	config.Assignments = resourcevault.FlattenAssignments(ctx, found.Assignments, &resp.Diagnostics)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
