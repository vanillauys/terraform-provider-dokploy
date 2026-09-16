// Package compose holds the dokploy_compose data source. It mirrors the
// dokploy_application data source: a lookup by id, or by name within an
// environment, that exposes the flat settings of the service. The source
// blocks and service_networks stay on the resource.
package compose

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
	_ datasource.DataSource                     = (*composeDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*composeDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*composeDataSource)(nil)
)

type composeDataSource struct{ client *client.Client }

func NewDataSource() datasource.DataSource { return &composeDataSource{} }

type model struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	EnvironmentID    types.String `tfsdk:"environment_id"`
	AppName          types.String `tfsdk:"app_name"`
	Description      types.String `tfsdk:"description"`
	ServerID         types.String `tfsdk:"server_id"`
	ComposeType      types.String `tfsdk:"compose_type"`
	SourceType       types.String `tfsdk:"source_type"`
	ComposePath      types.String `tfsdk:"compose_path"`
	Command          types.String `tfsdk:"command"`
	Suffix           types.String `tfsdk:"suffix"`
	Env              types.String `tfsdk:"env"`
	AutoDeploy       types.Bool   `tfsdk:"auto_deploy"`
	TriggerType      types.String `tfsdk:"trigger_type"`
	WatchPaths       types.List   `tfsdk:"watch_paths"`
	EnableSubmodules types.Bool   `tfsdk:"enable_submodules"`
	Randomize        types.Bool   `tfsdk:"randomize"`
	CreateEnvFile    types.Bool   `tfsdk:"create_env_file"`
	Icon             types.String `tfsdk:"icon"`
	Status           types.String `tfsdk:"status"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func (d *composeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compose"
}

func (d *composeDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("environment_id"), path.MatchRoot("name")),
	}
}

func (d *composeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a Dokploy compose service by id, or by name within an environment, for example to attach a " +
			"`dokploy_domain` or a `dokploy_backup` to a stack that Terraform does not manage:\n\n" +
			"```terraform\n" +
			"data \"dokploy_compose\" \"stalwart\" {\n  name           = \"stalwart\"\n  environment_id = data.dokploy_environment.prod.id\n}\n" +
			"```\n\n" +
			"The source blocks and `service_networks` of the resource are not part of the data source.\n\n" +
			"~> Dokploy does not enforce name uniqueness. If two compose services in the environment share a name, this " +
			"data source fails instead of a guess. Look the record up by `id` in that case.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Compose service id. Set this attribute, or set both `environment_id` and `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Exact display name. The lookup searches within `environment_id` and errors when zero or many services match.",
			},
			"environment_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the environment to search. Required with `name`.",
			},
			"app_name":     schema.StringAttribute{Computed: true, Description: "Internal Dokploy app name."},
			"description":  schema.StringAttribute{Computed: true, Description: "Free-form description, or null."},
			"server_id":    schema.StringAttribute{Computed: true, Description: "Id of the remote server that runs the service, or null for the Dokploy host."},
			"compose_type": schema.StringAttribute{Computed: true, Description: "`docker-compose` or `stack`."},
			"source_type":  schema.StringAttribute{Computed: true, Description: "Configured source type: `github`, `gitlab`, `bitbucket`, `gitea`, `git`, or `raw`."},
			"compose_path": schema.StringAttribute{Computed: true, Description: "Path to the compose file inside the repository."},
			"command":      schema.StringAttribute{Computed: true, Description: "Replacement deploy command, or null."},
			"suffix":       schema.StringAttribute{Computed: true, Description: "Suffix for the generated resource names, or null."},
			"env": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "Environment variables as multiline `KEY=value` lines, exactly as Dokploy stores them. " +
					"The attribute is sensitive because it usually holds credentials that this provider did not write. The plan output redacts it, but the state stores it in plain text, like all Terraform data.",
			},
			"auto_deploy":  schema.BoolAttribute{Computed: true, Description: "Whether a source change starts a redeploy, or null."},
			"trigger_type": schema.StringAttribute{Computed: true, Description: "Git event that starts an auto-deploy: `push` or `tag`, or null."},
			"watch_paths": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Paths that limit an auto-deploy, or null.",
			},
			"enable_submodules": schema.BoolAttribute{Computed: true, Description: "Whether Dokploy clones git submodules."},
			"randomize":         schema.BoolAttribute{Computed: true, Description: "Whether Dokploy randomizes the generated resource names."},
			"create_env_file":   schema.BoolAttribute{Computed: true, Description: "Whether Dokploy writes the environment variables to a `.env` file."},
			"icon":              schema.StringAttribute{Computed: true, Description: "Service icon for the Dokploy UI, or null."},
			"status":            schema.StringAttribute{Computed: true, Description: "Service status from Dokploy."},
			"created_at":        schema.StringAttribute{Computed: true, Description: "Creation timestamp from the server."},
		},
	}
}

func (d *composeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func boolOrNull(b *bool) types.Bool { return types.BoolPointerValue(b) }

func (d *composeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if config.ID.IsNull() {
		services, err := d.client.EnvironmentServices(ctx, config.EnvironmentID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Listing compose services", err.Error())
			return
		}
		id, err = client.FindServiceByName(services.Compose, config.Name.ValueString(), "compose")
		if err != nil {
			resp.Diagnostics.AddError("Looking up compose service by name", err.Error())
			return
		}
	}

	co, err := d.client.GetCompose(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading compose service", err.Error())
		return
	}
	config.ID = types.StringValue(co.ComposeID)
	config.Name = types.StringValue(co.Name)
	config.EnvironmentID = types.StringValue(co.EnvironmentID)
	config.AppName = types.StringValue(co.AppName)
	config.Description = tfutil.StringOrNull(co.Description)
	config.ServerID = tfutil.StringOrNull(co.ServerID)
	config.ComposeType = types.StringValue(co.ComposeType)
	config.SourceType = types.StringValue(co.SourceType)
	config.ComposePath = types.StringValue(co.ComposePath)
	config.Command = tfutil.StringOrNull(&co.Command)
	config.Suffix = tfutil.StringOrNull(&co.Suffix)
	config.Env = tfutil.StringOrNull(co.Env)
	config.AutoDeploy = boolOrNull(co.AutoDeploy)
	config.TriggerType = tfutil.StringOrNull(co.TriggerType)
	config.WatchPaths = tfutil.StringListOrNull(ctx, co.WatchPaths, &resp.Diagnostics)
	config.EnableSubmodules = types.BoolValue(co.EnableSubmodules != nil && *co.EnableSubmodules)
	config.Randomize = types.BoolValue(co.Randomize != nil && *co.Randomize)
	config.CreateEnvFile = types.BoolValue(co.CreateEnvFile)
	config.Icon = tfutil.StringOrNull(co.Icon)
	config.Status = types.StringValue(co.ComposeStatus)
	config.CreatedAt = types.StringValue(co.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
