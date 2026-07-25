package postgres

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
)

var (
	_ datasource.DataSource              = (*postgresDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*postgresDataSource)(nil)
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
		Description: "Look up a Dokploy postgres service by id. The database password is intentionally not exposed.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Required: true, Description: "Postgres service id."},
			"name":           schema.StringAttribute{Computed: true, Description: "Display name."},
			"app_name":       schema.StringAttribute{Computed: true, Description: "Dokploy-internal app name."},
			"environment_id": schema.StringAttribute{Computed: true, Description: "Environment id."},
			"database_name":  schema.StringAttribute{Computed: true, Description: "Database name."},
			"database_user":  schema.StringAttribute{Computed: true, Description: "Database user."},
			"docker_image":   schema.StringAttribute{Computed: true, Description: "Docker image."},
			"external_port":  schema.Int64Attribute{Computed: true, Description: "Exposed host port, if any."},
			"status":         schema.StringAttribute{Computed: true, Description: "Service status."},
			"created_at":     schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
		},
	}
}

func (d *postgresDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.client = c
}

func (d *postgresDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pg, err := d.client.GetPostgres(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading postgres", err.Error())
		return
	}
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
