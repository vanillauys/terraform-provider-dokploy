// Package certificate holds the dokploy_certificate data source.
package certificate

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
	return dsutil.IDOrName()
}

func (d *certificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	lookup := dsutil.Lookup{
		Kind: "certificate", Plural: "certificates",
		What: "a TLS certificate that already exists in Dokploy (Settings > Certificates), so that a domain with " +
			"`certificate_type = \"custom\"` can reference it",
		Example: "data \"dokploy_certificate\" \"wildcard\" {\n  name = \"wildcard-example-com\"\n}",
		Secret:  "the private key", SecretAttr: "`private_key`", Resource: "`dokploy_certificate`",
	}
	attrs := lookup.Attributes()
	attrs["certificate_data"] = dsutil.String("The certificate chain in PEM format.")
	attrs["certificate_path"] = dsutil.String("Name of the Traefik certificate file that Dokploy generates.")
	attrs["auto_renew"] = dsutil.Bool("Whether Dokploy renews the certificate.")
	attrs["server_id"] = dsutil.String("Id of the server that serves the certificate, or null for the Dokploy host.")
	attrs["organization_id"] = dsutil.String("Id of the organization that owns the certificate.")
	resp.Schema = schema.Schema{Description: lookup.Description(), Attributes: attrs}
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
