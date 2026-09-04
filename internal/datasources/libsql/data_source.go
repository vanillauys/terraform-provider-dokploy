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
// Like destination and the five database-engine data sources
// (internal/datasources/database), this data source does not expose the
// database password. Until v0.11.0 it did, Computed and Sensitive, and was
// the only one of six database-shaped data sources to do so. D2 in the Phase
// 1 brief removed it: one convention for every engine, and a lookup by id
// or name does not copy a credential into the consumer's state.
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
		Description: "Look up a Dokploy libsql service (a distributed SQLite / `sqld` database) by id, or by name " +
			"within an environment. The data source does not expose the database password.\n\n" +
			"~> Dokploy does not enforce name uniqueness within an environment. If more than one libsql " +
			"service shares a name, this data source fails instead of a guess. Look the record up by " +
			"`id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "LibSQL service id. Set it for a lookup by id, or leave it unset and set `name` and `environment_id`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Display name of the libsql service. Set exactly one of `id` or `name`. `name` requires `environment_id`.",
			},
			"environment_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the environment to search for a lookup by `name`. Required together with `name`.",
			},
			"app_name":    schema.StringAttribute{Computed: true, Description: "Internal Dokploy app name. The server always generates it."},
			"description": schema.StringAttribute{Computed: true, Description: "Free-form description."},
			"database_user": schema.StringAttribute{
				Computed:    true,
				Description: "LibSQL database user.",
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
				Description: "Whether sqld namespaces, the multi-database mode, are enabled.",
			},
			"docker_image": schema.StringAttribute{Computed: true, Description: "LibSQL Docker image."},
			"env": schema.StringAttribute{
				Computed:    true,
				Description: "Extra environment variables in the native Dokploy multiline `KEY=value` format.",
			},
			"external_port":       schema.Int64Attribute{Computed: true, Description: "Host port for the libsql HTTP interface, if any."},
			"external_admin_port": schema.Int64Attribute{Computed: true, Description: "Host port for the libsql admin interface, if any."},
			"external_grpc_port":  schema.Int64Attribute{Computed: true, Description: "Host port for the libsql gRPC replication interface, if any."},
			"command":             schema.StringAttribute{Computed: true, Description: "Container command override, if any."},
			"cpu_limit":           schema.StringAttribute{Computed: true, Description: "Hard CPU limit in Docker notation, for example `\"0.5\"`."},
			"cpu_reservation":     schema.StringAttribute{Computed: true, Description: "Reserved CPU in Docker notation, for example `\"0.25\"`."},
			"memory_limit":        schema.StringAttribute{Computed: true, Description: "Hard memory limit in Docker notation, for example `\"512m\"`."},
			"memory_reservation":  schema.StringAttribute{Computed: true, Description: "Reserved memory in Docker notation, for example `\"256m\"`."},
			"replicas":            schema.Int64Attribute{Computed: true, Description: "Number of container replicas."},
			"server_id":           schema.StringAttribute{Computed: true, Description: "Id of the remote server that runs the service, if not the Dokploy host."},
			"status":              schema.StringAttribute{Computed: true, Description: "Service status from Dokploy."},
			"created_at":          schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
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
