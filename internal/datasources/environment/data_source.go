package environment

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vanillauys/terraform-provider-dokploy/internal/client"
	resenvironment "github.com/vanillauys/terraform-provider-dokploy/internal/resources/environment"
	"github.com/vanillauys/terraform-provider-dokploy/internal/tfutil"
)

var (
	_ datasource.DataSource                     = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure        = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigValidators = (*environmentDataSource)(nil)
)

type environmentDataSource struct {
	client *client.Client
}

type dataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Env         types.String `tfsdk:"env"`
	IsDefault   types.Bool   `tfsdk:"is_default"`
}

func NewDataSource() datasource.DataSource { return &environmentDataSource{} }

func (d *environmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *environmentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Dokploy environment by id, or by name within a project. The usual reason to use this is to get the id of the `production` environment Dokploy creates with every project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Environment id. Set either this or both `project_id` and `name`.",
			},
			"project_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Id of the project to search. Required with `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Exact environment name, searched within `project_id`. Errors when zero or multiple environments match — Dokploy does not enforce unique names.",
			},
			"description": schema.StringAttribute{Computed: true, Description: "Free-form description."},
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
				Description: "Environment-level variables shared by every service in this environment, exactly as stored in Dokploy. " +
					"Marked sensitive because it typically holds credentials that this provider did not author; it is redacted in plan output but, like all Terraform data, stored in plain text in state.",
			},
			"is_default": schema.BoolAttribute{Computed: true, Description: "True for the `production` environment Dokploy creates with each project."},
		},
	}
}

func (d *environmentDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
		// project_id only means anything as the scope for a name search.
		datasourcevalidator.RequiredTogether(path.MatchRoot("project_id"), path.MatchRoot("name")),
	}
}

func (d *environmentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := tfutil.ClientFromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.client = c
	}
}

// FindByName does the client-side exact-match filter the spec prescribes
// where the API has no lookup query. It errors on multiple matches: Dokploy
// permits two environments in one project to share a name, so taking the
// first would silently bind to an arbitrary one.
func FindByName(envs []client.Environment, name string) (*client.Environment, error) {
	var found *client.Environment
	for i := range envs {
		if envs[i].Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("multiple environments named %q in this project; look it up by id instead", name)
		}
		found = &envs[i]
	}
	if found == nil {
		return nil, fmt.Errorf("no environment named %q in this project", name)
	}
	return found, nil
}

func (d *environmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config dataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := config.ID.ValueString()
	if config.ID.IsNull() {
		envs, err := d.client.ListEnvironments(ctx, config.ProjectID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Listing environments", err.Error())
			return
		}
		match, err := FindByName(envs, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Looking up environment by name", err.Error())
			return
		}
		id = match.EnvironmentID
	}

	// Always finish with environment.one: environment.byProjectId omits both
	// `env` and `projectId` from its rows, so a name lookup that stopped at
	// the list result would report them as empty.
	e, err := d.client.GetEnvironment(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Reading environment", err.Error())
		return
	}

	config.ID = types.StringValue(e.EnvironmentID)
	config.ProjectID = types.StringValue(e.ProjectID)
	config.Name = types.StringValue(e.Name)
	config.Description = resenvironment.EmptyToNull(e.Description)
	config.Env = resenvironment.EmptyToNull(e.Env)
	config.IsDefault = types.BoolValue(e.IsDefault)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
