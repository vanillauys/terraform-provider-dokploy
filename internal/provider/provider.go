package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	dsproject "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/project"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/application"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/postgres"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/project"
)

var _ provider.Provider = (*DokployProvider)(nil)

type resolvedConfig struct {
	endpoint string
	apiKey   string
	insecure bool
}

// resolveConfig applies the DOKPLOY_ENDPOINT / DOKPLOY_API_KEY fallbacks
// and reports which required settings are still missing.
func resolveConfig(m DokployProviderModel, getenv func(string) string) (resolvedConfig, []string) {
	rc := resolvedConfig{
		endpoint: m.Endpoint.ValueString(),
		apiKey:   m.ApiKey.ValueString(),
		insecure: m.Insecure.ValueBool(),
	}
	if rc.endpoint == "" {
		rc.endpoint = getenv("DOKPLOY_ENDPOINT")
	}
	if rc.apiKey == "" {
		rc.apiKey = getenv("DOKPLOY_API_KEY")
	}
	var missing []string
	if rc.endpoint == "" {
		missing = append(missing, `"endpoint" (or the DOKPLOY_ENDPOINT environment variable)`)
	}
	if rc.apiKey == "" {
		missing = append(missing, `"api_key" (or the DOKPLOY_API_KEY environment variable)`)
	}
	return rc, missing
}

type DokployProvider struct {
	version string
}

type DokployProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	ApiKey   types.String `tfsdk:"api_key"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DokployProvider{version: version}
	}
}

func (p *DokployProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "dokploy"
	resp.Version = p.version
}

func (p *DokployProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage resources on a Dokploy server.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the Dokploy server, e.g. `https://dokploy.example.com`. Falls back to the `DOKPLOY_ENDPOINT` environment variable.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Dokploy API key, sent as the `x-api-key` header. Falls back to the `DOKPLOY_API_KEY` environment variable.",
			},
			"insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip TLS certificate verification (for self-signed endpoints). Defaults to `false`.",
			},
		},
	}
}

func (p *DokployProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config DokployProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rc, missing := resolveConfig(config, os.Getenv)
	for _, m := range missing {
		resp.Diagnostics.AddError(
			"Missing provider configuration",
			fmt.Sprintf("The %s must be set.", m),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := client.New(rc.endpoint, rc.apiKey, rc.insecure, p.version)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider configuration", err.Error())
		return
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *DokployProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		project.NewResource,
		postgres.NewResource,
		application.NewResource,
	}
}

func (p *DokployProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		dsproject.NewDataSource,
	}
}
