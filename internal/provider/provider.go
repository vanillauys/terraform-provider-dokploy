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
	dsapplication "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/application"
	dsdatabase "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/database"
	dsdestination "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/destination"
	dsenvironment "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/environment"
	dsgitprovider "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/gitprovider"
	dsgitproviders "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/gitproviders"
	libsqldatasource "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/libsql"
	dsnetwork "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/network"
	dsorganization "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/organization"
	dsproject "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/project"
	dsserver "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/server"
	dssshkey "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/sshkey"
	dsuser "github.com/vanillauys/terraform-provider-dokploy/internal/datasources/user"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/ai"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/apikey"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/appchild"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/application"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/backup"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/bitbucketprovider"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/certificate"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/compose"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/database"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/destination"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/domain"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/environment"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/giteaprovider"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/gitlabprovider"
	libsqlresource "github.com/vanillauys/terraform-provider-dokploy/internal/resources/libsql"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/mount"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/network"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/notification"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/organization"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/project"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/registry"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/schedule"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/server"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/sshkey"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/user"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/userpermissions"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/vaultprovider"
	"github.com/vanillauys/terraform-provider-dokploy/internal/resources/volumebackup"
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
	// client is read (never written) by the database package's Kind
	// constructors, which Resources() calls fresh on every invocation — see
	// the comment on that registration below for why that makes a client
	// captured here, rather than one passed once at registration time,
	// necessary.
	client *client.Client
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
				Description: "Base URL of the Dokploy server, for example `https://dokploy.example.com`. If unset, the provider reads the `DOKPLOY_ENDPOINT` environment variable.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Dokploy API key. The provider sends it as the `x-api-key` header. If unset, the provider reads the `DOKPLOY_API_KEY` environment variable.",
			},
			"insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip the TLS certificate verification, for self-signed endpoints. Defaults to `false`.",
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
	p.client = c
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *DokployProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		project.NewResource,
		environment.NewResource,
		// database.PostgresKind(p.client) is deliberately NOT hoisted out of
		// this closure into a value computed once here. Resources() itself
		// runs exactly once, cached by the framework from its very first
		// GetProviderSchema call — which always precedes Configure, so
		// p.client would still be nil at that point. But the closure below
		// (the `func() resource.Resource` element of this slice) is what the
		// framework actually caches; it re-invokes THAT closure fresh for
		// every subsequent Create/Read/Update/Delete/ImportState call, all of
		// which happen after Configure has already set p.client. So
		// `database.PostgresKind(p.client)` reads the field lazily, at call
		// time, and only ever observes the nil client on the harmless
		// Metadata/Schema-only pass. Verified directly against
		// terraform-plugin-framework v1.19.0 (internal/fwserver/server.go,
		// Server.Resource) rather than assumed; see database.PostgresKind's
		// doc comment for the same reasoning from the other side. Tasks 5-7
		// must register their engines' Kinds the same way.
		func() resource.Resource { return database.NewResource(database.PostgresKind(p.client))() },
		// Same reasoning as PostgresKind above, mirrored exactly for mysql
		// (wave-2 task 5). Tasks 6-7 register mariadb/mongo/redis the same
		// way.
		func() resource.Resource { return database.NewResource(database.MysqlKind(p.client))() },
		// Same reasoning again, mirrored for redis (wave-2 task 6).
		func() resource.Resource { return database.NewResource(database.RedisKind(p.client))() },
		// Same reasoning again, mirrored for mariadb (wave-2 task 7).
		func() resource.Resource { return database.NewResource(database.MariadbKind(p.client))() },
		// Same reasoning again, mirrored for mongo (wave-2 task 7).
		func() resource.Resource { return database.NewResource(database.MongoKind(p.client))() },
		application.NewResource,
		domain.NewResource,
		mount.NewResource,
		destination.NewResource,
		sshkey.NewResource,
		server.NewResource,
		certificate.NewResource,
		ai.NewResource,
		registry.NewResource,
		gitlabprovider.NewResource,
		bitbucketprovider.NewResource,
		giteaprovider.NewResource,
		organization.NewResource,
		user.NewResource,
		userpermissions.NewResource,
		apikey.NewResource,
		network.NewResource,
		schedule.NewResource,
		vaultprovider.NewResource,
		volumebackup.NewResource,
		backup.NewResource,
		compose.NewResource,
		libsqlresource.NewResource,
		appchild.NewResource(appchild.PortKind()),
		appchild.NewResource(appchild.RedirectKind()),
		appchild.NewResource(appchild.SecurityKind()),
		notification.NewResource(notification.SlackKind()),
		notification.NewResource(notification.DiscordKind()),
		notification.NewResource(notification.TelegramKind()),
		notification.NewResource(notification.EmailKind()),
		notification.NewResource(notification.ResendKind()),
		notification.NewResource(notification.GotifyKind()),
		notification.NewResource(notification.NtfyKind()),
		notification.NewResource(notification.MattermostKind()),
		notification.NewResource(notification.LarkKind()),
		notification.NewResource(notification.TeamsKind()),
		notification.NewResource(notification.PushoverKind()),
		notification.NewResource(notification.CustomKind()),
	}
}

func (p *DokployProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		dsproject.NewDataSource,
		dsapplication.NewDataSource,
		// database.PostgresKind(p.client) is deliberately re-evaluated inside
		// this closure rather than hoisted out, for the exact same reason as
		// the resource registration below: DataSources() itself is cached by
		// the framework from its first GetProviderSchema call (before
		// Configure), but the `func() datasource.DataSource` element of this
		// slice is re-invoked fresh for every real Read call, by which time
		// p.client is the real, configured client. See Resources()'s comment
		// on the resource-side registration and dsdatabase.genericDataSource's
		// doc comment for why this data source has no Configure() of its own.
		func() datasource.DataSource { return dsdatabase.NewDataSource(database.PostgresKind(p.client))() },
		// Same reasoning, mirrored for mysql (wave-2 task 5).
		func() datasource.DataSource { return dsdatabase.NewDataSource(database.MysqlKind(p.client))() },
		// Same reasoning, mirrored for redis (wave-2 task 6).
		func() datasource.DataSource { return dsdatabase.NewDataSource(database.RedisKind(p.client))() },
		// Same reasoning, mirrored for mariadb (wave-2 task 7).
		func() datasource.DataSource { return dsdatabase.NewDataSource(database.MariadbKind(p.client))() },
		// Same reasoning, mirrored for mongo (wave-2 task 7).
		func() datasource.DataSource { return dsdatabase.NewDataSource(database.MongoKind(p.client))() },
		dsenvironment.NewDataSource,
		dsgitprovider.NewDataSource,
		dsgitproviders.NewGitlabDataSource,
		dsgitproviders.NewBitbucketDataSource,
		dsgitproviders.NewGiteaDataSource,
		dsdestination.NewDataSource,
		dssshkey.NewDataSource,
		dsserver.NewDataSource,
		dsorganization.NewDataSource,
		dsuser.NewDataSource,
		libsqldatasource.NewDataSource,
		dsnetwork.NewDataSource,
	}
}
