// Package certificate holds the dokploy_certificate data source.
package certificate

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
	_ datasource.DataSource                     = (*certificateDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*certificateDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*certificateDataSource)(nil)
)

type certificateDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &certificateDataSource{} }

// private_key is deliberately NOT modelled. Every read of a certificate
// returns the key in cleartext (client.Certificate), so nothing on the wire
// keeps it out; the data source does, because a consumer needs only the id.
type model struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CertificateData types.String `tfsdk:"certificate_data"`
	CertificatePath types.String `tfsdk:"certificate_path"`
	AutoRenew       types.Bool   `tfsdk:"auto_renew"`
	ServerID        types.String `tfsdk:"server_id"`
	OrganizationID  types.String `tfsdk:"organization_id"`
}

func (d *certificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (d *certificateDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *certificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a TLS certificate that already exists in Dokploy (Settings > Certificates), so that a domain " +
			"with `certificate_type = \"custom\"` can reference it:\n\n" +
			"```terraform\n" +
			"data \"dokploy_certificate\" \"wildcard\" {\n  name = \"wildcard-example-com\"\n}\n" +
			"```\n\n" +
			"~> **The data source does not expose the private key.** `private_key` exists on the `dokploy_certificate` " +
			"resource, but not here, by design. A consumer needs only the id.\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two certificates share a name, this data source fails instead " +
			"of a guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Certificate id. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"certificate_data": schema.StringAttribute{Computed: true, Description: "The certificate chain in PEM format."},
			"certificate_path": schema.StringAttribute{Computed: true, Description: "Name of the Traefik certificate file that Dokploy generates."},
			"auto_renew":       schema.BoolAttribute{Computed: true, Description: "Whether Dokploy renews the certificate."},
			"server_id":        schema.StringAttribute{Computed: true, Description: "Id of the server that serves the certificate, or null for the Dokploy host."},
			"organization_id":  schema.StringAttribute{Computed: true, Description: "Id of the organization that owns the certificate."},
		},
	}
}

func (d *certificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *certificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Certificate
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetCertificate(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the certificate", err.Error())
			return
		}
		found = got
	} else {
		certs, err := d.client.ListCertificates(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing certificates", err.Error())
			return
		}
		got, err := lookup.Record(certs, config.Name.ValueString(), "certificate", func(c client.Certificate) string { return c.Name })
		if err != nil {
			resp.Diagnostics.AddError("Finding the certificate", err.Error())
			return
		}
		found = &got
	}
	config.ID = types.StringValue(found.CertificateID)
	config.Name = types.StringValue(found.Name)
	config.CertificateData = types.StringValue(found.CertificateData)
	config.CertificatePath = types.StringValue(found.CertificatePath)
	config.AutoRenew = types.BoolValue(found.AutoRenew)
	config.ServerID = tfutil.StringOrNull(&found.ServerID)
	config.OrganizationID = types.StringValue(found.OrganizationID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
