// Package network holds the dokploy_network data source.
//
// This data source exists mainly for networks created, or imported, through
// the Dokploy UI rather than through this provider: network.import (adopting
// a Docker-level network into Dokploy's database) is unmodeled (spec §4.3,
// and internal/resources/network/resource.go's package doc), so a
// UI-imported network reaches Terraform either through this data source or
// through `terraform import` on the dokploy_network resource.
package network

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
	_ datasource.DataSource                     = (*networkDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*networkDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*networkDataSource)(nil)
)

type networkDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &networkDataSource{} }

// ipam is deliberately NOT modelled here, same blast-radius reasoning as
// destination's credentials on a smaller scale: a consumer needs only the
// id to attach a service to the network (see network_ids on
// dokploy_application and the database resources, and service_networks on
// dokploy_compose), and address pools belong to whoever manages the
// dokploy_network resource, not to every consumer that references it.
type model struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	ServerID   types.String `tfsdk:"server_id"`
	Driver     types.String `tfsdk:"driver"`
	Internal   types.Bool   `tfsdk:"internal"`
	Attachable types.Bool   `tfsdk:"attachable"`
	EnableIPv4 types.Bool   `tfsdk:"enable_ipv4"`
	EnableIPv6 types.Bool   `tfsdk:"enable_ipv6"`
	MTU        types.Int64  `tfsdk:"mtu"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

func (d *networkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (d *networkDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.Conflicting(path.MatchRoot("id"), path.MatchRoot("server_id")),
	}
}

func (d *networkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a Docker network already registered in Dokploy, whether created by " +
			"this provider or by the Dokploy UI (including a network imported through the UI).\n\n" +
			"```terraform\n" +
			"data \"dokploy_network\" \"shared\" {\n  name = \"shared-network\"\n}\n\n" +
			"resource \"dokploy_application\" \"app\" {\n  network_ids = [data.dokploy_network.shared.id]\n  # ...\n}\n" +
			"```\n\n" +
			"~> **`ipam` is not exposed here.** A consumer needs only the id to attach a service to the " +
			"network; copying a shared network's address pools into every consumer's state widens their " +
			"blast radius for no gain.\n\n" +
			"~> Network names are unique server-wide in Dokploy, so a name lookup never needs `server_id` " +
			"to disambiguate. `server_id` narrows the lookup by server anyway, and can only be set " +
			"together with `name`, not `id`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Network id. Set this to look it up by id, or leave it unset and set `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Network name, unique server-wide in Dokploy. Exactly one of `id` or `name` must be set.",
			},
			"server_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Id of the remote server to narrow the `name` lookup to. Only usable together " +
					"with `name`; conflicts with `id`.",
			},
			"driver": schema.StringAttribute{
				Computed:    true,
				Description: "Network driver: `bridge` or `overlay`.",
			},
			"internal": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether external access to the network is restricted.",
			},
			"attachable": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether manual container attachment is allowed (overlay networks).",
			},
			"enable_ipv4": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether IPv4 is enabled on the network.",
			},
			"enable_ipv6": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether IPv6 is enabled on the network.",
			},
			"mtu": schema.Int64Attribute{
				Computed:    true,
				Description: "MTU for the network. Null when Docker's default applies.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Creation timestamp (server-side).",
			},
		},
	}
}

func (d *networkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByName resolves a network name (optionally narrowed by serverID) to
// exactly one record.
//
// It never returns matches[0] on a multiple match. Dokploy rejects a
// network.create that reuses an existing name with an HTTP 400 (verified
// live, wave 6b task 1: names are unique server-wide) so the multi-match
// branch below is unreachable in normal operation. It stays as defensive
// code: if it is ever reached, the server state disagrees with what Dokploy
// itself is supposed to guarantee, and silently picking the first match
// would hide exactly that drift.
func findByName(networks []client.Network, name string, serverID *string) (*client.Network, error) {
	var matches []client.Network
	for _, n := range networks {
		if n.Name != name {
			continue
		}
		if serverID != nil && (n.ServerID == nil || *n.ServerID != *serverID) {
			continue
		}
		matches = append(matches, n)
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no network named %q", name)
	default:
		return nil, fmt.Errorf(
			"%d networks are named %q; Dokploy rejects duplicate names, so this indicates server drift - look it up by id and report it",
			len(matches), name)
	}
}

func (d *networkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ExactlyOneOf above guarantees exactly one of id/name is set, and
	// Conflicting guarantees server_id is unset whenever id is set. The id
	// path reads the record directly rather than filtering network.all: one
	// request instead of one plus a scan, and a wrong id surfaces as the
	// server's own not-found rather than as "no network named".
	var found *client.Network
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetNetwork(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the network", err.Error())
			return
		}
		found = got
	} else {
		networks, err := d.client.ListNetworks(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Listing networks", err.Error())
			return
		}
		got, err := findByName(networks, config.Name.ValueString(), config.ServerID.ValueStringPointer())
		if err != nil {
			resp.Diagnostics.AddError("Finding the network", err.Error())
			return
		}
		found = got
	}

	config.ID = types.StringValue(found.NetworkID)
	config.Name = types.StringValue(found.Name)
	config.ServerID = tfutil.StringOrNull(found.ServerID)
	config.Driver = types.StringValue(found.Driver)
	config.Internal = types.BoolValue(found.Internal)
	config.Attachable = types.BoolValue(found.Attachable)
	config.EnableIPv4 = types.BoolValue(found.EnableIPv4)
	config.EnableIPv6 = types.BoolValue(found.EnableIPv6)
	config.MTU = types.Int64PointerValue(found.MTU)
	config.CreatedAt = types.StringValue(found.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
