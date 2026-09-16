// Package domain holds the dokploy_domain data source.
//
// Domain hosts are not unique in Dokploy: the same host can attach to more
// than one domain, and to an application and a compose service at once. A
// host lookup therefore takes an optional application_id or compose_id
// filter. Without a filter it walks every service in the organization
// (client.ListAllDomains), because Dokploy has no domain.all endpoint.
package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/lookup"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*domainDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*domainDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*domainDataSource)(nil)
)

type domainDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &domainDataSource{} }

type model struct {
	ID                 types.String `tfsdk:"id"`
	Host               types.String `tfsdk:"host"`
	ApplicationID      types.String `tfsdk:"application_id"`
	ComposeID          types.String `tfsdk:"compose_id"`
	Path               types.String `tfsdk:"path"`
	InternalPath       types.String `tfsdk:"internal_path"`
	Port               types.Int64  `tfsdk:"port"`
	HTTPS              types.Bool   `tfsdk:"https"`
	StripPath          types.Bool   `tfsdk:"strip_path"`
	CertificateType    types.String `tfsdk:"certificate_type"`
	CustomCertResolver types.String `tfsdk:"custom_cert_resolver"`
	CustomEntrypoint   types.String `tfsdk:"custom_entrypoint"`
	ServiceName        types.String `tfsdk:"service_name"`
	ForwardAuthEnabled types.Bool   `tfsdk:"forward_auth_enabled"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	Middlewares        types.List   `tfsdk:"middlewares"`
	DomainType         types.String `tfsdk:"domain_type"`
	UniqueConfigKey    types.Int64  `tfsdk:"unique_config_key"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

func (d *domainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}

func (d *domainDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("host")),
	}
}

func (d *domainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a domain, a Traefik router rule, that already exists on a Dokploy application or compose service.\n\n" +
			"```terraform\n" +
			"data \"dokploy_domain\" \"mail\" {\n  host       = \"mail.example.com\"\n  compose_id = data.dokploy_compose.stalwart.id\n}\n" +
			"```\n\n" +
			"~> Dokploy does not enforce host uniqueness: the same host can attach to more than one domain. " +
			"Set `application_id` or `compose_id` to limit the search to one service. Without a filter, the lookup reads the " +
			"domains of every service in the organization, one request per service. If more than one domain matches, " +
			"this data source fails instead of a guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Domain id. Set it for a lookup by id, or leave it unset and set `host`.",
			},
			"host": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Hostname that the domain serves, for example `app.example.com`. Set exactly one of `id` or `host`.",
			},
			"application_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the application that the domain serves. As a filter, it limits a `host` lookup to that application's domains.",
				Validators:  []validator.String{stringvalidator.ConflictsWith(path.MatchRoot("id"), path.MatchRoot("compose_id"))},
			},
			"compose_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the compose service that the domain serves. As a filter, it limits a `host` lookup to that service's domains.",
				Validators:  []validator.String{stringvalidator.ConflictsWith(path.MatchRoot("id"), path.MatchRoot("application_id"))},
			},
			"path":                 schema.StringAttribute{Computed: true, Description: "External path that the rule matches."},
			"internal_path":        schema.StringAttribute{Computed: true, Description: "Path that Traefik forwards to the container."},
			"port":                 schema.Int64Attribute{Computed: true, Description: "Container port that Traefik forwards to."},
			"https":                schema.BoolAttribute{Computed: true, Description: "Whether the domain is served over HTTPS."},
			"strip_path":           schema.BoolAttribute{Computed: true, Description: "Whether Traefik strips `path` before the forward."},
			"certificate_type":     schema.StringAttribute{Computed: true, Description: "Certificate strategy: `letsencrypt`, `none`, or `custom`."},
			"custom_cert_resolver": schema.StringAttribute{Computed: true, Description: "Traefik certificate resolver name, or null."},
			"custom_entrypoint":    schema.StringAttribute{Computed: true, Description: "Traefik entrypoint, or null for the default."},
			"service_name":         schema.StringAttribute{Computed: true, Description: "Compose service that the domain routes to, or null."},
			"forward_auth_enabled": schema.BoolAttribute{Computed: true, Description: "Whether the domain routes through the forward-auth middleware."},
			"enabled":              schema.BoolAttribute{Computed: true, Description: "Whether Traefik serves the domain."},
			"middlewares": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Traefik middlewares on the domain.",
			},
			"domain_type":       schema.StringAttribute{Computed: true, Description: "`application` or `compose`."},
			"unique_config_key": schema.Int64Attribute{Computed: true, Description: "Ordering key that the server assigns."},
			"created_at":        schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *domainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByHost resolves a host to exactly one domain among candidates. It
// never returns [0] on a multiple match: Dokploy does not enforce host
// uniqueness, so two domains, even on one service, can share a host.
func findByHost(candidates []client.Domain, host, scope string) (*client.Domain, error) {
	found, err := lookup.Find(candidates, func(dom client.Domain) bool { return dom.Host == host })
	switch {
	case errors.Is(err, lookup.ErrMultipleMatches):
		return nil, fmt.Errorf("more than one domain has the host %q %s; hosts are not unique in Dokploy, so look it up by id, or narrow the search with application_id or compose_id", host, scope)
	case errors.Is(err, lookup.ErrNoMatch):
		return nil, fmt.Errorf("no domain has the host %q %s", host, scope)
	case err != nil:
		return nil, err
	}
	return &found, nil
}

// candidates lists the domains that a host lookup searches: the domains of
// the filtered service when the config names one, else every domain in the
// organization.
func (d *domainDataSource) candidates(ctx context.Context, config model) ([]client.Domain, string, error) {
	switch {
	case !config.ApplicationID.IsNull():
		ds, err := d.client.ListDomainsByApplication(ctx, config.ApplicationID.ValueString())
		return ds, "on application " + config.ApplicationID.ValueString(), err
	case !config.ComposeID.IsNull():
		ds, err := d.client.ListDomainsByCompose(ctx, config.ComposeID.ValueString())
		return ds, "on compose service " + config.ComposeID.ValueString(), err
	}
	ds, err := d.client.ListAllDomains(ctx)
	return ds, "in the organization", err
}

func (d *domainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Domain
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetDomain(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the domain", err.Error())
			return
		}
		found = got
	} else {
		candidates, scope, err := d.candidates(ctx, config)
		if err != nil {
			resp.Diagnostics.AddError("Listing domains", err.Error())
			return
		}
		got, err := findByHost(candidates, config.Host.ValueString(), scope)
		if err != nil {
			resp.Diagnostics.AddError("Finding the domain", err.Error())
			return
		}
		found = got
	}
	config.ID = types.StringValue(found.DomainID)
	config.Host = types.StringValue(found.Host)
	config.ApplicationID = tfutil.StringOrNull(found.ApplicationID)
	config.ComposeID = tfutil.StringOrNull(found.ComposeID)
	config.Path = types.StringValue(found.Path)
	config.InternalPath = types.StringValue(found.InternalPath)
	config.Port = types.Int64Value(found.Port)
	config.HTTPS = types.BoolValue(found.HTTPS)
	config.StripPath = types.BoolValue(found.StripPath)
	config.CertificateType = types.StringValue(found.CertificateType)
	config.CustomCertResolver = tfutil.StringOrNull(found.CustomCertResolver)
	config.CustomEntrypoint = tfutil.StringOrNull(found.CustomEntrypoint)
	config.ServiceName = tfutil.StringOrNull(found.ServiceName)
	config.ForwardAuthEnabled = types.BoolValue(found.ForwardAuthEnabled)
	config.Enabled = types.BoolValue(found.Enabled)
	config.DomainType = types.StringValue(found.DomainType)
	config.UniqueConfigKey = types.Int64Value(found.UniqueConfigKey)
	config.CreatedAt = types.StringValue(found.CreatedAt)

	middlewares, diags := types.ListValueFrom(ctx, types.StringType, found.Middlewares)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.Middlewares = middlewares
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
