// Package server holds the dokploy_server data source.
package server

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*serverDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*serverDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*serverDataSource)(nil)
)

type serverDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &serverDataSource{} }

type model struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	IPAddress           types.String `tfsdk:"ip_address"`
	Port                types.Int64  `tfsdk:"port"`
	Username            types.String `tfsdk:"username"`
	SSHKeyID            types.String `tfsdk:"ssh_key_id"`
	ServerType          types.String `tfsdk:"server_type"`
	EnableDockerCleanup types.Bool   `tfsdk:"enable_docker_cleanup"`
	Command             types.String `tfsdk:"command"`
	AppName             types.String `tfsdk:"app_name"`
	Status              types.String `tfsdk:"status"`
	OrganizationID      types.String `tfsdk:"organization_id"`
	CreatedAt           types.String `tfsdk:"created_at"`
}

func (d *serverDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (d *serverDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *serverDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a remote server that already exists in Dokploy (Settings > Servers), so that a service can " +
			"run on it by name:\n\n" +
			"```terraform\n" +
			"data \"dokploy_server\" \"worker\" {\n  name = \"worker-1\"\n}\n\n" +
			"resource \"dokploy_postgres\" \"db\" {\n  server_id = data.dokploy_server.worker.id\n  # ...\n}\n" +
			"```\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two servers share a name, this data source fails instead of " +
			"a guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Server id. Set it for a lookup by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name as shown in Dokploy. Set exactly one of `id` or `name`.",
			},
			"description":           schema.StringAttribute{Computed: true, Description: "Free-text description, or null."},
			"ip_address":            schema.StringAttribute{Computed: true, Description: "IP address or hostname that Dokploy connects to."},
			"port":                  schema.Int64Attribute{Computed: true, Description: "SSH port."},
			"username":              schema.StringAttribute{Computed: true, Description: "SSH user."},
			"ssh_key_id":            schema.StringAttribute{Computed: true, Description: "Id of the SSH key that Dokploy authenticates with, or null."},
			"server_type":           schema.StringAttribute{Computed: true, Description: "`deploy` or `build`."},
			"enable_docker_cleanup": schema.BoolAttribute{Computed: true, Description: "Whether the daily Docker cleanup runs on the server."},
			"command":               schema.StringAttribute{Computed: true, Description: "Custom setup command, or null."},
			"app_name":              schema.StringAttribute{Computed: true, Description: "Internal name that Dokploy generates for the server."},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Connection status that Dokploy reports, `active` or `inactive`. It changes when Dokploy loses the SSH connection.",
			},
			"organization_id": schema.StringAttribute{Computed: true, Description: "Id of the organization that owns the server."},
			"created_at":      schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *serverDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByName resolves a name to exactly one record. It never returns [0] on
// a multiple match: Dokploy does not enforce name uniqueness on servers.
func findByName(servers []client.Server, name string) (*client.Server, error) {
	var matches []client.Server
	for _, s := range servers {
		if s.Name == name {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no server named %q", name)
	default:
		return nil, fmt.Errorf(
			"%d servers are named %q; names are not unique in Dokploy, so look it up by id instead",
			len(matches), name)
	}
}

func (d *serverDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var found *client.Server
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetServer(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the server", err.Error())
			return
		}
		found = got
	} else {
		servers, err := d.client.ListServers(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing servers", err.Error())
			return
		}
		got, err := findByName(servers, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Finding the server", err.Error())
			return
		}
		found = got
	}
	config.ID = types.StringValue(found.ServerID)
	config.Name = types.StringValue(found.Name)
	config.Description = tfutil.StringOrNull(&found.Description)
	config.IPAddress = types.StringValue(found.IPAddress)
	config.Port = types.Int64Value(found.Port)
	config.Username = types.StringValue(found.Username)
	config.SSHKeyID = tfutil.StringOrNull(&found.SSHKeyID)
	config.ServerType = types.StringValue(found.ServerType)
	config.EnableDockerCleanup = types.BoolValue(found.EnableDockerCleanup)
	config.Command = tfutil.StringOrNull(&found.Command)
	config.AppName = types.StringValue(found.AppName)
	config.Status = types.StringValue(found.ServerStatus)
	config.OrganizationID = types.StringValue(found.OrganizationID)
	config.CreatedAt = types.StringValue(found.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
