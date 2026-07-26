package postgres

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*postgresDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*postgresDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*postgresDataSource)(nil)
)

type postgresDataSource struct {
	client *client.Client
}

type dataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	AppName       types.String `tfsdk:"app_name"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	DatabaseName  types.String `tfsdk:"database_name"`
	DatabaseUser  types.String `tfsdk:"database_user"`
	DockerImage   types.String `tfsdk:"docker_image"`
	ExternalPort  types.Int64  `tfsdk:"external_port"`
	Status        types.String `tfsdk:"status"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func NewDataSource() datasource.DataSource { return &postgresDataSource{} }

func (d *postgresDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres"
}

func (d *postgresDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Dokploy postgres service by id, or by name within an environment. The database password is intentionally not exposed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Postgres service id. Set either this or both `environment_id` and `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Exact postgres service name, searched within `environment_id`. Errors when zero or multiple postgres services match.",
			},
			"app_name": schema.StringAttribute{Computed: true, Description: "Dokploy-internal app name."},
			"environment_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the environment to search. Required with `name`.",
			},
			"database_name": schema.StringAttribute{Computed: true, Description: "Database name."},
			"database_user": schema.StringAttribute{Computed: true, Description: "Database user."},
			"docker_image":  schema.StringAttribute{Computed: true, Description: "Docker image."},
			"external_port": schema.Int64Attribute{Computed: true, Description: "Exposed host port, if any."},
			"status":        schema.StringAttribute{Computed: true, Description: "Service status."},
			"created_at":    schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
		},
	}
}

func (d *postgresDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("environment_id"), path.MatchRoot("name")),
	}
}

func (d *postgresDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *postgresDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if config.ID.IsNull() {
		services, err := d.client.EnvironmentServices(ctx, config.EnvironmentID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Listing postgres services", err.Error())
			return
		}
		id, err = client.FindServiceByName(services.Postgres, config.Name.ValueString(), "postgres")
		if err != nil {
			resp.Diagnostics.AddError("Looking up postgres service by name", err.Error())
			return
		}
	}

	pg, err := d.client.GetPostgres(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading postgres", err.Error())
		return
	}
	config.ID = types.StringValue(pg.PostgresID)
	config.Name = types.StringValue(pg.Name)
	config.AppName = types.StringValue(pg.AppName)
	config.EnvironmentID = types.StringValue(pg.EnvironmentID)
	config.DatabaseName = types.StringValue(pg.DatabaseName)
	config.DatabaseUser = types.StringValue(pg.DatabaseUser)
	config.DockerImage = types.StringValue(pg.DockerImage)
	config.ExternalPort = types.Int64PointerValue(pg.ExternalPort)
	config.Status = types.StringValue(pg.ApplicationStatus)
	config.CreatedAt = types.StringValue(pg.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
