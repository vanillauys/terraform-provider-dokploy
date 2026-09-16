// Package compose holds the dokploy_compose data source. It mirrors the
// dokploy_application data source: a lookup by id, or by name within an
// environment, that exposes the flat settings of the service. The source
// blocks and service_networks stay on the resource.
package compose

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/datasources/dsutil"
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
	return dsutil.ServiceLookup()
}

func (d *composeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	lookup := dsutil.Lookup{
		Kind: "compose service", Plural: "compose services in the environment",
		What: "a Dokploy compose service by id, or by name within an environment, for example to attach a " +
			"`dokploy_domain` or a `dokploy_backup` to a stack that Terraform does not manage",
		Example: "data \"dokploy_compose\" \"stalwart\" {\n  name           = \"stalwart\"\n  environment_id = data.dokploy_environment.prod.id\n}",
		Note:    "The source blocks and `service_networks` of the resource are not part of the data source.",
	}
	attrs := lookup.ServiceAttributes()
	attrs["app_name"] = dsutil.String("Internal Dokploy app name.")
	attrs["description"] = dsutil.String("Free-form description, or null.")
	attrs["server_id"] = dsutil.String("Id of the remote server that runs the service, or null for the Dokploy host.")
	attrs["compose_type"] = dsutil.String("`docker-compose` or `stack`.")
	attrs["source_type"] = dsutil.String("Configured source type: `github`, `gitlab`, `bitbucket`, `gitea`, `git`, or `raw`.")
	attrs["compose_path"] = dsutil.String("Path to the compose file inside the repository.")
	attrs["command"] = dsutil.String("Replacement deploy command, or null.")
	attrs["suffix"] = dsutil.String("Suffix for the generated resource names, or null.")
	attrs["env"] = schema.StringAttribute{
		Computed:  true,
		Sensitive: true,
		Description: "Environment variables as multiline `KEY=value` lines, exactly as Dokploy stores them. " +
			"The attribute is sensitive because it usually holds credentials that this provider did not write. The plan output redacts it, but the state stores it in plain text, like all Terraform data.",
	}
	attrs["auto_deploy"] = dsutil.Bool("Whether a source change starts a redeploy, or null.")
	attrs["trigger_type"] = dsutil.String("Git event that starts an auto-deploy: `push` or `tag`, or null.")
	attrs["watch_paths"] = dsutil.StringList("Paths that limit an auto-deploy, or null.")
	attrs["enable_submodules"] = dsutil.Bool("Whether Dokploy clones git submodules.")
	attrs["randomize"] = dsutil.Bool("Whether Dokploy randomizes the generated resource names.")
	attrs["create_env_file"] = dsutil.Bool("Whether Dokploy writes the environment variables to a `.env` file.")
	attrs["icon"] = dsutil.String("Service icon for the Dokploy UI, or null.")
	attrs["status"] = dsutil.String("Service status from Dokploy.")
	attrs["created_at"] = dsutil.String("Creation timestamp from the server.")
	resp.Schema = schema.Schema{Description: lookup.Description(), Attributes: attrs}
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

	id, diags := dsutil.ResolveService(ctx, d.client, config.ID, config.EnvironmentID, config.Name, "compose",
		func(s *client.EnvironmentServices) []client.ServiceRef { return s.Compose })
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
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
