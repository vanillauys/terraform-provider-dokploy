package application

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*applicationDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*applicationDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*applicationDataSource)(nil)
)

type applicationDataSource struct {
	client *client.Client
}

type dataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	AppName       types.String `tfsdk:"app_name"`
	Description   types.String `tfsdk:"description"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	SourceType    types.String `tfsdk:"source_type"`
	Status        types.String `tfsdk:"status"`
	CreatedAt     types.String `tfsdk:"created_at"`
	Env           types.String `tfsdk:"env"`

	// v1.4.0 (#52). preview_deployments leaves out build_secrets: the data
	// source exposes no secret that this provider did not write.
	Title              types.String `tfsdk:"title"`
	Subtitle           types.String `tfsdk:"subtitle"`
	PreviewDeployments types.Object `tfsdk:"preview_deployments"`
	Rollback           types.Object `tfsdk:"rollback"`
	BuildServerID      types.String `tfsdk:"build_server_id"`
	BuildRegistryID    types.String `tfsdk:"build_registry_id"`
	CleanCache         types.Bool   `tfsdk:"clean_cache"`
	DropBuildPath      types.String `tfsdk:"drop_build_path"`
}

var previewAttrTypes = map[string]attr.Type{
	"enabled": types.BoolType, "env": types.StringType, "build_args": types.StringType,
	"certificate_type": types.StringType, "custom_cert_resolver": types.StringType, "https": types.BoolType,
	"labels": types.ListType{ElemType: types.StringType}, "limit": types.Int64Type, "path": types.StringType,
	"port": types.Int64Type, "require_collaborator_permissions": types.BoolType, "wildcard": types.StringType,
}

var rollbackAttrTypes = map[string]attr.Type{
	"enabled": types.BoolType, "registry_id": types.StringType,
}

func NewDataSource() datasource.DataSource { return &applicationDataSource{} }

func (d *applicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (d *applicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Dokploy application by id, or by name within an environment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Application id. Set this attribute, or set both `environment_id` and `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Exact application name. The lookup searches within `environment_id` and errors when zero or many applications match.",
			},
			"app_name":    schema.StringAttribute{Computed: true, Description: "Internal Dokploy app name."},
			"description": schema.StringAttribute{Computed: true, Description: "Description."},
			"environment_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the environment to search. Required with `name`.",
			},
			"source_type": schema.StringAttribute{Computed: true, Description: "Configured source type: `github`, `git`, or `docker`."},
			"status":      schema.StringAttribute{Computed: true, Description: "Application status."},
			"created_at":  schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
			// Marked sensitive, unlike the resource's `env`. On the resource
			// the value is authored by the practitioner, who can decide what
			// it holds; here it is whatever anyone put in the Dokploy UI —
			// commonly database URLs and API tokens — and a data-source
			// consumer has no way to mark it sensitive themselves. Sensitive
			// keeps it out of plan output; note that Terraform state is still
			// unencrypted, hence the wording below. The sibling
			// `dokploy_postgres` data source makes the same call more bluntly
			// by not exposing the database password at all.
			"env": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "Environment variables as multiline `KEY=value` lines, exactly as Dokploy stores them. " +
					"The attribute is sensitive because it usually holds credentials that this provider did not write. The plan output redacts it, but the state stores it in plain text, like all Terraform data.",
			},
			"title":    schema.StringAttribute{Computed: true, Description: "Display title in the Dokploy UI, or null."},
			"subtitle": schema.StringAttribute{Computed: true, Description: "Display subtitle in the Dokploy UI, or null."},
			"preview_deployments": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Preview deployment settings. The build secrets are not part of the data source.",
				Attributes: map[string]schema.Attribute{
					"enabled":                          schema.BoolAttribute{Computed: true, Description: "Whether Dokploy creates a preview deployment for each pull request."},
					"env":                              schema.StringAttribute{Computed: true, Description: "Environment variables for the preview deployments, or null."},
					"build_args":                       schema.StringAttribute{Computed: true, Description: "Build-time arguments for the preview deployments, or null."},
					"certificate_type":                 schema.StringAttribute{Computed: true, Description: "Certificate strategy for the preview domains: `letsencrypt`, `none`, or `custom`."},
					"custom_cert_resolver":             schema.StringAttribute{Computed: true, Description: "Traefik certificate resolver name, or null."},
					"https":                            schema.BoolAttribute{Computed: true, Description: "Whether the preview domains are served over HTTPS."},
					"labels":                           schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Docker labels for the preview containers, or null."},
					"limit":                            schema.Int64Attribute{Computed: true, Description: "Maximum number of preview deployments that exist at once."},
					"path":                             schema.StringAttribute{Computed: true, Description: "External path that the preview domains match."},
					"port":                             schema.Int64Attribute{Computed: true, Description: "Container port that the preview domains forward to."},
					"require_collaborator_permissions": schema.BoolAttribute{Computed: true, Description: "Whether only pull requests from collaborators get a preview."},
					"wildcard":                         schema.StringAttribute{Computed: true, Description: "Wildcard host for the preview domains, or null."},
				},
			},
			"rollback": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Rollback settings.",
				Attributes: map[string]schema.Attribute{
					"enabled":     schema.BoolAttribute{Computed: true, Description: "Whether Dokploy keeps the image of each successful deploy for a rollback."},
					"registry_id": schema.StringAttribute{Computed: true, Description: "Id of the registry that stores the rollback images, or null."},
				},
			},
			"build_server_id":   schema.StringAttribute{Computed: true, Description: "Id of the build server, or null."},
			"build_registry_id": schema.StringAttribute{Computed: true, Description: "Id of the registry that hands the image from a build server to the application server, or null."},
			"clean_cache":       schema.BoolAttribute{Computed: true, Description: "Whether Dokploy builds without the Docker layer cache."},
			"drop_build_path":   schema.StringAttribute{Computed: true, Description: "Path where Dokploy drops an uploaded build archive, or null."},
		},
	}
}

func (d *applicationDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		datasourcevalidator.RequiredTogether(path.MatchRoot("environment_id"), path.MatchRoot("name")),
	}
}

func (d *applicationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

func (d *applicationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if config.ID.IsNull() {
		services, err := d.client.EnvironmentServices(ctx, config.EnvironmentID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Listing applications", err.Error())
			return
		}
		id, err = client.FindServiceByName(services.Applications, config.Name.ValueString(), "application")
		if err != nil {
			resp.Diagnostics.AddError("Looking up application by name", err.Error())
			return
		}
	}

	app, err := d.client.GetApplication(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading application", err.Error())
		return
	}
	config.ID = types.StringValue(app.ApplicationID)
	config.Name = types.StringValue(app.Name)
	config.AppName = types.StringValue(app.AppName)
	config.Description = tfutil.StringOrNull(app.Description)
	config.EnvironmentID = types.StringValue(app.EnvironmentID)
	config.SourceType = types.StringValue(app.SourceType)
	config.Status = types.StringValue(app.ApplicationStatus)
	config.CreatedAt = types.StringValue(app.CreatedAt)
	config.Env = tfutil.StringOrNull(app.Env)
	config.Title = tfutil.StringOrNull(app.Title)
	config.Subtitle = tfutil.StringOrNull(app.Subtitle)
	config.BuildServerID = tfutil.StringOrNull(app.BuildServerID)
	config.BuildRegistryID = tfutil.StringOrNull(app.BuildRegistryID)
	config.CleanCache = types.BoolValue(app.CleanCache)
	config.DropBuildPath = tfutil.StringOrNull(app.DropBuildPath)

	labels := tfutil.StringListOrNull(ctx, app.PreviewLabels, &resp.Diagnostics)
	preview, diags := types.ObjectValue(previewAttrTypes, map[string]attr.Value{
		"enabled":                          types.BoolValue(app.IsPreviewDeploymentsActive),
		"env":                              tfutil.StringOrNull(app.PreviewEnv),
		"build_args":                       tfutil.StringOrNull(app.PreviewBuildArgs),
		"certificate_type":                 types.StringValue(app.PreviewCertificateType),
		"custom_cert_resolver":             tfutil.StringOrNull(app.PreviewCustomCertResolver),
		"https":                            types.BoolValue(app.PreviewHTTPS),
		"labels":                           labels,
		"limit":                            types.Int64Value(app.PreviewLimit),
		"path":                             types.StringValue(app.PreviewPath),
		"port":                             types.Int64Value(app.PreviewPort),
		"require_collaborator_permissions": types.BoolValue(app.PreviewRequireCollaboratorPermissions),
		"wildcard":                         tfutil.StringOrNull(app.PreviewWildcard),
	})
	resp.Diagnostics.Append(diags...)
	config.PreviewDeployments = preview
	rollback, diags := types.ObjectValue(rollbackAttrTypes, map[string]attr.Value{
		"enabled":     types.BoolValue(app.RollbackActive),
		"registry_id": tfutil.StringOrNull(app.RollbackRegistryID),
	})
	resp.Diagnostics.Append(diags...)
	config.Rollback = rollback
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
