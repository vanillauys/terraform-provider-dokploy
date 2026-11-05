// Package telemetryprovider holds the dokploy_telemetry_provider data source.
package telemetryprovider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
)

// model has no endpoint, api_key or api_secret: Dokploy never returns them.
type model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ProviderType   types.String `tfsdk:"provider_type"`
	Signals        types.List   `tfsdk:"signals"`
	ExtraConfig    types.Map    `tfsdk:"extra_config"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	OrganizationID types.String `tfsdk:"organization_id"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func flatten(p *client.TelemetryProvider, out *model) {
	out.ID = types.StringValue(p.TelemetryProviderID)
	out.Name = types.StringValue(p.Name)
	out.ProviderType = types.StringValue(p.ProviderType)
	out.Signals, _ = types.ListValueFrom(context.Background(), types.StringType, p.Signals)
	out.ExtraConfig, _ = types.MapValueFrom(context.Background(), types.StringType, p.ExtraConfigValues())
	out.Enabled = types.BoolValue(p.Enabled)
	out.OrganizationID = types.StringValue(p.OrganizationID)
	out.CreatedAt = types.StringValue(p.CreatedAt)
}

// NewDataSource returns the dokploy_telemetry_provider data source.
func NewDataSource() datasource.DataSource {
	l := dsutil.Lookup{
		Kind: "telemetry provider", Plural: "telemetry providers",
		What:    "a telemetry provider that already exists in Dokploy (Settings > Logs & Metrics)",
		Example: "data \"dokploy_telemetry_provider\" \"loki\" {\n  name = \"loki\"\n}",
		Note: "Dokploy never returns the endpoint, the API key, or the API secret of a provider, so the data " +
			"source does not expose them.",
	}
	attrs := l.Attributes()
	attrs["provider_type"] = dsutil.String("Destination type, for example `loki` or `datadog`.")
	attrs["signals"] = dsutil.StringList("Data that the Vector agent sends: `logs`, `metrics`, or both.")
	attrs["extra_config"] = schema.MapAttribute{
		Computed: true, ElementType: types.StringType,
		Description: "Fields that only some types use, for example `region` or `tenantId`.",
	}
	attrs["enabled"] = dsutil.Bool("Whether the Vector agents send data to the destination.")
	attrs["organization_id"] = dsutil.String("Id of the organization that owns the provider.")
	attrs["created_at"] = dsutil.String("Creation timestamp from the server.")
	return dsutil.NewDataSource(dsutil.Record[model, client.TelemetryProvider]{
		Name:        "telemetry_provider",
		Description: l.Description(),
		Attributes:  attrs,
		Validators:  dsutil.IDOrName(),
		ID:          func(m *model) types.String { return m.ID },
		Get: func(ctx context.Context, c *client.Client, id string) (*client.TelemetryProvider, error) {
			return c.GetTelemetryProvider(ctx, id)
		},
		Find: func(ctx context.Context, c *client.Client, m *model) (*client.TelemetryProvider, error) {
			all, err := c.ListTelemetryProviders(ctx)
			if err != nil {
				return nil, err
			}
			return dsutil.Find(all, func(x client.TelemetryProvider) bool { return x.Name == m.Name.ValueString() },
				"telemetry provider", "the name "+m.Name.ValueString())
		},
		Flatten: flatten,
	})()
}
