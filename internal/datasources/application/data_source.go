package application

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource              = (*applicationDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*applicationDataSource)(nil)
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
}

func NewDataSource() datasource.DataSource { return &applicationDataSource{} }

func (d *applicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (d *applicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Dokploy application by id.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Required: true, Description: "Application id."},
			"name":           schema.StringAttribute{Computed: true, Description: "Display name."},
			"app_name":       schema.StringAttribute{Computed: true, Description: "Dokploy-internal app name."},
			"description":    schema.StringAttribute{Computed: true, Description: "Description."},
			"environment_id": schema.StringAttribute{Computed: true, Description: "Environment id."},
			"source_type":    schema.StringAttribute{Computed: true, Description: "Configured source type (github, git, docker)."},
			"status":         schema.StringAttribute{Computed: true, Description: "Application status."},
			"created_at":     schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
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
				Description: "Environment variables (multiline `KEY=value`), exactly as stored in Dokploy. " +
					"Marked sensitive because it typically holds credentials that this provider did not author; it is redacted in plan output but, like all Terraform data, stored in plain text in state.",
			},
		},
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
	app, err := d.client.GetApplication(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading application", err.Error())
		return
	}
	config.Name = types.StringValue(app.Name)
	config.AppName = types.StringValue(app.AppName)
	config.Description = types.StringPointerValue(app.Description)
	config.EnvironmentID = types.StringValue(app.EnvironmentID)
	config.SourceType = types.StringValue(app.SourceType)
	config.Status = types.StringValue(app.ApplicationStatus)
	config.CreatedAt = types.StringValue(app.CreatedAt)
	config.Env = types.StringPointerValue(app.Env)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
