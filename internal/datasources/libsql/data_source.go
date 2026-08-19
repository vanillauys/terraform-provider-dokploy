// Package libsql holds the dokploy_libsql data source: it looks up an
// existing LibSQL (sqld) database service by id, or by name within an
// environment.
//
// It is modelled on internal/datasources/destination, the standalone
// precedent for a data source that is not one of the five Kind-generic
// database engines (internal/datasources/database): a plain struct, its
// own Schema/Read, and a local findByName that never returns a slice's
// [0] element on ambiguity.
//
// One thing diverges from destination on purpose: database_password IS
// modelled here, Computed and Sensitive, where destination's data source
// omits its credentials entirely. destination's access_key/secret_access_key
// describe a SHARED backup target that many resources reference, so copying
// them into every consumer's state widens the blast radius of one shared
// secret for no gain. A libsql service's database_password is not shared -
// it belongs to this one service - and the five database-engine data
// sources (internal/datasources/database) already expose their own
// credential attributes the same way, Sensitive but not omitted. Omitting
// it here would make dokploy_libsql the only one of six database-shaped
// data sources that cannot report its own password to a consumer that only
// has an id or name, not the managing resource.
package libsql

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
	_ datasource.DataSource                     = (*libsqlDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*libsqlDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*libsqlDataSource)(nil)
)

type libsqlDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &libsqlDataSource{} }

// model is this data source's flat, non-generic read shape - mirroring
// internal/resources/libsql's resourceModel minus the two provider-only
// attributes (deploy_on_change, deployment_timeout) that have nothing to
// read back from the server.
type model struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	EnvironmentID     types.String `tfsdk:"environment_id"`
	AppName           types.String `tfsdk:"app_name"`
	Description       types.String `tfsdk:"description"`
	DatabaseUser      types.String `tfsdk:"database_user"`
	DatabasePassword  types.String `tfsdk:"database_password"`
	SqldNode          types.String `tfsdk:"sqld_node"`
	SqldPrimaryURL    types.String `tfsdk:"sqld_primary_url"`
	EnableNamespaces  types.Bool   `tfsdk:"enable_namespaces"`
	DockerImage       types.String `tfsdk:"docker_image"`
	Env               types.String `tfsdk:"env"`
	ExternalPort      types.Int64  `tfsdk:"external_port"`
	ExternalAdminPort types.Int64  `tfsdk:"external_admin_port"`
	ExternalGRPCPort  types.Int64  `tfsdk:"external_grpc_port"`
	Command           types.String `tfsdk:"command"`
	CPULimit          types.String `tfsdk:"cpu_limit"`
	CPUReservation    types.String `tfsdk:"cpu_reservation"`
	MemoryLimit       types.String `tfsdk:"memory_limit"`
	MemoryReservation types.String `tfsdk:"memory_reservation"`
	Replicas          types.Int64  `tfsdk:"replicas"`
	ServerID          types.String `tfsdk:"server_id"`
	Status            types.String `tfsdk:"status"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

func (d *libsqlDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_libsql"
}

func (d *libsqlDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("environment_id"), path.MatchRoot("name")),
	}
}

func (d *libsqlDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a Dokploy libsql service (a distributed SQLite / `sqld` database) already " +
			"registered in Dokploy, by id or by name within an environment.\n\n" +
			"~> Dokploy does not enforce name uniqueness within an environment. If more than one libsql " +
			"service shares a name this data source fails rather than picking one; look the record up by " +
			"`id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "LibSQL service id. Set this to look it up by id, or leave it unset and set `name` and `environment_id`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name of the libsql service. Exactly one of `id` or `name` must be set. `environment_id` is required together with `name`.",
			},
			"environment_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the environment to search when looking up by `name`. Required together with `name`.",
			},
			"app_name":    schema.StringAttribute{Computed: true, Description: "Dokploy-internal app name. Always server-generated."},
			"description": schema.StringAttribute{Computed: true, Description: "Free-form description."},
			"database_user": schema.StringAttribute{
				Computed:    true,
				Description: "LibSQL database user.",
			},
			// See this file's package doc comment for why, unlike
			// dokploy_destination's data source, this credential IS exposed.
			"database_password": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "LibSQL database password.",
			},
			"sqld_node": schema.StringAttribute{
				Computed:    true,
				Description: "Topology role: `primary` or `replica`.",
			},
			"sqld_primary_url": schema.StringAttribute{
				Computed:    true,
				Description: "URL of the primary sqld node. Set only when `sqld_node` is `replica`.",
			},
			"enable_namespaces": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether sqld namespaces (multi-database mode) are enabled.",
			},
			"docker_image": schema.StringAttribute{Computed: true, Description: "LibSQL docker image."},
			"env": schema.StringAttribute{
				Computed:    true,
				Description: "Extra environment variables in Dokploy's native multiline `KEY=value` format.",
			},
			"external_port":       schema.Int64Attribute{Computed: true, Description: "Host port the libsql HTTP interface is exposed on, if any."},
			"external_admin_port": schema.Int64Attribute{Computed: true, Description: "Host port the libsql admin interface is exposed on, if any."},
			"external_grpc_port":  schema.Int64Attribute{Computed: true, Description: "Host port the libsql gRPC replication interface is exposed on, if any."},
			"command":             schema.StringAttribute{Computed: true, Description: "Container command override, if any."},
			"cpu_limit":           schema.StringAttribute{Computed: true, Description: "Hard CPU limit, Docker-style (e.g. `\"0.5\"`)."},
			"cpu_reservation":     schema.StringAttribute{Computed: true, Description: "Reserved CPU, Docker-style (e.g. `\"0.25\"`)."},
			"memory_limit":        schema.StringAttribute{Computed: true, Description: "Hard memory limit, Docker-style (e.g. `\"512m\"`)."},
			"memory_reservation":  schema.StringAttribute{Computed: true, Description: "Reserved memory, Docker-style (e.g. `\"256m\"`)."},
			"replicas":            schema.Int64Attribute{Computed: true, Description: "Number of container replicas."},
			"server_id":           schema.StringAttribute{Computed: true, Description: "Remote server the service runs on, if not the Dokploy host."},
			"status":              schema.StringAttribute{Computed: true, Description: "Service status reported by Dokploy."},
			"created_at":          schema.StringAttribute{Computed: true, Description: "Creation timestamp (server-side)."},
		},
	}
}

func (d *libsqlDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// findByName resolves a libsql service name to exactly one record within
// the refs ListLibsqlByEnvironment returned for one environment.
//
// It never returns refs[0] on a multiple match. Dokploy does not enforce
// name uniqueness on services within an environment, so two libsql records
// may legitimately share a name, and silently picking the first would bind
// configuration to whichever order the server happened to return - a data
// source that resolves differently between plans with no visible cause.
func findByName(refs []client.ServiceRef, name string) (*client.ServiceRef, error) {
	var matches []client.ServiceRef
	for _, r := range refs {
		if r.Name == name {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("no libsql service named %q in this environment", name)
	default:
		return nil, fmt.Errorf(
			"%d libsql services are named %q in this environment; names are not unique in Dokploy, so look it up by id instead",
			len(matches), name)
	}
}

// int64OrNull mirrors internal/resources/libsql/model.go's helper of the
// same name - duplicated rather than imported, since that one is unexported
// in a different package (internal/resources/libsql vs this package,
// internal/datasources/libsql).
func int64OrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func (d *libsqlDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// ExactlyOneOf above guarantees exactly one of id/name is set, and
	// RequiredTogether guarantees environment_id is set whenever name is.
	// The id path reads the record directly rather than filtering
	// ListLibsqlByEnvironment: one request instead of one plus a scan, and
	// a wrong id surfaces as the server's own not-found rather than as "no
	// libsql service named".
	var found *client.Libsql
	if id := config.ID.ValueString(); id != "" {
		got, err := d.client.GetLibsql(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Reading the libsql service", err.Error())
			return
		}
		found = got
	} else {
		refs, err := d.client.ListLibsqlByEnvironment(ctx, config.EnvironmentID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Listing libsql services", err.Error())
			return
		}
		ref, err := findByName(refs, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Finding the libsql service", err.Error())
			return
		}
		got, err := d.client.GetLibsql(ctx, ref.ID)
		if err != nil {
			resp.Diagnostics.AddError("Reading the libsql service", err.Error())
			return
		}
		found = got
	}

	config.ID = types.StringValue(found.LibsqlID)
	config.Name = types.StringValue(found.Name)
	config.EnvironmentID = types.StringValue(found.EnvironmentID)
	config.AppName = types.StringValue(found.AppName)
	config.Description = tfutil.StringOrNull(found.Description)
	config.DatabaseUser = types.StringValue(found.DatabaseUser)
	config.DatabasePassword = types.StringValue(found.DatabasePassword)
	config.SqldNode = types.StringValue(found.SqldNode)
	config.SqldPrimaryURL = tfutil.StringOrNull(found.SqldPrimaryURL)
	config.EnableNamespaces = types.BoolValue(found.EnableNamespaces)
	config.DockerImage = types.StringValue(found.DockerImage)
	config.Env = tfutil.StringOrNull(found.Env)
	config.ExternalPort = int64OrNull(found.ExternalPort)
	config.ExternalAdminPort = int64OrNull(found.ExternalAdminPort)
	config.ExternalGRPCPort = int64OrNull(found.ExternalGRPCPort)
	config.Command = tfutil.StringOrNull(found.Command)
	config.CPULimit = tfutil.StringOrNull(found.CPULimit)
	config.CPUReservation = tfutil.StringOrNull(found.CPUReservation)
	config.MemoryLimit = tfutil.StringOrNull(found.MemoryLimit)
	config.MemoryReservation = tfutil.StringOrNull(found.MemoryReservation)
	config.Replicas = types.Int64Value(found.Replicas)
	config.ServerID = tfutil.StringOrNull(found.ServerID)
	config.Status = types.StringValue(found.ApplicationStatus)
	config.CreatedAt = types.StringValue(found.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
